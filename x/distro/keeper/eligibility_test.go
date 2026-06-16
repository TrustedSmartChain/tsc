package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	licensetypes "github.com/webstack-sdk/webstack/x/licenses/types"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// TestSubmitRejectsLicenseNotValidOnDate is the headline eligibility case: a
// license minted for day 2 cannot vote on day 1's distribution.
func TestSubmitRejectsLicenseNotValidOnDate(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(2)
	signer, valOwner := accs[0], accs[1]
	f := setupDistribution(t,
		mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: sdk.ValAddress(valOwner), validator: unitValidator(sdk.ValAddress(valOwner))},
		// License valid only from day 2 onward.
		mockLicensesKeeper{licenses: []licensetypes.License{licenseFrom(signer.String(), dateOf(2), "")}},
		2, // current day = 2, so days 1 and 2 are both within the voting window
	)

	// Not valid on day 1 (license starts day 2): rejected.
	root1, hproof1 := headerOnlySubmission(dateOf(1), 0)
	_, err := f.msgServer.SubmitDistributionRoot(f.ctx, &types.MsgSubmitDistributionRoot{
		Signer: signer.String(), Date: dateOf(1), DistroType: defaultType,
		MerkleRoot: root1, TotalsByCategory: defaultHeaderTotals(), HeaderProof: hproof1,
	})
	require.ErrorContains(t, err, "valid on")

	// Valid on day 2: accepted.
	root2, hproof2 := headerOnlySubmission(dateOf(2), 0)
	_, err = f.msgServer.SubmitDistributionRoot(f.ctx, &types.MsgSubmitDistributionRoot{
		Signer: signer.String(), Date: dateOf(2), DistroType: defaultType,
		MerkleRoot: root2, TotalsByCategory: defaultHeaderTotals(), HeaderProof: hproof2,
	})
	require.NoError(t, err)
}

// TestChallengeRejectsLicenseNotValidOnDate applies the same rule to challenges.
func TestChallengeRejectsLicenseNotValidOnDate(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(6)
	voters := accs[0:4]
	valOwner := accs[4]
	challenger := accs[5]
	stake := map[string]math.Int{
		voters[0].String(): math.NewInt(25), voters[1].String(): math.NewInt(25),
		voters[2].String(): math.NewInt(25), voters[3].String(): math.NewInt(25),
	}
	valAddr := sdk.ValAddress(valOwner)
	staking := mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: valAddr, validator: unitValidator(valAddr), stake: stake}
	licenses := mockLicensesKeeper{licenses: []licensetypes.License{
		activeLicense(voters[0].String()), activeLicense(voters[1].String()),
		activeLicense(voters[2].String()), activeLicense(voters[3].String()),
		licenseFrom(challenger.String(), dateOf(2), ""), // valid only from day 2
	}}
	f := setupDistribution(t, staking, licenses, 2)

	// Reach consensus on day 1 and move it to PENDING.
	for _, v := range voters {
		f.submit(t, v, 1, 0)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1))

	// The challenger's license only became valid the day AFTER day 1, so it may
	// not challenge day 1's distribution.
	f.bank.balances[challenger.String()] = sdk.NewCoins(sdk.NewCoin(testDenom, math.NewInt(100)))
	_, err := f.msgServer.ChallengeDistribution(f.ctx, &types.MsgChallengeDistribution{Challenger: challenger.String(), Date: dateOf(1), DistroType: defaultType})
	require.ErrorContains(t, err, "valid on")
}

// TestTallyExcludesLicensesNotValidOnDay proves the tally itself (not just the
// submit gate) judges eligibility as of the distribution day: votes from holders
// not licensed on that day are not counted toward consensus.
func TestTallyExcludesLicensesNotValidOnDay(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(5)
	voters := accs[0:4]
	valOwner := accs[4]
	stake := map[string]math.Int{
		voters[0].String(): math.NewInt(25), voters[1].String(): math.NewInt(25),
		voters[2].String(): math.NewInt(25), voters[3].String(): math.NewInt(25),
	}
	valAddr := sdk.ValAddress(valOwner)
	staking := mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: valAddr, validator: unitValidator(valAddr), stake: stake}
	// All four licensed only from day 2 — so none are eligible on day 1.
	licenses := mockLicensesKeeper{licenses: []licensetypes.License{
		licenseFrom(voters[0].String(), dateOf(2), ""), licenseFrom(voters[1].String(), dateOf(2), ""),
		licenseFrom(voters[2].String(), dateOf(2), ""), licenseFrom(voters[3].String(), dateOf(2), ""),
	}}
	f := setupDistribution(t, staking, licenses, 2)

	root := []byte("unanimous-root-padded-to-32bytes")
	seedVotingDay := func(day int64) {
		date := dateOf(day)
		require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(date, defaultType), types.Distribution{Date: date, DistroType: defaultType, Status: types.DISTRIBUTION_STATUS_VOTING, VotingSinceDate: date}))
		require.NoError(t, f.k.ActiveDistributions.Set(f.ctx, collections.Join(date, defaultType)))
		for _, v := range voters {
			require.NoError(t, f.k.Votes.Set(f.ctx, collections.Join3(date, defaultType, v.String()), types.DistributionVote{Date: date, DistroType: defaultType, Signer: v.String(), MerkleRoot: root}))
		}
	}

	// Day 1: nobody was licensed yet, so the unanimous votes do NOT count.
	seedVotingDay(1)
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1))
	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_VOTING, ed.Status, "ineligible votes must not reach consensus")

	// Day 2: the same voters ARE licensed, so the same votes reach consensus.
	seedVotingDay(2)
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 2))
	ed, err = f.k.Distributions.Get(f.ctx, collections.Join(dateOf(2), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_PENDING, ed.Status, "same voters are eligible on their license start day")
}

// TestTallyMemoizesAcrossDays proves the pass-wide cache: tallying two days in a
// single epoch end loads the license set once and computes each voter's stake
// once, rather than once per (day, signer).
func TestTallyMemoizesAcrossDays(t *testing.T) {
	accs := simtestutil.CreateIncrementalAccounts(5)
	voters := accs[0:4]
	valOwner := accs[4]
	stake := map[string]math.Int{
		voters[0].String(): math.NewInt(25), voters[1].String(): math.NewInt(25),
		voters[2].String(): math.NewInt(25), voters[3].String(): math.NewInt(25),
	}
	valAddr := sdk.ValAddress(valOwner)

	var byType, totalBonded, dels int
	staking := mockStakingKeeper{
		totalBonded: math.NewInt(100), valAddr: valAddr, validator: unitValidator(valAddr), stake: stake,
		totalBondedCalls: &totalBonded, delegationsCalls: &dels,
	}
	licenses := mockLicensesKeeper{
		licenses: []licensetypes.License{
			activeLicense(voters[0].String()), activeLicense(voters[1].String()),
			activeLicense(voters[2].String()), activeLicense(voters[3].String()),
		},
		byTypeCalls: &byType,
	}
	f := setupDistribution(t, staking, licenses, 2)

	for _, day := range []int64{1, 2} {
		for _, v := range voters {
			f.submit(t, v, day, 0)
		}
	}

	// Isolate the tally pass (submission also queries the licenses keeper).
	byType, totalBonded, dels = 0, 0, 0

	// A single epoch end tallies both day 1 and day 2 under one shared cache.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 2))

	require.Equal(t, 1, byType, "LicensesByType loaded once for the whole pass, not per day")
	require.Equal(t, 1, totalBonded, "TotalBondedTokens computed once for the whole pass")
	require.Equal(t, len(voters), dels, "each voter's stake computed once, not once per (day, voter)")
}
