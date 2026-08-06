package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/stretchr/testify/require"

	networktypes "github.com/webstack-sdk/webstack/x/network/types"

	"github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

func attestations(n int) []types.ContractAttestation {
	out := make([]types.ContractAttestation, n)
	for i := range out {
		out[i] = types.ContractAttestation{
			ContractAddress: "0x00000000000000000000000000000000000000aa",
			CurrentSupply:   math.NewInt(int64(1000 + i)),
			BlockHeight:     42,
		}
	}
	return out
}

func TestAttestRwaTrustOnly(t *testing.T) {
	f := SetupTest(t)

	trust := f.addNode(nodeTypeTrust, networktypes.NodeActive)
	nano := f.addNode(nodeTypeNano, networktypes.NodeActive)

	// MsgAttestRwa names only trust. Being licensed for a tier is not the same
	// as the tier being allowed to make this claim.
	_, err := f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  nano.Address,
		Attestations: attestations(1),
	})
	require.ErrorIs(t, err, types.ErrNodeTypeNotAllowed)

	// Trust nodes may; activity is touched and one event per attestation is
	// emitted.
	_, err = f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  trust.Address,
		Attestations: attestations(3),
	})
	require.NoError(t, err)
	require.Equal(t, []string{trust.Address}, f.network.touched)

	events := 0
	for _, ev := range f.ctx.EventManager().Events() {
		if ev.Type == types.EventTypeAttestRwa {
			events++
		}
	}
	require.Equal(t, 3, events)
}

// Both fixture tiers are named by MsgAttestRwu, so both may report usage. The
// list is explicit rather than "any active node", so a tier added later reports
// nothing until it is added to that list too.
func TestAttestRwuAllowedTypes(t *testing.T) {
	f := SetupTest(t)

	for _, nodeType := range []string{nodeTypeTrust, nodeTypeNano} {
		node := f.addNode(nodeType, networktypes.NodeActive)
		_, err := f.msgServer.AttestRwu(f.ctx, &types.MsgAttestRwu{
			NodeAddress:  node.Address,
			Attestations: attestations(1),
		})
		require.NoError(t, err)
	}
}

// A node type registered in x/network and fully licensed still attests nothing
// unless an attest message names it. Registration and payment are not
// authorization; the allowed tiers are compiled into the messages.
func TestUnconfiguredNodeTypeAttestsNothing(t *testing.T) {
	f := SetupTest(t)

	const bare = "bare"
	node := f.network.addNode(bare, networktypes.NodeActive)
	f.network.license(node.Operator, bare)

	_, err := f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.ErrorIs(t, err, types.ErrNodeTypeNotAllowed)

	_, err = f.msgServer.AttestRwu(f.ctx, &types.MsgAttestRwu{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.ErrorIs(t, err, types.ErrNodeTypeNotAllowed)
}

func TestAttestNodeStanding(t *testing.T) {
	f := SetupTest(t)

	unknown := simtestutil.CreateIncrementalAccounts(1)[0].String()
	_, err := f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  unknown,
		Attestations: attestations(1),
	})
	require.ErrorIs(t, err, types.ErrNodeNotFound)

	deactivated := f.addNode(nodeTypeTrust, networktypes.NodeDeactivated)
	_, err = f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  deactivated.Address,
		Attestations: attestations(1),
	})
	require.ErrorIs(t, err, types.ErrNodeNotActive)
}

// TestAttestQuotaReread pins the handler side of the quota contract: the
// ante increments the counters, the handler only re-reads them.
func TestAttestQuotaReread(t *testing.T) {
	f := SetupTest(t)
	node := f.addNode(nodeTypeTrust, networktypes.NodeActive)

	params, err := f.k.Params.Get(f.ctx)
	require.NoError(t, err)

	// An over-limit counter for today rejects the attestation.
	require.NoError(t, f.k.RwaCounters.Set(f.ctx, node.Address, types.ActivityCounter{
		LatestTime: f.ctx.BlockTime(),
		DailyCount: params.RwaDailyLimit + 1,
	}))
	_, err = f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.ErrorIs(t, err, types.ErrQuotaExceeded)

	// A stale over-limit counter from a previous day does not.
	require.NoError(t, f.k.RwaCounters.Set(f.ctx, node.Address, types.ActivityCounter{
		LatestTime: f.ctx.BlockTime().Add(-24 * time.Hour),
		DailyCount: params.RwaDailyLimit + 1,
	}))
	_, err = f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.NoError(t, err)

	// The RWA counter does not gate RWU attestations.
	require.NoError(t, f.k.RwaCounters.Set(f.ctx, node.Address, types.ActivityCounter{
		LatestTime: f.ctx.BlockTime(),
		DailyCount: params.RwaDailyLimit + 1,
	}))
	_, err = f.msgServer.AttestRwu(f.ctx, &types.MsgAttestRwu{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.NoError(t, err)
}

func TestUpdateParams(t *testing.T) {
	f := SetupTest(t)

	valid := types.Params{RwaDailyLimit: 7, RwuDailyLimit: 9}

	_, err := f.msgServer.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: simtestutil.CreateIncrementalAccounts(1)[0].String(),
		Params:    valid,
	})
	require.ErrorContains(t, err, "invalid authority")

	_, err = f.msgServer.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: f.govModAddr,
		Params:    types.Params{RwaDailyLimit: 0, RwuDailyLimit: 5},
	})
	require.ErrorContains(t, err, "rwa_daily_limit")

	_, err = f.msgServer.UpdateParams(f.ctx, &types.MsgUpdateParams{
		Authority: f.govModAddr,
		Params:    valid,
	})
	require.NoError(t, err)

	got, err := f.k.Params.Get(f.ctx)
	require.NoError(t, err)
	require.Equal(t, valid, got)
}

func TestValidateBasicBounds(t *testing.T) {
	node := simtestutil.CreateIncrementalAccounts(1)[0].String()

	tests := []struct {
		name      string
		msg       *types.MsgAttestRwa
		expErrMsg string
	}{
		{
			name: "valid",
			msg:  &types.MsgAttestRwa{NodeAddress: node, Attestations: attestations(100)},
		},
		{
			name:      "invalid signer",
			msg:       &types.MsgAttestRwa{NodeAddress: "not-bech32", Attestations: attestations(1)},
			expErrMsg: "invalid node address",
		},
		{
			name:      "empty attestations",
			msg:       &types.MsgAttestRwa{NodeAddress: node},
			expErrMsg: "must not be empty",
		},
		{
			name:      "too many attestations",
			msg:       &types.MsgAttestRwa{NodeAddress: node, Attestations: attestations(101)},
			expErrMsg: "exceeds max",
		},
		{
			name: "empty contract address",
			msg: &types.MsgAttestRwa{NodeAddress: node, Attestations: []types.ContractAttestation{
				{CurrentSupply: math.NewInt(1)},
			}},
			expErrMsg: "contract_address must not be empty",
		},
		{
			name: "nil supply",
			msg: &types.MsgAttestRwa{NodeAddress: node, Attestations: []types.ContractAttestation{
				{ContractAddress: "0xaa"},
			}},
			expErrMsg: "current_supply must be set",
		},
		{
			name: "negative supply",
			msg: &types.MsgAttestRwa{NodeAddress: node, Attestations: []types.ContractAttestation{
				{ContractAddress: "0xaa", CurrentSupply: math.NewInt(-1)},
			}},
			expErrMsg: "must not be negative",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.msg.ValidateBasic()
			if tc.expErrMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.expErrMsg)
		})
	}
}
