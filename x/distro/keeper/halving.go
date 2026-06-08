package keeper

import (
	"time"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// dateBudget returns the maximum total amount distributable for a single day,
// derived from the halving schedule:
//
//	budget(date) = TotalDistributableAt(date) - TotalDistributableAt(date-1d)
//
// (the start date's budget is just TotalDistributableAt(startDate)). The sum of
// all daily budgets equals the schedule's cumulative emission. This is the
// per-day cap that bounds on-demand claim minting.
func dateBudget(params types.Params, date string) (math.Int, error) {
	schedule, err := newHalvingSchedule(params)
	if err != nil {
		return math.ZeroInt(), err
	}

	target, err := parseDate(date)
	if err != nil {
		return math.ZeroInt(), errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid date %q: must be YYYY-MM-DD", date)
	}
	if target.Before(schedule.StartDate) {
		return math.ZeroInt(), errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "date %q is before the distribution start date", date)
	}

	cumN, err := schedule.TotalDistributableAt(target)
	if err != nil {
		return math.ZeroInt(), err
	}
	if target.Equal(schedule.StartDate) {
		return cumN, nil
	}

	cumPrev, err := schedule.TotalDistributableAt(target.AddDate(0, 0, -1))
	if err != nil {
		return math.ZeroInt(), err
	}

	budget := cumN.Sub(cumPrev)
	if budget.IsNegative() {
		return math.ZeroInt(), nil
	}
	return budget, nil
}

func parseDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}

// dateForEpoch maps an x/epochs epoch number to its distribution day
// (YYYY-MM-DD): epoch 1 is the start date, epoch N is startDate+(N-1) days. This
// is how the daily epoch hook is translated into the date the module keys state
// by.
func dateForEpoch(startDateStr string, epoch int64) (string, error) {
	start, err := parseDate(startDateStr)
	if err != nil {
		return "", errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid distribution start date: %v", err)
	}
	if epoch <= 0 {
		return "", errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "epoch must be positive")
	}
	return start.AddDate(0, 0, int(epoch-1)).Format("2006-01-02"), nil
}

// daysBetween returns the number of whole days from `from` to `to` (positive
// when `to` is later). Both dates must be YYYY-MM-DD.
func daysBetween(from, to string) (int64, error) {
	f, err := parseDate(from)
	if err != nil {
		return 0, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid date %q: must be YYYY-MM-DD", from)
	}
	t, err := parseDate(to)
	if err != nil {
		return 0, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid date %q: must be YYYY-MM-DD", to)
	}
	return int64(t.Sub(f).Hours() / 24), nil
}

// HalvingSchedule encapsulates the token distribution logic with halving periods.
type HalvingSchedule struct {
	StartDate       time.Time
	MonthsPerPeriod uint64
	MaxSupply       math.Int
}

func newHalvingSchedule(params types.Params) (*HalvingSchedule, error) {
	startDate, err := parseDate(params.DistributionStartDate)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid distribution start date: %v", err)
	}

	maxSupply, ok := math.NewIntFromString(params.MaxSupply)
	if !ok {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "invalid max supply")
	}

	if params.MonthsInHalvingPeriod == 0 {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "param MonthsInHalvingPeriod must be greater than zero")
	}

	return &HalvingSchedule{
		StartDate:       startDate,
		MonthsPerPeriod: params.MonthsInHalvingPeriod,
		MaxSupply:       maxSupply,
	}, nil
}

// PeriodAllocation returns the total tokens allocated for a given period.
// Period 1: MaxSupply/2, Period 2: MaxSupply/4, Period 3: MaxSupply/8, etc.
func (h *HalvingSchedule) PeriodAllocation(period uint64) math.Int {
	if period == 0 {
		return math.ZeroInt()
	}

	// Period n allocation = MaxSupply / 2^n
	divisor := math.NewIntFromUint64(1 << period) // 2^period using bit shift
	return h.MaxSupply.Quo(divisor)
}

// PeriodBounds returns the start (inclusive) and end (inclusive) dates for a period.
func (h *HalvingSchedule) PeriodBounds(period uint64) (start, end time.Time) {
	monthsOffset := int((period - 1) * h.MonthsPerPeriod)
	start = h.StartDate.AddDate(0, monthsOffset, 0)
	end = h.StartDate.AddDate(0, monthsOffset+int(h.MonthsPerPeriod), -1)
	return start, end
}

// CurrentPeriod returns the halving period for a given date.
// Uses day-based comparison to correctly handle leap years and variable month lengths.
func (h *HalvingSchedule) CurrentPeriod(targetDate time.Time) (uint64, error) {
	if targetDate.Before(h.StartDate) {
		return 0, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "target date is before distribution start date")
	}

	// Find which period contains the target date by checking period bounds
	period := uint64(1)
	for {
		periodStart, periodEnd := h.PeriodBounds(period)

		// Check if targetDate falls within this period (inclusive on both ends)
		if !targetDate.Before(periodStart) && !targetDate.After(periodEnd) {
			return period, nil
		}

		period++
		if period > 1000 { // prevent infinite loop
			return period, nil
		}
	}
}

// daysInRange counts days between two dates (inclusive), properly handling leap years.
func daysInRange(start, end time.Time) int {
	return int(end.Sub(start).Hours()/24) + 1 // +1 because both start and end are inclusive
}

// daysSinceStart counts days from start to target.
func daysSinceStart(start, target time.Time) int {
	days := int(target.Sub(start).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// TotalDistributableAt calculates the maximum tokens that can be distributed by a given date.
func (h *HalvingSchedule) TotalDistributableAt(targetDate time.Time) (math.Int, error) {
	currentPeriod, err := h.CurrentPeriod(targetDate)
	if err != nil {
		return math.ZeroInt(), err
	}

	total := math.ZeroInt()

	// Add full allocations from all completed periods
	for period := uint64(1); period < currentPeriod; period++ {
		total = total.Add(h.PeriodAllocation(period))
	}

	// Add pro-rata allocation from current period
	periodStart, periodEnd := h.PeriodBounds(currentPeriod)
	daysInPeriod := daysInRange(periodStart, periodEnd)
	daysElapsed := daysSinceStart(periodStart, targetDate) + 1 // +1 because current day counts

	if daysInPeriod > 0 {
		periodAllocation := h.PeriodAllocation(currentPeriod)
		// Pro-rata: (allocation * daysElapsed) / daysInPeriod
		currentPeriodAmount := periodAllocation.Mul(math.NewInt(int64(daysElapsed))).Quo(math.NewInt(int64(daysInPeriod)))
		total = total.Add(currentPeriodAmount)
	}

	return total, nil
}
