package keeper_test

import (
	"testing"

	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/stretchr/testify/require"

	networktypes "github.com/nodelabs-sdk/nodelabs/x/network/types"

	"github.com/TrustedSmartChain/tsc/v4/x/attestation/types"
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

	// And execute an attestation so the export carries a last-attested time.
	_, err := f.msgServer.AttestRwu(f.ctx, &types.MsgAttestRwu{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.NoError(t, err)

	exported := f.k.ExportGenesis(f.ctx)
	require.NoError(t, exported.Validate())
	require.Len(t, exported.RwaCounters, 1)
	require.Len(t, exported.RwuCounters, 1)
	require.Len(t, exported.NodeStats, 1)
	require.NotNil(t, exported.NodeStats[0].LastRwuTime)
	require.Nil(t, exported.NodeStats[0].LastRwaTime)

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
	lastRwa := fixtureBlockTime
	validStats := types.AttestationStats{NodeAddress: node, LastRwaTime: &lastRwa}

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
		{
			name: "duplicate node stats",
			mutate: func(gs *types.GenesisState) {
				gs.NodeStats = append(gs.NodeStats, validStats)
			},
			expErrMsg: "duplicate stats",
		},
		{
			name: "invalid node stats address",
			mutate: func(gs *types.GenesisState) {
				gs.NodeStats[0].NodeAddress = "not-bech32"
			},
			expErrMsg: "invalid node address",
		},
		{
			name: "stats with no times set",
			mutate: func(gs *types.GenesisState) {
				gs.NodeStats[0] = types.AttestationStats{NodeAddress: node}
			},
			expErrMsg: "at least one of last_rwa_time and last_rwu_time must be set",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gs := types.GenesisState{
				Params:      types.DefaultParams(),
				RwaCounters: []types.AttestationCounter{valid},
				RwuCounters: []types.AttestationCounter{valid},
				NodeStats:   []types.AttestationStats{validStats},
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
