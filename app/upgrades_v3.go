package app

import (
	"context"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	licenses "github.com/webstack-sdk/webstack/x/licenses"
	licensestypes "github.com/webstack-sdk/webstack/x/licenses/types"
)

const (
	UpgradeNameV3       = "v3"
	LicensesModuleOwner = "tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"
)

func (app *ChainApp) registerV3UpgradeHandler() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeNameV3,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Info("Running v3 upgrade: adding licenses module")

			// Skip InitGenesis for the licenses module so we can seed params
			// directly — DefaultGenesis has an empty Owner, which fails validation.
			fromVM[licensestypes.ModuleName] = licenses.ConsensusVersion

			versionMap, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			sdkCtx.Logger().Info("Setting licenses module params", "owner", LicensesModuleOwner)
			if err := app.LicensesKeeper.SetParams(ctx, licensestypes.Params{Owner: LicensesModuleOwner}); err != nil {
				return nil, err
			}

			return versionMap, nil
		},
	)
}

func v3StoreUpgrades() storetypes.StoreUpgrades {
	return storetypes.StoreUpgrades{
		Added: []string{
			licensestypes.StoreKey,
		},
	}
}
