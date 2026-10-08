package app

import (
	"context"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

const (
	UpgradeNameV4_1 = "v4.1"

	// wasmStoreKey is x/wasm's store key, spelled out because the module is
	// no longer a dependency.
	wasmStoreKey = "wasm"

	// ForkHeightV4_1 is the height at which this upgrade applies WITHOUT a
	// plan in state. Until then this binary computes the same app hash as the
	// pre-removal binary (see registerV4_1UpgradeHandler), so nodes can switch
	// to it at any time before ForkHeightV4_1. At ForkHeightV4_1 the handler
	// runs in PreBlocker and every node's state changes together.
	//
	// While 0, fork activation is disabled and the upgrade applies only
	// through an on-chain plan named UpgradeNameV4_1, which is what local
	// nets, tests, and the upgrade harness use.
	ForkHeightV4_1 int64 = 817700
)

// registerV4_1UpgradeHandler registers the v4.1 handler, which removes CosmWasm.
//
// x/wasm was added at v2 with wasmd's default params but never used: mainnet
// has no stored code, no contracts, no wasm IBC channels and an empty wasm
// module account. Removing it drops wasmd/wasmvm (and their open advisories)
// from the binary instead of tracking every CosmWasm security release.
//
// This binary does not run x/wasm, but it keeps the wasm store mounted. The
// app hash commits to every mounted store, and x/wasm has no begin/end
// blockers, so an untouched wasm store hashes the same as under the old
// binary. That lets nodes switch binaries before the upgrade height instead
// of all at one block. The exception is a block containing a wasm tx (a wasm
// msg, or an IBC handshake on a wasm port): the old binary executes it and
// this one cannot decode it, so old and new nodes would diverge on it. No
// such tx has ever been sent on mainnet.
//
// At the upgrade height the handler deletes every key in the wasm store. The
// store itself stays mounted, empty: unmounting it would need every node to
// restart at exactly that height. Two other harmless remnants stay in state:
// x/wasm's entry in the upgrade module's version map, and the wasm module
// account, which is kept blocked (retiredModuleAccounts).
func (app *ChainApp) registerV4_1UpgradeHandler() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeNameV4_1,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Info("Running v4.1 upgrade: removing CosmWasm")

			deleted := clearKVStore(sdkCtx.KVStore(app.GetKey(wasmStoreKey)))
			sdkCtx.Logger().Info("Cleared the wasm store", "keys_deleted", deleted)

			return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		},
	)
}

// clearKVStore deletes every key in store and returns how many it deleted.
// Keys are collected before deleting, since mutating a store while iterating
// it is not safe.
func clearKVStore(store storetypes.KVStore) int {
	var keys [][]byte
	iter := store.Iterator(nil, nil)
	for ; iter.Valid(); iter.Next() {
		keys = append(keys, append([]byte(nil), iter.Key()...))
	}
	iter.Close()

	for _, key := range keys {
		store.Delete(key)
	}
	return len(keys)
}
