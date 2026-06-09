package keeper_test

import (
	"testing"

	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// TestChallengeTimesOutWhenRevoteStalls covers the UNDER_REVIEW liveness bound:
// if a challenge reopens voting but the re-vote never reaches consensus, the day
// must not stay UNDER_REVIEW (bond + distribution locked) forever. Once the
// re-vote window elapses the challenge resolves as frivolous — the original root
// is restored, the bond is burned, and the day returns to PENDING and promotes.
func TestChallengeTimesOutWhenRevoteStalls(t *testing.T) {
	f, voters := fourVoterConsensus(t) // current epoch = 1
	rootA := []byte("root-A-the-original-consensus!!!")
	rootB := []byte("root-B-the-spoiler-no-consensus!")

	for _, v := range voters {
		f.submit(t, v, 1, rootA)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1)) // -> PENDING (rootA, since=day1)

	// Fund and challenge.
	f.bank.balances[voters[0].String()] = sdk.NewCoins(sdk.NewCoin(testDenom, math.NewInt(100)))
	_, err := f.msgServer.ChallengeDistribution(f.ctx, &types.MsgChallengeDistribution{Challenger: voters[0].String(), Date: dateOf(1)})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(100), f.bank.balances[types.ModuleName].AmountOf(testDenom)) // escrowed

	// Split the re-vote so no root reaches the 2/3 thresholds: two voters move to
	// rootB, two remain on rootA (2/4 license, 50/100 stake each — neither passes).
	f.submit(t, voters[0], 1, rootB)
	f.submit(t, voters[1], 1, rootB)

	// Within the re-vote window it stays UNDER_REVIEW (no consensus, not timed out).
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 2))
	ed, err := f.k.Distributions.Get(f.ctx, dateOf(1))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_UNDER_REVIEW, ed.Status)
	require.Equal(t, math.NewInt(100), f.bank.balances[types.ModuleName].AmountOf(testDenom)) // still escrowed

	// Once the re-vote window (vote_window_days, measured from the challenge day)
	// elapses, it times out: bond burned, original root restored, back to PENDING.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1+int64(types.DefaultVoteWindowDays)))
	ed, err = f.k.Distributions.Get(f.ctx, dateOf(1))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_PENDING, ed.Status)
	require.Equal(t, rootA, ed.MerkleRoot, "the pre-challenge consensus root must be restored")
	require.Empty(t, ed.Challenger)
	require.Empty(t, ed.ChallengeBond)
	require.True(t, f.bank.balances[voters[0].String()].AmountOf(testDenom).IsZero(), "bond not refunded")
	require.True(t, f.bank.balances[types.ModuleName].AmountOf(testDenom).IsZero(), "bond burned")
	require.Equal(t, dateOf(1), ed.PendingSinceDate, "original review timer must be preserved")

	// The resumed (already-elapsed) review timer promotes it to LIVE next epoch.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 2+int64(types.DefaultVoteWindowDays)))
	ed, err = f.k.Distributions.Get(f.ctx, dateOf(1))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_LIVE, ed.Status)
	require.Equal(t, rootA, ed.MerkleRoot)
}

// TestChallengeWithinWindowStillResolvesNormally guards against the timeout path
// interfering with a normal, promptly-resolved challenge: a re-vote that reaches
// consensus before the window elapses resolves as before (no timeout).
func TestChallengeWithinWindowStillResolvesNormally(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	rootA := []byte("root-A-buggy-original-result-32b")
	rootB := []byte("root-B-corrected-recompute--32by")

	for _, v := range voters {
		f.submit(t, v, 1, rootA)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1)) // -> PENDING

	f.bank.balances[voters[0].String()] = sdk.NewCoins(sdk.NewCoin(testDenom, math.NewInt(100)))
	_, err := f.msgServer.ChallengeDistribution(f.ctx, &types.MsgChallengeDistribution{Challenger: voters[0].String(), Date: dateOf(1)})
	require.NoError(t, err)

	// Supermajority adopts the corrected root B before the window elapses.
	f.submit(t, voters[0], 1, rootB)
	f.submit(t, voters[1], 1, rootB)
	f.submit(t, voters[2], 1, rootB)

	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 2))
	ed, err := f.k.Distributions.Get(f.ctx, dateOf(1))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_PENDING, ed.Status)
	require.Equal(t, rootB, ed.MerkleRoot, "corrected root adopted, not timed out")
	require.Equal(t, math.NewInt(100), f.bank.balances[voters[0].String()].AmountOf(testDenom), "bond refunded (challenge upheld)")
}

// TestUpheldChallengeRefundToBlockedChallengerBurnsBond covers M2: when an upheld
// challenge's bond cannot be refunded (the challenger address is blocked from
// receiving funds), the transition must still complete — the bond is burned
// rather than erroring and wedging the day UNDER_REVIEW forever.
func TestUpheldChallengeRefundToBlockedChallengerBurnsBond(t *testing.T) {
	f, voters := fourVoterConsensus(t)
	rootA := []byte("root-A-buggy-original-result-32b")
	rootB := []byte("root-B-corrected-recompute--32by")

	for _, v := range voters {
		f.submit(t, v, 1, rootA)
	}
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 1)) // -> PENDING (rootA)

	// Challenge, then block the challenger from receiving funds (e.g. it is a
	// blocked module address) so the refund will fail.
	f.bank.balances[voters[0].String()] = sdk.NewCoins(sdk.NewCoin(testDenom, math.NewInt(100)))
	_, err := f.msgServer.ChallengeDistribution(f.ctx, &types.MsgChallengeDistribution{Challenger: voters[0].String(), Date: dateOf(1)})
	require.NoError(t, err)
	require.Equal(t, math.NewInt(100), f.bank.balances[types.ModuleName].AmountOf(testDenom)) // escrowed
	f.bank.blocked[voters[0].String()] = true

	// Supermajority adopts the corrected root B => challenge upheld => refund due,
	// but the refund is undeliverable. The day must still advance to PENDING with
	// the corrected root, and the bond is burned (not stuck in the module).
	f.submit(t, voters[0], 1, rootB)
	f.submit(t, voters[1], 1, rootB)
	f.submit(t, voters[2], 1, rootB)

	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 2))
	ed, err := f.k.Distributions.Get(f.ctx, dateOf(1))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_PENDING, ed.Status, "day must not wedge UNDER_REVIEW")
	require.Equal(t, rootB, ed.MerkleRoot)
	require.Empty(t, ed.Challenger)
	require.Empty(t, ed.ChallengeBond)
	require.True(t, f.bank.balances[types.ModuleName].AmountOf(testDenom).IsZero(), "bond burned, not left escrowed")
	require.True(t, f.bank.balances[voters[0].String()].AmountOf(testDenom).IsZero(), "blocked challenger not refunded")

	// And it still promotes to LIVE on the next epoch.
	require.NoError(t, f.k.EpochHooks().AfterEpochEnd(f.ctx, "day", 3))
	ed, err = f.k.Distributions.Get(f.ctx, dateOf(1))
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_LIVE, ed.Status)
	require.Equal(t, rootB, ed.MerkleRoot)
}
