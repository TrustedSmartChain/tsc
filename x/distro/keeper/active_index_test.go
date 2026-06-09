package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	licensetypes "github.com/webstack-sdk/webstack/x/licenses/types"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

func active(t *testing.T, f *distFixture, date string) bool {
	t.Helper()
	has, err := f.k.ActiveDistributions.Has(f.ctx, date)
	require.NoError(t, err)
	return has
}

// activeDates returns every date currently in the active index.
func activeDates(t *testing.T, f *distFixture) []string {
	t.Helper()
	var out []string
	require.NoError(t, f.k.ActiveDistributions.Walk(f.ctx, nil, func(date string) (bool, error) {
		out = append(out, date)
		return false, nil
	}))
	return out
}

// TestActiveIndexTracksLifecycle verifies a day is indexed while non-terminal and
// dropped once it reaches LIVE, so the epoch hook stops scanning it.
func TestActiveIndexTracksLifecycle(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	root := []byte("consensus-root-padded-to-32bytes")

	// Not present before any submission.
	require.False(t, active(t, f, dateOf(1)))

	for _, v := range voters {
		f.submit(t, v, 1, root)
	}
	// Indexed once voting opens.
	require.True(t, active(t, f, dateOf(1)))

	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1)) // -> PENDING
	require.True(t, active(t, f, dateOf(1)), "still indexed while PENDING")

	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 2)) // -> LIVE
	require.False(t, active(t, f, dateOf(1)), "dropped once LIVE")
	require.Empty(t, activeDates(t, f), "no active days remain")
}

// TestActiveIndexDropsExpired verifies an expired day is removed from the index.
func TestActiveIndexDropsExpired(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(2)
	signer, other := accs[0], accs[1]
	f := setupDistribution(t,
		mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: sdk.ValAddress(other), validator: unitValidator(sdk.ValAddress(other)), stake: map[string]math.Int{signer.String(): math.NewInt(10)}},
		mockLicensesKeeper{licenses: []licensetypes.License{activeLicense(signer.String()), activeLicense(other.String())}},
		1,
	)
	f.submit(t, signer, 1, []byte("lonely-root---------------------"))
	require.True(t, active(t, f, dateOf(1)))

	// Within the window it stays indexed.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1))
	require.True(t, active(t, f, dateOf(1)))

	// After the window it expires and is dropped from the index.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1+int64(types.DefaultVoteWindowDays)))
	require.False(t, active(t, f, dateOf(1)))
}

// TestActiveIndexSelfHealsMissingDistribution covers the defensive path: a stale
// index entry with no backing distribution is pruned rather than erroring.
func TestActiveIndexSelfHealsMissingDistribution(t *testing.T) {
	f, _ := fourVoterConsensus(t)

	// Point the index at a date with no Distribution record.
	require.NoError(t, f.k.ActiveDistributions.Set(f.ctx, dateOf(1)))
	require.True(t, active(t, f, dateOf(1)))

	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1))
	require.False(t, active(t, f, dateOf(1)), "stale index entry pruned")
}

// TestRebuildActiveDistributions checks the upgrade-time backfill indexes exactly
// the non-terminal days.
func TestRebuildActiveDistributions(t *testing.T) {
	f, _ := fourVoterConsensus(t)

	seed := []struct {
		date   string
		status types.DistributionStatus
	}{
		{dateOf(1), types.DISTRIBUTION_STATUS_VOTING},
		{dateOf(2), types.DISTRIBUTION_STATUS_PENDING},
		{dateOf(3), types.DISTRIBUTION_STATUS_UNDER_REVIEW},
		{dateOf(4), types.DISTRIBUTION_STATUS_LIVE},
		{dateOf(5), types.DISTRIBUTION_STATUS_EXPIRED},
	}
	for _, s := range seed {
		require.NoError(t, f.k.Distributions.Set(f.ctx, s.date, types.Distribution{Date: s.date, Status: s.status, MerkleRoot: []byte("r")}))
	}

	require.NoError(t, f.k.RebuildActiveDistributions(f.ctx))

	require.Equal(t, []string{dateOf(1), dateOf(2), dateOf(3)}, activeDates(t, f))
}
