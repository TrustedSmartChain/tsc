package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"

	"cosmossdk.io/core/header"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

// setupV4_1 returns an app with genesis committed and a context for
// the next block. The commit matters: ApplyUpgrade reads the module version
// map that InitChainer stored, and without it RunMigrations would treat every
// module as new and re-run its InitGenesis. HeaderInfo is set as BaseApp sets
// it in PreBlocker, since the upgrade keeper records the done height from it.
func setupV4_1(t *testing.T, chainID string) (*ChainApp, sdk.Context) {
	t.Helper()

	blockTime := time.Unix(1_790_000_000, 0).UTC()
	chainApp := Setup(t, chainID, 9001)
	_, err := chainApp.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: chainApp.LastBlockHeight() + 1,
		Time:   blockTime.Add(-time.Minute),
	})
	require.NoError(t, err)
	_, err = chainApp.Commit()
	require.NoError(t, err)

	height := chainApp.LastBlockHeight() + 1
	ctx := chainApp.NewUncachedContext(false, cmtproto.Header{
		Height:  height,
		ChainID: chainID,
		Time:    blockTime,
	}).WithHeaderInfo(header.Info{Height: height, Time: blockTime, ChainID: chainID})
	return chainApp, ctx
}

// lastCommitHasWasmStore reports whether the latest commit includes a wasm
// store, i.e. whether the wasm store is part of the app hash.
func lastCommitHasWasmStore(t *testing.T, chainApp *ChainApp) bool {
	t.Helper()
	type commitInfoGetter interface {
		GetCommitInfo(int64) (*storetypes.CommitInfo, error)
	}
	cms, ok := chainApp.CommitMultiStore().(commitInfoGetter)
	require.True(t, ok)
	info, err := cms.GetCommitInfo(chainApp.LastBlockHeight())
	require.NoError(t, err)
	for _, si := range info.StoreInfos {
		if si.Name == wasmStoreKey {
			return true
		}
	}
	return false
}

// The binary must keep hashing the wasm store until the upgrade, or a node
// that switches to it early computes a different app hash than the network.
func TestWasmStoreStaysMounted(t *testing.T) {
	chainApp, _ := setupV4_1(t, "wasm-mount-test")

	require.NotNil(t, chainApp.GetKey(wasmStoreKey))
	require.True(t, lastCommitHasWasmStore(t, chainApp))

	_, wired := chainApp.ModuleManager.Modules[wasmStoreKey]
	require.False(t, wired, "x/wasm must not be in the module manager")
}

func TestV4_1UpgradeClearsWasmStore(t *testing.T) {
	chainApp, ctx := setupV4_1(t, "wasm-upgrade-test")

	// What x/wasm left in its store on a pre-upgrade chain.
	store := ctx.KVStore(chainApp.GetKey(wasmStoreKey))
	for _, k := range []string{"params", "seq_code", "seq_instance"} {
		store.Set([]byte(k), []byte{1})
	}

	// The version map a pre-upgrade chain carries still lists x/wasm; the
	// handler must run cleanly over it.
	vm, err := chainApp.UpgradeKeeper.GetModuleVersionMap(ctx)
	require.NoError(t, err)
	vm[wasmStoreKey] = 4
	require.NoError(t, chainApp.UpgradeKeeper.SetModuleVersionMap(ctx, vm))

	require.NoError(t, chainApp.UpgradeKeeper.ApplyUpgrade(ctx, upgradetypes.Plan{
		Name:   UpgradeNameV4_1,
		Height: ctx.BlockHeight(),
	}))

	iter := store.Iterator(nil, nil)
	require.False(t, iter.Valid(), "wasm store still has keys after the upgrade")
	require.NoError(t, iter.Close())

	done, err := chainApp.UpgradeKeeper.GetDoneHeight(ctx, UpgradeNameV4_1)
	require.NoError(t, err)
	require.Equal(t, ctx.BlockHeight(), done)
}

// TestV4_1ForkUpgrade drives the upgrade through the fork path: no plan in
// state, applied by height from PreBlocker.
func TestV4_1ForkUpgrade(t *testing.T) {
	chainApp, ctx := setupV4_1(t, "wasm-fork-test")
	store := ctx.KVStore(chainApp.GetKey(wasmStoreKey))
	store.Set([]byte("params"), []byte{1})

	var found bool
	for _, fu := range forkUpgrades() {
		if fu.Name == UpgradeNameV4_1 {
			found = true
			require.Equal(t, ForkHeightV4_1, fu.Height)
		}
	}
	require.True(t, found, "v4.1 missing from forkUpgrades()")

	fu := forkUpgrade{Name: UpgradeNameV4_1, Height: ctx.BlockHeight()}

	// Before the fork height: nothing happens.
	chainApp.applyForkUpgrade(ctx.WithBlockHeight(fu.Height-1), fu)
	require.True(t, store.Has([]byte("params")))
	done, err := chainApp.UpgradeKeeper.GetDoneHeight(ctx, UpgradeNameV4_1)
	require.NoError(t, err)
	require.Zero(t, done)

	// Fork height: the store is cleared and the plan is recorded as applied.
	chainApp.applyForkUpgrade(ctx, fu)
	require.False(t, store.Has([]byte("params")))
	done, err = chainApp.UpgradeKeeper.GetDoneHeight(ctx, UpgradeNameV4_1)
	require.NoError(t, err)
	require.Equal(t, fu.Height, done)

	// Idempotent: a second pass at the same height is a no-op, not a panic.
	require.NotPanics(t, func() { chainApp.applyForkUpgrade(ctx, fu) })
}

// The wasm module account outlives the module, so it must stay blocked.
func TestRetiredWasmAccountStaysBlocked(t *testing.T) {
	_, inPerms := GetMaccPerms()[wasmStoreKey]
	require.False(t, inPerms)
	require.True(t, BlockedAddresses()[authtypes.NewModuleAddress(wasmStoreKey).String()])
}
