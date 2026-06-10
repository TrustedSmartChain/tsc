package keeper

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// dayString maps a 1-based day index to its YYYY-MM-DD date using the schedule's
// start date (day 1 == start date).
func dayString(p types.Params, n int) string {
	start, err := time.Parse("2006-01-02", p.DistributionStartDate)
	if err != nil {
		panic(err)
	}
	return start.AddDate(0, 0, n-1).Format("2006-01-02")
}

// Day 1's budget is the whole cumulative emission up to and including the start
// date (there is no prior day to subtract).
func TestDateBudgetDayOneEqualsCumulative(t *testing.T) {
	p := types.DefaultParams()
	sched, err := newHalvingSchedule(p)
	require.NoError(t, err)

	cum, err := sched.TotalDistributableAt(sched.StartDate)
	require.NoError(t, err)

	b, err := dateBudget(p, dayString(p, 1))
	require.NoError(t, err)
	require.Equal(t, cum, b)
}

// Every later day's budget is exactly the one-day delta of the cumulative curve.
func TestDateBudgetIsCumulativeDelta(t *testing.T) {
	p := types.DefaultParams()
	sched, err := newHalvingSchedule(p)
	require.NoError(t, err)

	for n := 2; n <= 5; n++ {
		dayN := sched.StartDate.AddDate(0, 0, n-1)
		dayPrev := sched.StartDate.AddDate(0, 0, n-2)
		cumN, err := sched.TotalDistributableAt(dayN)
		require.NoError(t, err)
		cumPrev, err := sched.TotalDistributableAt(dayPrev)
		require.NoError(t, err)

		b, err := dateBudget(p, dayString(p, n))
		require.NoError(t, err)
		require.Equal(t, cumN.Sub(cumPrev), b)
		require.True(t, b.IsPositive())
	}
}

// The daily budgets telescope: summing them up to day N equals the cumulative
// distributable at day N. This is the core invariant that bounds total minting
// to the emission curve.
func TestDateBudgetTelescopes(t *testing.T) {
	p := types.DefaultParams()
	sched, err := newHalvingSchedule(p)
	require.NoError(t, err)

	const N = 30
	sum := math.ZeroInt()
	for n := 1; n <= N; n++ {
		b, err := dateBudget(p, dayString(p, n))
		require.NoError(t, err)
		sum = sum.Add(b)
	}

	cumN, err := sched.TotalDistributableAt(sched.StartDate.AddDate(0, 0, N-1))
	require.NoError(t, err)
	require.Equal(t, cumN, sum)
}

func TestDateBudgetRejectsBeforeStart(t *testing.T) {
	p := types.DefaultParams() // start date 2025-07-22
	_, err := dateBudget(p, "2025-07-21")
	require.Error(t, err)
	require.ErrorContains(t, err, "before the distribution start date")
}

func TestDateBudgetRejectsMalformedDate(t *testing.T) {
	p := types.DefaultParams()
	_, err := dateBudget(p, "not-a-date")
	require.Error(t, err)
	require.ErrorContains(t, err, "YYYY-MM-DD")
}

// PeriodAllocation must not panic for large periods: 1<<period overflows a
// uint64 at period 64 (wrapping to 0) and would divide by zero. The big.Int
// divisor keeps it correct — periods past ~84 round to zero.
func TestPeriodAllocationLargePeriodDoesNotPanic(t *testing.T) {
	sched, err := newHalvingSchedule(types.DefaultParams())
	require.NoError(t, err)

	// Around the uint64-shift boundary it must stay well-defined.
	require.True(t, sched.PeriodAllocation(63).IsPositive())
	require.NotPanics(t, func() { sched.PeriodAllocation(64) })
	require.NotPanics(t, func() { sched.PeriodAllocation(200) })
	// Far beyond the supply's bit-width the allocation is exactly zero.
	require.True(t, sched.PeriodAllocation(200).IsZero())
	// Allocations are monotonically non-increasing across the boundary.
	require.True(t, sched.PeriodAllocation(64).LTE(sched.PeriodAllocation(63)))
}

// The keeper must be a pure delegate of the x/distro/types halving functions:
// for any date, dateBudget(params, date) must equal types.DateBudget with the
// params spelled out, and the schedule's cumulative curve must equal
// types.TotalDistributableAt. This guards against the keeper ever growing its
// own copy of the math and drifting from what off-chain workers compute.
func TestKeeperHalvingMatchesTypesAcrossDates(t *testing.T) {
	p := types.DefaultParams()
	maxSupply, ok := math.NewIntFromString(p.MaxSupply)
	require.True(t, ok)

	sched, err := newHalvingSchedule(p)
	require.NoError(t, err)

	// Sample the schedule: early days, both sides of the period 1 → 2 boundary,
	// mid-period days, and a date many periods in the future.
	_, p1End := sched.PeriodBounds(1)
	boundary := int(p1End.Sub(sched.StartDate).Hours()/24) + 1 // day index of period 1's last day
	days := []int{1, 2, 30, 365, boundary - 1, boundary, boundary + 1, boundary + 2, boundary + 400, 365 * 20}

	for _, n := range days {
		date := dayString(p, n)

		keeperBudget, err := dateBudget(p, date)
		require.NoError(t, err, date)
		typesBudget, err := types.DateBudget(date, maxSupply, p.DistributionStartDate, p.MonthsInHalvingPeriod)
		require.NoError(t, err, date)
		require.Equal(t, typesBudget, keeperBudget, "DateBudget mismatch on %s", date)

		keeperCum, err := sched.TotalDistributableAt(sched.StartDate.AddDate(0, 0, n-1))
		require.NoError(t, err, date)
		typesCum, err := types.TotalDistributableAt(date, maxSupply, p.DistributionStartDate, p.MonthsInHalvingPeriod)
		require.NoError(t, err, date)
		require.Equal(t, typesCum, keeperCum, "TotalDistributableAt mismatch on %s", date)
	}
}

// CurrentPeriod returns an error (rather than silently returning an
// out-of-schedule period) for a date beyond the supported horizon.
func TestCurrentPeriodBeyondHorizonErrors(t *testing.T) {
	sched, err := newHalvingSchedule(types.DefaultParams())
	require.NoError(t, err)

	// MaxHalvingPeriods * MonthsPerPeriod months out is past the supported range.
	farFuture := sched.StartDate.AddDate(0, int((types.MaxHalvingPeriods+2)*sched.MonthsPerPeriod), 0)
	_, err = sched.CurrentPeriod(farFuture)
	require.Error(t, err)
	require.ErrorContains(t, err, "beyond the supported halving schedule")
}
