package app

import (
	"context"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
)

const (
	UpgradeNameV4 = "v4"

	// ForkHeightV4 is the height at which the v4 upgrade applies WITHOUT a
	// plan in state: mainnet halted at 693954 on the Cosmos Labs team's
	// recommendation (no MsgSoftwareUpgrade possible), so the new binary
	// hardcodes activation at the first block after the halt.
	//
	// When 0, fork activation is disabled and v4 can only apply through a
	// normal on-chain plan — which is what local nets, tests, and the upgrade
	// harness use.
	ForkHeightV4 int64 = 693956
)

// registerV4UpgradeHandler registers the v4 handler: the cosmos/evm
// v0.5.1→v0.6.2 security upgrade (plus the private August 2026 hotfix pinned
// in go.mod). The dependency stack is otherwise unchanged — same cosmos-sdk,
// cometbft, ibc-go, wasmd and geth fork — and vm/erc20/feemarket stay at
// consensus version 1, so RunMigrations has nothing to migrate. It still runs
// to write the module version map and record the applied plan, which is what
// `tscd q upgrade applied v4` verifies after the restart. No StoreUpgrades,
// no store loader.
func (app *ChainApp) registerV4UpgradeHandler() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeNameV4,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Info("Running v4 upgrade: cosmos/evm v0.6.2 security migration")
			return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		},
	)
}

// forkUpgrade is an upgrade scheduled by code rather than by an on-chain
// plan: validators coordinate a halt, swap binaries, and the new binary
// applies the upgrade at a hardcoded height.
type forkUpgrade struct {
	Name   string
	Height int64
}

// forkUpgrades lists every fork-style upgrade this binary knows about. A zero
// height disables fork activation for that entry, leaving the plan-in-state
// path (local nets, tests, the upgrade harness) as the only way to apply it.
func forkUpgrades() []forkUpgrade {
	return []forkUpgrade{
		{Name: UpgradeNameV4, Height: ForkHeightV4},
		{Name: UpgradeNameV4_1, Height: ForkHeightV4_1},
	}
}

// applyForkUpgrades hard-applies scheduled-by-code upgrades at their fork
// height. Called from PreBlocker before the upgrade module's own PreBlock, so
// a fork upgrade behaves exactly like a plan-in-state upgrade from the
// handler's point of view: ApplyUpgrade runs the handler, records the applied
// plan (AppliedPlan("v4") answers with the fork height), and bumps the module
// version map. Deterministic across validators because height and code are
// identical everywhere.
func (app *ChainApp) applyForkUpgrades(ctx sdk.Context) {
	for _, fu := range forkUpgrades() {
		app.applyForkUpgrade(ctx, fu)
	}
}

// applyForkUpgrade applies a single fork upgrade if ctx is at its height and
// it has not already been applied.
func (app *ChainApp) applyForkUpgrade(ctx sdk.Context, fu forkUpgrade) {
	if fu.Height == 0 || ctx.BlockHeight() != fu.Height {
		return
	}
	if done, _ := app.UpgradeKeeper.GetDoneHeight(ctx, fu.Name); done != 0 {
		return
	}
	if err := app.UpgradeKeeper.ApplyUpgrade(ctx, upgradetypes.Plan{
		Name:   fu.Name,
		Height: fu.Height,
	}); err != nil {
		panic(err)
	}
}
