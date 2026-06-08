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
