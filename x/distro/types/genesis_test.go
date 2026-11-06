package types_test

import (
	"encoding/json"
	"os"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v4/x/distro/types"
)

// TestMain sets TSC's bech32 prefixes, which the default minting and
// receiving addresses are encoded with.
func TestMain(m *testing.M) {
	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount("tsc", "tscpub")
	os.Exit(m.Run())
}

func TestGenesisState_Validate(t *testing.T) {
	tests := []struct {
		desc     string
		genState *types.GenesisState
		valid    bool
	}{
		{
			desc:     "default is valid",
			genState: types.DefaultGenesis(),
			valid:    true,
		},
		{
			desc:     "empty params are invalid",
			genState: &types.GenesisState{},
			valid:    false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.desc, func(t *testing.T) {
			err := tc.genState.Validate()
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

// The supply cap is the whitepaper's fixed 21,000,000 TSC, in aTSC (18
// decimals), and is not a param governance could change.
func TestMaxSupplyIsFixed(t *testing.T) {
	want, ok := math.NewIntFromString("21000000000000000000000000")
	require.True(t, ok)
	require.True(t, want.Equal(types.MaxSupply()))

	bz, err := json.Marshal(types.DefaultParams())
	require.NoError(t, err)
	require.NotContains(t, string(bz), "max_supply")
	require.NotContains(t, string(bz), "MaxSupply")
}
