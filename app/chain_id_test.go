package app

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/client/flags"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"

	evmserverconfig "github.com/cosmos/evm/server/config"
	srvflags "github.com/cosmos/evm/server/flags"
)

func TestEVMChainIDFromChainID(t *testing.T) {
	cases := []struct {
		chainID string
		want    uint64
		ok      bool
	}{
		{"tsc_8878788-1", 8878788, true},
		{"tsc_8878788-2", 8878788, true},
		{"localchain_9000-2", 9000, true},
		{"chain-test", 0, false},
		{"tsc_0-1", 0, false},
		{"tsc_8878788", 0, false},
		{"TSC_8878788-1", 0, false},
		{"", 0, false},
	}
	for _, tc := range cases {
		got, ok := EVMChainIDFromChainID(tc.chainID)
		require.Equal(t, tc.ok, ok, tc.chainID)
		require.Equal(t, tc.want, got, tc.chainID)
	}
}

func TestChainIDFromOpts(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "config"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "config", "genesis.json"),
		[]byte(`{"app_name":"tscd","chain_id":"tsc_8878788-1","app_state":{}}`),
		0o644,
	))

	// genesis is the source
	id, err := ChainIDFromOpts(simtestutil.AppOptionsMap{flags.FlagHome: home})
	require.NoError(t, err)
	require.Equal(t, "tsc_8878788-1", id)

	// a flag that agrees with genesis is fine
	id, err = ChainIDFromOpts(simtestutil.AppOptionsMap{flags.FlagHome: home, flags.FlagChainID: "tsc_8878788-1"})
	require.NoError(t, err)
	require.Equal(t, "tsc_8878788-1", id)

	// a flag that contradicts genesis is an error, not an override
	_, err = ChainIDFromOpts(simtestutil.AppOptionsMap{flags.FlagHome: home, flags.FlagChainID: "other_1-1"})
	require.ErrorContains(t, err, "contradicts")

	// genesis_file override, absolute path
	other := filepath.Join(t.TempDir(), "g.json")
	require.NoError(t, os.WriteFile(other, []byte(`{"chain_id":"tsc_87878-1"}`), 0o644))
	id, err = ChainIDFromOpts(simtestutil.AppOptionsMap{flags.FlagHome: home, "genesis_file": other})
	require.NoError(t, err)
	require.Equal(t, "tsc_87878-1", id)

	// no genesis: the flag alone is honoured (test helpers)
	id, err = ChainIDFromOpts(simtestutil.AppOptionsMap{flags.FlagHome: t.TempDir(), flags.FlagChainID: "chain-test"})
	require.NoError(t, err)
	require.Equal(t, "chain-test", id)

	// nothing to resolve
	_, err = ChainIDFromOpts(simtestutil.AppOptionsMap{flags.FlagHome: t.TempDir()})
	require.Error(t, err)
}

func TestResolveEVMChainIDGenesisAuthoritative(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "config"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(home, "config", "genesis.json"), []byte(`{"chain_id":"tsc_8878788-1"}`), 0o644))

	// genesis alone
	id, err := ResolveEVMChainID(simtestutil.AppOptionsMap{flags.FlagHome: home})
	require.NoError(t, err)
	require.Equal(t, uint64(8878788), id)

	// a contradicting --chain-id is an error even when app.toml has a value to fall back to
	_, err = ResolveEVMChainID(simtestutil.AppOptionsMap{
		flags.FlagHome: home, flags.FlagChainID: "tsc_9000-1", srvflags.EVMChainID: uint64(262144),
	})
	require.ErrorContains(t, err, "contradicts")

	// an unreadable genesis is an error, not a silent fallback
	require.NoError(t, os.WriteFile(filepath.Join(home, "config", "genesis.json"), []byte(`not json`), 0o644))
	_, err = ResolveEVMChainID(simtestutil.AppOptionsMap{flags.FlagHome: home, srvflags.EVMChainID: uint64(9001)})
	require.Error(t, err)
}

func TestResolveEVMChainID(t *testing.T) {
	const dflt = uint64(evmserverconfig.DefaultEVMChainID)

	cases := []struct {
		name       string
		chainID    string
		configured uint64
		want       uint64
		wantErr    bool
	}{
		{"derived from suffix", "tsc_8878788-1", 0, 8878788, false},
		{"derived, matching config", "tsc_8878788-1", 8878788, 8878788, false},
		{"derived, default config ignored", "tsc_8878788-1", dflt, 8878788, false},
		{"derived, contradicting config", "tsc_8878788-1", 9001, 0, true},
		{"no suffix, configured", "chain-test", 9001, 9001, false},
		{"no suffix, unconfigured", "chain-test", 0, dflt, false},
		{"no chain id at all", "", 0, dflt, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			opts := simtestutil.AppOptionsMap{flags.FlagHome: t.TempDir()}
			if tc.chainID != "" {
				opts[flags.FlagChainID] = tc.chainID
			}
			if tc.configured != 0 {
				opts[srvflags.EVMChainID] = tc.configured
			}
			got, err := ResolveEVMChainID(opts)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, tc.want, got)
		})
	}
}
