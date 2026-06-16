package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/keeper"
	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

func TestClaimBudgetInvariant(t *testing.T) {
	f, _ := fourVoterConsensus(t)
	inv := keeper.ClaimBudgetInvariant(f.k)

	// No claims: holds.
	_, broken := inv(f.ctx)
	require.False(t, broken)

	// A day that has claimed within its (large) day-1 budget: still holds.
	require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(dateOf(1), defaultType), types.Distribution{
		Date: dateOf(1), DistroType: defaultType, Status: types.DISTRIBUTION_STATUS_LIVE, MerkleRoot: []byte("r"), ClaimedAmount: "1000",
	}))
	_, broken = inv(f.ctx)
	require.False(t, broken)

	// A day that has claimed beyond any single day's budget: broken.
	require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(dateOf(1), defaultType), types.Distribution{
		Date: dateOf(1), DistroType: defaultType, Status: types.DISTRIBUTION_STATUS_LIVE, MerkleRoot: []byte("r"), ClaimedAmount: types.DefaultMaxSupply,
	}))
	msg, broken := inv(f.ctx)
	require.True(t, broken)
	require.Contains(t, msg, "exceeds budget")
}

func TestBondSolvencyInvariant(t *testing.T) {
	f, _ := fourVoterConsensus(t)
	inv := keeper.BondSolvencyInvariant(f.k)

	// No escrowed bonds and no balance: holds.
	_, broken := inv(f.ctx)
	require.False(t, broken)

	// An UNDER_REVIEW day with an escrowed bond fully backed by module balance.
	require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(dateOf(1), defaultType), types.Distribution{
		Date: dateOf(1), DistroType: defaultType, Status: types.DISTRIBUTION_STATUS_UNDER_REVIEW,
		MerkleRoot: []byte("r"), Challenger: "tsc1challenger", ChallengeBond: "100",
	}))
	f.bank.balances[types.ModuleName] = sdk.NewCoins(sdk.NewCoin(testDenom, math.NewInt(100)))
	_, broken = inv(f.ctx)
	require.False(t, broken, "balance exactly covers the bond")

	// Drain the module below the outstanding bond: insolvent, broken.
	f.bank.balances[types.ModuleName] = sdk.NewCoins(sdk.NewCoin(testDenom, math.NewInt(40)))
	msg, broken := inv(f.ctx)
	require.True(t, broken)
	require.Contains(t, msg, "insolvent")
}

// TestAuditQuery exercises the runtime audit endpoint: it aggregates the
// per-invariant results and the overall broken flag.
func TestAuditQuery(t *testing.T) {
	f, _ := fourVoterConsensus(t)
	q := keeper.NewQuerier(f.k)

	// Healthy state.
	resp, err := q.Audit(f.ctx, &types.QueryAuditRequest{})
	require.NoError(t, err)
	require.False(t, resp.Broken)
	require.Len(t, resp.Results, 2)
	for _, r := range resp.Results {
		require.False(t, r.Broken, r.Name)
	}

	// Introduce an over-budget day; the audit must report broken with the
	// offending invariant flagged.
	require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(dateOf(1), defaultType), types.Distribution{
		Date: dateOf(1), DistroType: defaultType, Status: types.DISTRIBUTION_STATUS_LIVE, MerkleRoot: []byte("r"), ClaimedAmount: types.DefaultMaxSupply,
	}))
	resp, err = q.Audit(f.ctx, &types.QueryAuditRequest{})
	require.NoError(t, err)
	require.True(t, resp.Broken)

	var budget types.AuditResult
	for _, r := range resp.Results {
		if r.Name == "claim-budget" {
			budget = r
		}
	}
	require.True(t, budget.Broken)
	require.Contains(t, budget.Message, "over budget")

	// A nil request is rejected.
	_, err = q.Audit(f.ctx, nil)
	require.Error(t, err)
}
