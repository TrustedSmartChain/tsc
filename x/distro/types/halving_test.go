package types_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// These tests pin the halving math exposed by x/distro/types as pure functions.
// Off-chain workers import these functions to compute the per-day budget, so
// the expected values below are derived independently (explicit day counts and
// integer arithmetic) rather than by calling the code under test.

// Default schedule: start 2025-07-22, 48-month periods, max supply 21e24.
// Period 1 spans 2025-07-22 .. 2029-07-21 = 1461 days (2028 is a leap year);
// period 2 spans 2029-07-22 .. 2033-07-21 = 1461 days (2032 is a leap year).
const daysInDefaultPeriod = 1461

func defaultScheduleInputs(t *testing.T) (maxSupply math.Int, startDate string, months uint64) {
	t.Helper()
	p := types.DefaultParams()
	maxSupply, ok := math.NewIntFromString(p.MaxSupply)
	require.True(t, ok)
	return maxSupply, p.DistributionStartDate, p.MonthsInHalvingPeriod
}

// On the start date the cumulative emission is one day's pro-rata slice of
// period 1's allocation, and the day budget equals it (no prior day).
func TestTypesHalvingStartDate(t *testing.T) {
	maxSupply, start, months := defaultScheduleInputs(t)
	alloc1 := maxSupply.QuoRaw(2)
	expected := alloc1.MulRaw(1).QuoRaw(daysInDefaultPeriod)

	cum, err := types.TotalDistributableAt(start, maxSupply, start, months)
	require.NoError(t, err)
	require.Equal(t, expected, cum)

	budget, err := types.DateBudget(start, maxSupply, start, months)
	require.NoError(t, err)
	require.Equal(t, expected, budget)
}

// On the last day of period 1 the pro-rata fraction is daysInPeriod/daysInPeriod,
// so the cumulative emission is exactly the period's full allocation —
// truncation leaves no dust at a period boundary.
func TestTypesHalvingLastDayOfPeriodOne(t *testing.T) {
	maxSupply, start, months := defaultScheduleInputs(t)
	alloc1 := maxSupply.QuoRaw(2)

	cum, err := types.TotalDistributableAt("2029-07-21", maxSupply, start, months)
	require.NoError(t, err)
	require.Equal(t, alloc1, cum)

	// The day budget is the delta against day daysInPeriod-1.
	expectedBudget := alloc1.Sub(alloc1.MulRaw(daysInDefaultPeriod - 1).QuoRaw(daysInDefaultPeriod))
	budget, err := types.DateBudget("2029-07-21", maxSupply, start, months)
	require.NoError(t, err)
	require.Equal(t, expectedBudget, budget)
}

// On the first day of period 2 the cumulative emission is period 1's full
// allocation plus one day's slice of period 2's (halved) allocation.
func TestTypesHalvingFirstDayOfPeriodTwo(t *testing.T) {
	maxSupply, start, months := defaultScheduleInputs(t)
	alloc1 := maxSupply.QuoRaw(2)
	alloc2 := maxSupply.QuoRaw(4)

	cum, err := types.TotalDistributableAt("2029-07-22", maxSupply, start, months)
	require.NoError(t, err)
	require.Equal(t, alloc1.Add(alloc2.MulRaw(1).QuoRaw(daysInDefaultPeriod)), cum)

	// The first day of period 2 pays one slice of the halved allocation: the
	// delta against period 1's last day (exactly alloc1).
	budget, err := types.DateBudget("2029-07-22", maxSupply, start, months)
	require.NoError(t, err)
	require.Equal(t, alloc2.MulRaw(1).QuoRaw(daysInDefaultPeriod), budget)
}

