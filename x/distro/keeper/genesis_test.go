package keeper_test

import (
	"testing"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
	"github.com/stretchr/testify/require"
)

func TestGenesis(t *testing.T) {
	f := SetupTest(t)

	genesisState := &types.GenesisState{
		Params: types.DefaultParams(),
		CategoryClaimTotals: []types.CategoryClaimTotal{
			{Date: dateOf(1), Category: "type1", Total: "1500"},
			{Date: dateOf(1), Category: "type2", Total: "2000"},
		},
	}

	f.k.InitGenesis(f.ctx, genesisState)

	got := f.k.ExportGenesis(f.ctx)
	require.NotNil(t, got)
	require.ElementsMatch(t, genesisState.CategoryClaimTotals, got.CategoryClaimTotals)
}
