package keeper_test

import (
	"testing"

	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/stretchr/testify/require"

	networktypes "github.com/webstack-sdk/webstack/x/network/types"

	"github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

func TestGenesisRoundTrip(t *testing.T) {
	f := SetupTest(t)
	node := f.addNode(nodeTypeTrust, networktypes.NodeActive)

	// Burn some quota so the export carries counters.
	require.NoError(t, f.k.CheckAndConsumeGaslessQuota(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	}))
	require.NoError(t, f.k.CheckAndConsumeGaslessQuota(f.ctx, &types.MsgAttestRwu{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	}))

	exported := f.k.ExportGenesis(f.ctx)
	require.NoError(t, exported.Validate())
	require.Len(t, exported.RwaCounters, 1)
	require.Len(t, exported.RwuCounters, 1)

	g := SetupTest(t)
	require.NoError(t, g.k.InitGenesis(g.ctx, exported))
	require.Equal(t, exported, g.k.ExportGenesis(g.ctx))
}

func TestGenesisValidate(t *testing.T) {
	node := simtestutil.CreateIncrementalAccounts(1)[0].String()
	valid := types.AttestationCounter{
		NodeAddress: node,
		Counter:     types.ActivityCounter{LatestTime: fixtureBlockTime, DailyCount: 1},
	}

	tests := []struct {
		name      string
		mutate    func(*types.GenesisState)
		expErrMsg string
	}{
		{
			name:   "valid",
			mutate: func(*types.GenesisState) {},
		},
		{
			name: "invalid params",
			mutate: func(gs *types.GenesisState) {
				gs.Params.RwuDailyLimit = 0
			},
			expErrMsg: "rwu_daily_limit",
		},
		{
			name: "duplicate counter",
			mutate: func(gs *types.GenesisState) {
				gs.RwaCounters = append(gs.RwaCounters, valid)
			},
			expErrMsg: "duplicate counter",
		},
		{
			name: "invalid node address",
			mutate: func(gs *types.GenesisState) {
				gs.RwuCounters[0].NodeAddress = "not-bech32"
			},
			expErrMsg: "invalid node address",
		},
		{
			name: "zero latest_time",
			mutate: func(gs *types.GenesisState) {
				gs.RwaCounters[0].Counter.LatestTime = types.ActivityCounter{}.LatestTime
			},
			expErrMsg: "latest_time must be set",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gs := types.GenesisState{
				Params:      types.DefaultParams(),
				RwaCounters: []types.AttestationCounter{valid},
				RwuCounters: []types.AttestationCounter{valid},
			}
			tc.mutate(&gs)
			err := gs.Validate()
			if tc.expErrMsg == "" {
				require.NoError(t, err)
				return
			}
			require.ErrorContains(t, err, tc.expErrMsg)
		})
	}
}
