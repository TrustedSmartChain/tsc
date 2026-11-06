package app

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"
)

const (
	UpgradeNameV4_2 = "v4.2"

	// ForkHeightV4_2 is the height at which v4.2 applies WITHOUT a plan in
	// state, like v4 and v4.1.
	//
	// Unlike v4.1, nodes must NOT run this binary before the fork height. Its
	// x/license and x/network read the v2 denormalized counts, which do not
	// exist until the migrations below run: before then every operator reads
	// as unlicensed, and gasless txs the v4.1 binary admits would be rejected.
	// Every node has to switch at the same block: set halt-height to
	// ForkHeightV4_2 in app.toml (BaseApp refuses to execute that block, so
	// the last committed height is ForkHeightV4_2-1), then restart on this
	// binary, which applies the upgrade in PreBlocker at ForkHeightV4_2.
	//
	// While 0, fork activation is disabled and v4.2 applies only through an
	// on-chain plan named UpgradeNameV4_2, which is what local nets, tests,
	// and the upgrade harness use.
	ForkHeightV4_2 int64 = 0

	// v4_2MaxGaslessGas is the network module's max_gasless_gas after v4.2.
	// A gasless MsgActivateNode costs ~60.5k gas plus ~47k per tx (nodelabs
	// localnet measurement), so the previous 300k cap admitted only 4
	// activations per tx against max_gasless_msgs = 10. 700k fits 10 (~652k)
	// with headroom. The cap only bounds one tx's share of a block; gasless
	// spam is bounded by the license-derived admission quotas.
	v4_2MaxGaslessGas uint64 = 700_000

	// v4_2SignedBlocksWindow is x/slashing's signed_blocks_window after v4.2.
	// Mainnet moves from ~30s to ~10s blocks alongside v4.2 (timeout_commit,
	// a node config change), which would cut the old 1,000-block window from
	// ~8.5h to ~2.8h and the downtime a validator survives from ~4.3h to
	// ~1.4h at min_signed_per_window = 0.5. 10,000 is the common production
	// value (Cosmos Hub): ~28h at 10s blocks, ~14h of tolerated downtime.
	v4_2SignedBlocksWindow int64 = 10_000

	// legacyGaslessKindAuthorize is the GaslessCounters kind gasless
	// MsgAuthorizeActivationKey used before nodelabs v0.3.0-alpha.2 made it a
	// paid msg. Nothing reads these counters any more.
	legacyGaslessKindAuthorize = "authorize"

	// mintStoreKey is x/mint's store key, spelled out because v4.2 removes the
	// module from the app.
	mintStoreKey = "mint"
)

// registerV4_2UpgradeHandler registers the v4.2 handler.
//
// v4.2 ships, and the handler migrates state for, the following:
//
//   - nodelabs v0.3.0: MsgActivateNode's gas no longer grows with fleet size.
//     RunMigrations runs the x/license and x/network v1->v2 migrations:
//     x/license backfills ActiveLicenseCounts from its holder index, and
//     x/network replaces the per-node RecentNodeActivity index (prefix 13)
//     with RecentNodeCounts windows built from the node records. Both modules
//     keep their store keys, so there are no store upgrades. max_gasless_gas
//     is raised to v4_2MaxGaslessGas so a gasless tx fits max_gasless_msgs
//     activations.
//   - nodelabs v0.3.0-alpha.2: MsgAuthorizeActivationKey always pays a fee
//     (off the gasless allowlist). Its leftover daily admission counters are
//     deleted (clearLegacyAuthorizeCounters).
//   - Fixed 21,000,000 TSC supply: x/distro's max_supply param is replaced by
//     the compiled-in types.MaxSupply (x/distro v1->v2 migration, run by
//     RunMigrations, re-writes its params without the field), and x/mint is
//     removed from the app: its store is deleted (v4_2StoreUpgrades) and its
//     module account stays blocked (retiredModuleAccounts).
//   - 10s blocks: x/slashing's signed_blocks_window goes to
//     v4_2SignedBlocksWindow, and every validator's missed-block record is
//     reset to match the new window (raiseSignedBlocksWindow). The block time
//     itself is timeout_commit in each validator's config.toml, not state.
//
// It also ships changes that need no state migration:
//
//   - Unordered transactions (authkeeper.WithUnorderedTransactions in
//     app.go). Their nonces live under a new prefix of the existing x/auth
//     store; x/auth's PreBlocker prunes them.
//   - cosmos/evm v0.6.4 security fixes (GHSA-p8x8, GHSA-c6xv, GHSA-wm4f). No
//     consensus version changes. GHSA-7xxp (EIP-712) does not reach TSC: its
//     ante router rejects the Web3Tx extension.
//   - Chain ID derivation from genesis (app/chain_id.go). Mainnet still
//     derives EVM chain ID 87878.
//
// All of it changes execution, so every node must switch at the same block
// (see ForkHeightV4_2).
func (app *ChainApp) registerV4_2UpgradeHandler() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeNameV4_2,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Info("Running v4.2 upgrade")

			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			if err := app.raiseMaxGaslessGas(ctx, v4_2MaxGaslessGas); err != nil {
				return nil, err
			}

			if err := app.clearLegacyAuthorizeCounters(ctx); err != nil {
				return nil, err
			}

			if err := app.raiseSignedBlocksWindow(ctx, v4_2SignedBlocksWindow); err != nil {
				return nil, err
			}

			return vm, nil
		},
	)
}

