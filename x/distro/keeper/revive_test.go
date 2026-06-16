package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	licensetypes "github.com/webstack-sdk/webstack/x/licenses/types"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// govAuthority is the module's governance authority address, the only signer
// permitted to revive an expired distribution.
func govAuthority() string {
	return authtypes.NewModuleAddress(govtypes.ModuleName).String()
}

// fourVoterConsensusAt is fourVoterConsensus with a configurable "current" epoch
// (the mock epochs keeper's CurrentEpoch), so revival — which stamps the fresh
// voting window at the current day — can be exercised with time advanced.
func fourVoterConsensusAt(t *testing.T, current int64) (*distFixture, []sdk.AccAddress) {
	t.Helper()
	accs := simtestutil.CreateIncrementalAccounts(5)
	voters := accs[0:4]
	valOwner := accs[4]
	stake := map[string]math.Int{
		voters[0].String(): math.NewInt(25),
		voters[1].String(): math.NewInt(25),
		voters[2].String(): math.NewInt(25),
		voters[3].String(): math.NewInt(25),
	}
	valAddr := sdk.ValAddress(valOwner)
	staking := mockStakingKeeper{totalBonded: math.NewInt(100), valAddr: valAddr, validator: unitValidator(valAddr), stake: stake}
	licenses := mockLicensesKeeper{licenses: []licensetypes.License{
		activeLicense(voters[0].String()), activeLicense(voters[1].String()),
		activeLicense(voters[2].String()), activeLicense(voters[3].String()),
	}}
	return setupDistribution(t, staking, licenses, current), voters
}

// TestReviveExpiredReopensToConsensus drives the full revival happy path: an
// EXPIRED day is revived by governance, re-voted to consensus, and claimed.
func TestReviveExpiredReopensToConsensus(t *testing.T) {
	const current = int64(10)
	f, voters := fourVoterConsensusAt(t, current)
	recipient := simtestutil.CreateIncrementalAccounts(1)[0]

	// Seed an EXPIRED day-1 distribution (never reached consensus in its window).
	require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(dateOf(1), defaultType), types.Distribution{
		Date:       dateOf(1),
		DistroType: defaultType,
		Status:     types.DISTRIBUTION_STATUS_EXPIRED,
	}))

	// Revive it via the governance authority.
	_, err := f.msgServer.ReviveDistribution(f.ctx, &types.MsgReviveDistribution{
		Authority: govAuthority(), Date: dateOf(1), DistroType: defaultType,
	})
	require.NoError(t, err)

	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_VOTING, ed.Status)
	require.Equal(t, dateOf(current), ed.VotingSinceDate, "window must restart at the current day")

	// Re-vote to consensus on the (old) day-1 date even though it is far older
	// than the voting window — the day is already open, so the age-gate is skipped.
	rewards := []rewardLeaf{{nonce: 0, addr: recipient.String(), total: "1000", cats: map[string]string{"type1": "1000"}}}
	root, hproof, rewardProofs := buildSubmission(defaultType, dateOf(1), 0, defaultHeaderTotals(), rewards)
	for _, v := range voters {
		f.submitRoot(t, v, 1, root, hproof)
	}

	// Advance: consensus -> PENDING (since=current) -> LIVE after the review delay.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", current))
	ed, err = f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_PENDING, ed.Status)

	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", current+1))
	ed, err = f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_LIVE, ed.Status)
	require.Equal(t, root, ed.MerkleRoot)

	// The revived day's rewards are now claimable, minted from day 1's budget.
	_, err = f.msgServer.Claim(f.ctx, &types.MsgClaim{
		Claimer: recipient.String(), Date: dateOf(1), DistroType: defaultType, Nonce: 0, Address: recipient.String(),
		Total: "1000", Categories: map[string]string{"type1": "1000"}, Proof: rewardProofs[0],
	})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(1000), f.bank.balances[recipient.String()].AmountOf(testDenom))
}

