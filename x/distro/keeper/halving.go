package keeper

import (
	"time"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// epochBudget returns the maximum total amount distributable for an epoch (day),
// derived from the halving schedule. Epoch N is treated as the Nth distribution
// day, so budget(N) = TotalDistributableAt(dayN) - TotalDistributableAt(dayN-1),
// and the sum of all epoch budgets equals the schedule's cumulative emission.
// This is the per-epoch cap that bounds on-demand claim minting.
func epochBudget(params types.Params, epoch int64) (math.Int, error) {
	if epoch <= 0 {
		return math.ZeroInt(), errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "epoch must be positive")
	}

	schedule, err := newHalvingSchedule(params)
	if err != nil {
		return math.ZeroInt(), err
	}

	dayN := schedule.StartDate.AddDate(0, 0, int(epoch-1))
	cumN, err := schedule.TotalDistributableAt(dayN)
	if err != nil {
		return math.ZeroInt(), err
	}
	if epoch == 1 {
		return cumN, nil
	}

	dayPrev := schedule.StartDate.AddDate(0, 0, int(epoch-2))
	cumPrev, err := schedule.TotalDistributableAt(dayPrev)
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