// A schedule whose period contains Feb 29 must count 366 days: the pro-rata
// denominator includes the leap day and the budgets telescope across it.
func TestTypesHalvingLeapYearBoundary(t *testing.T) {
	maxSupply := math.NewInt(1_000_000_000_000)
	start := "2024-01-01"
	months := uint64(12) // period 1 = 2024-01-01 .. 2024-12-31 = 366 days (2024 is a leap year)
	alloc1 := maxSupply.QuoRaw(2)

	// 2024-02-29 is day 60 of the period.
	cumFeb29, err := types.TotalDistributableAt("2024-02-29", maxSupply, start, months)
	require.NoError(t, err)
	require.Equal(t, alloc1.MulRaw(60).QuoRaw(366), cumFeb29)

	// Budgets on the leap day and the day after are exact one-day deltas.
	for _, tc := range []struct {
		date string
		day  int64
	}{
		{"2024-02-29", 60},
		{"2024-03-01", 61},
	} {
		budget, err := types.DateBudget(tc.date, maxSupply, start, months)
		require.NoError(t, err)
		expected := alloc1.MulRaw(tc.day).QuoRaw(366).Sub(alloc1.MulRaw(tc.day - 1).QuoRaw(366))
		require.Equal(t, expected, budget, tc.date)
	}

	// The last day of the 366-day period still lands exactly on the allocation.
	cumEnd, err := types.TotalDistributableAt("2024-12-31", maxSupply, start, months)
	require.NoError(t, err)
	require.Equal(t, alloc1, cumEnd)
}

// A date far in the future (period 5 of the default schedule) sums the four
// completed allocations plus the pro-rata slice of the current period.
func TestTypesHalvingFarFuture(t *testing.T) {
	maxSupply, start, months := defaultScheduleInputs(t)

	startDate, err := time.Parse("2006-01-02", start)
	require.NoError(t, err)
	p5Start := startDate.AddDate(0, 4*int(months), 0)               // 2041-07-22
	p5End := startDate.AddDate(0, 5*int(months), -1)                // 2045-07-21
	daysInP5 := int64(p5End.Sub(p5Start).Hours()/24) + 1            // 1461 (2044 is a leap year)
	target := p5Start.AddDate(0, 0, 100).Format("2006-01-02")       // day 101 of period 5

	completed := math.ZeroInt()
	for n := int64(1); n <= 4; n++ {
		completed = completed.Add(maxSupply.Quo(math.NewInt(1 << n)))
	}
	alloc5 := maxSupply.QuoRaw(32)
	expected := completed.Add(alloc5.MulRaw(101).Quo(math.NewInt(daysInP5)))

	cum, err := types.TotalDistributableAt(target, maxSupply, start, months)
	require.NoError(t, err)
	require.Equal(t, expected, cum)

	// Beyond the supported horizon the functions error instead of looping.
	horizon := startDate.AddDate(0, int((types.MaxHalvingPeriods+2)*months), 0).Format("2006-01-02")
	_, err = types.TotalDistributableAt(horizon, maxSupply, start, months)
	require.Error(t, err)
	require.ErrorContains(t, err, "beyond the supported halving schedule")
}

func TestTypesHalvingInputValidation(t *testing.T) {
	maxSupply, start, months := defaultScheduleInputs(t)

	_, err := types.DateBudget("not-a-date", maxSupply, start, months)
	require.Error(t, err)
	require.ErrorContains(t, err, "YYYY-MM-DD")

	_, err = types.DateBudget("2025-07-21", maxSupply, start, months)
	require.Error(t, err)
	require.ErrorContains(t, err, "before the distribution start date")

	_, err = types.TotalDistributableAt("2025-07-21", maxSupply, start, months)
	require.Error(t, err)
	require.ErrorContains(t, err, "before distribution start date")

	_, err = types.DateBudget(start, maxSupply, "bad-start", months)
	require.Error(t, err)
	require.ErrorContains(t, err, "invalid distribution start date")

	_, err = types.DateBudget(start, maxSupply, start, 0)
	require.Error(t, err)
	require.ErrorContains(t, err, "greater than zero")
}