// raiseMaxGaslessGas sets the network module's max_gasless_gas to at least
// floor and leaves every other param untouched. A cap already at or above
// floor is kept: the upgrade never tightens a value governance chose.
func (app *ChainApp) raiseMaxGaslessGas(ctx context.Context, floor uint64) error {
	params, err := app.NetworkKeeper.GetParams(ctx)
	if err != nil {
		return fmt.Errorf("read network params: %w", err)
	}
	if params.MaxGaslessGas >= floor {
		return nil
	}

	before := params.MaxGaslessGas
	params.MaxGaslessGas = floor
	if err := params.Validate(); err != nil {
		return fmt.Errorf("network params after raising max_gasless_gas: %w", err)
	}
	if err := app.NetworkKeeper.Params.Set(ctx, params); err != nil {
		return err
	}

	sdk.UnwrapSDKContext(ctx).Logger().Info("Raised max_gasless_gas", "before", before, "after", floor)
	return nil
}

// clearLegacyAuthorizeCounters deletes x/network's gasless admission counters
// of kind "authorize", one per operator that authorized a key while
// MsgAuthorizeActivationKey was gasless.
func (app *ChainApp) clearLegacyAuthorizeCounters(ctx context.Context) error {
	rng := collections.NewPrefixedTripleRange[string, string, string](legacyGaslessKindAuthorize)
	keys, err := app.NetworkKeeper.GaslessCounters.Iterate(ctx, rng)
	if err != nil {
		return err
	}
	n, err := keys.Keys()
	if err != nil {
		return err
	}
	if err := app.NetworkKeeper.GaslessCounters.Clear(ctx, rng); err != nil {
		return err
	}
	sdk.UnwrapSDKContext(ctx).Logger().Info("Cleared legacy authorize gasless counters", "count", len(n))
	return nil
}

// raiseSignedBlocksWindow sets x/slashing's signed_blocks_window to at least
// floor and leaves every other slashing param untouched; a larger window is
// kept. When it changes, every validator's missed-block bitmap and counter are
// reset, as the SDK does on jailing: the bitmap is indexed by
// IndexOffset % window, so records kept under the old window would otherwise
// linger at the wrong positions for a full new window.
func (app *ChainApp) raiseSignedBlocksWindow(ctx context.Context, floor int64) error {
	params, err := app.SlashingKeeper.GetParams(ctx)
	if err != nil {
		return fmt.Errorf("read slashing params: %w", err)
	}
	if params.SignedBlocksWindow >= floor {
		return nil
	}

	before := params.SignedBlocksWindow
	params.SignedBlocksWindow = floor
	if err := params.Validate(); err != nil {
		return fmt.Errorf("slashing params after raising signed_blocks_window: %w", err)
	}
	if err := app.SlashingKeeper.SetParams(ctx, params); err != nil {
		return err
	}

	// Collect first: the signing infos are rewritten below.
	type entry struct {
		addr sdk.ConsAddress
		info slashingtypes.ValidatorSigningInfo
	}
	var infos []entry
	if err := app.SlashingKeeper.IterateValidatorSigningInfos(ctx, func(addr sdk.ConsAddress, info slashingtypes.ValidatorSigningInfo) bool {
		infos = append(infos, entry{addr, info})
		return false
	}); err != nil {
		return err
	}
	for _, e := range infos {
		e.info.MissedBlocksCounter = 0
		e.info.IndexOffset = 0
		if err := app.SlashingKeeper.DeleteMissedBlockBitmap(ctx, e.addr); err != nil {
			return err
		}
		if err := app.SlashingKeeper.SetValidatorSigningInfo(ctx, e.addr, e.info); err != nil {
			return err
		}
	}

	sdk.UnwrapSDKContext(ctx).Logger().Info("Raised signed_blocks_window", "before", before, "after", floor, "signing_infos_reset", len(infos))
	return nil
}

// v4_2StoreUpgrades deletes x/mint's store. The supply is fixed at
// types.MaxSupply and x/distro is its only issuer, so v4.2 removes x/mint (it
// ran at zero inflation, but governance could have turned it on).
//
// Deletion is disk cleanup: the app hash covers only mounted stores, and this
// binary no longer mounts "mint", so consensus does not depend on the loader
// running.
func v4_2StoreUpgrades() storetypes.StoreUpgrades {
	return storetypes.StoreUpgrades{Deleted: []string{mintStoreKey}}
}
