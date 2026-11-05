package app

import (
	"context"
	"fmt"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
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
)

// registerV4_2UpgradeHandler registers the v4.2 handler: the nodelabs
// v0.3.0 bump, which makes MsgActivateNode's gas independent of fleet size.
//
// RunMigrations runs the x/license and x/network v1->v2 migrations:
// x/license backfills ActiveLicenseCounts from its holder index, and
// x/network replaces the per-node RecentNodeActivity index (prefix 13) with
// RecentNodeCounts windows built from the node records. Both modules keep
// their store keys, so there are no store upgrades.
//
// It then raises max_gasless_gas to v4_2MaxGaslessGas.
func (app *ChainApp) registerV4_2UpgradeHandler() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeNameV4_2,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Info("Running v4.2 upgrade: nodelabs v0.3.0 license and network migrations")

			vm, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			if err := app.raiseMaxGaslessGas(ctx, v4_2MaxGaslessGas); err != nil {
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