// TestReviveClearsStaleFields ensures revival wipes any leftover challenge /
// tally bookkeeping so the reopened day starts clean.
func TestReviveClearsStaleFields(t *testing.T) {
	f, _ := fourVoterConsensusAt(t, 10)

	// An expired day that (hypothetically) still carries stale fields.
	require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(dateOf(1), defaultType), types.Distribution{
		Date:             dateOf(1),
		DistroType:       defaultType,
		Status:           types.DISTRIBUTION_STATUS_EXPIRED,
		MerkleRoot:       []byte("stale-root"),
		LicenseTally:     "0.9",
		StakeTally:       "0.9",
		PendingSinceDate: dateOf(2),
		Challenger:       "tsc1stalechallenger",
		ChallengeBond:    "100",
	}))

	_, err := f.msgServer.ReviveDistribution(f.ctx, &types.MsgReviveDistribution{
		Authority: govAuthority(), Date: dateOf(1), DistroType: defaultType,
	})
	require.NoError(t, err)

	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_VOTING, ed.Status)
	require.Empty(t, ed.MerkleRoot)
	require.Empty(t, ed.LicenseTally)
	require.Empty(t, ed.StakeTally)
	require.Empty(t, ed.PendingSinceDate)
	require.Empty(t, ed.Challenger)
	require.Empty(t, ed.ChallengeBond)
}

// TestRevivedDayUsesFreshWindow proves the reopened day's expiry is measured
// from the revival date, not its (ancient) calendar date: it survives until a
// full VoteWindowDays has elapsed from revival, then re-expires.
func TestRevivedDayUsesFreshWindow(t *testing.T) {
	const current = int64(10)
	f, _ := fourVoterConsensusAt(t, current)

	require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(dateOf(1), defaultType), types.Distribution{
		Date:       dateOf(1),
		DistroType: defaultType,
		Status:     types.DISTRIBUTION_STATUS_EXPIRED,
	}))
	_, err := f.msgServer.ReviveDistribution(f.ctx, &types.MsgReviveDistribution{
		Authority: govAuthority(), Date: dateOf(1), DistroType: defaultType,
	})
	require.NoError(t, err)

	window := int64(types.DefaultVoteWindowDays)

	// One epoch short of a full fresh window: still VOTING (would have long since
	// expired if measured from day 1).
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", current+window-1))
	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_VOTING, ed.Status)

	// A full fresh window from revival: re-expires.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", current+window))
	ed, err = f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_EXPIRED, ed.Status)
}

func TestReviveRejectsNonAuthority(t *testing.T) {
	f, _ := fourVoterConsensusAt(t, 10)
	notGov := simtestutil.CreateIncrementalAccounts(1)[0]

	require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(dateOf(1), defaultType), types.Distribution{
		Date:       dateOf(1),
		DistroType: defaultType,
		Status:     types.DISTRIBUTION_STATUS_EXPIRED,
	}))

	_, err := f.msgServer.ReviveDistribution(f.ctx, &types.MsgReviveDistribution{
		Authority: notGov.String(), Date: dateOf(1), DistroType: defaultType,
	})
	require.ErrorContains(t, err, "invalid authority")

	// State is unchanged.
	ed, err := f.k.Distributions.Get(f.ctx, collections.Join(dateOf(1), defaultType))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_EXPIRED, ed.Status)
}

func TestReviveRejectsNonExpired(t *testing.T) {
	f, _ := fourVoterConsensusAt(t, 10)

	// A day that is still VOTING cannot be "revived".
	require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(dateOf(1), defaultType), types.Distribution{
		Date:       dateOf(1),
		DistroType: defaultType,
		Status:     types.DISTRIBUTION_STATUS_VOTING,
	}))
	_, err := f.msgServer.ReviveDistribution(f.ctx, &types.MsgReviveDistribution{
		Authority: govAuthority(), Date: dateOf(1), DistroType: defaultType,
	})
	require.ErrorContains(t, err, "not expired")

	// An unknown date is rejected too.
	_, err = f.msgServer.ReviveDistribution(f.ctx, &types.MsgReviveDistribution{
		Authority: govAuthority(), Date: dateOf(2), DistroType: defaultType,
	})
	require.ErrorContains(t, err, "no distribution")
}
