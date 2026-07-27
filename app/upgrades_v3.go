package app

import (
	"context"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/ethereum/go-ethereum/common"
	licensetypes "github.com/webstack-sdk/webstack/x/license/types"
	permissiontypes "github.com/webstack-sdk/webstack/x/permission/types"
)

const (
	UpgradeNameV3 = "v3"
	// LicenseNamespaceOwner controls grants in the permission module's
	// "license" namespace: it may issue and revoke licenses directly and
	// delegate either permission to other addresses, scoped per license type.
	LicenseNamespaceOwner = "tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"
)

func (app *ChainApp) registerV3UpgradeHandler() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeNameV3,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Info("Running v3 upgrade: adding license and permission modules")

			// Both modules ship an empty DefaultGenesis that passes validation,
			// so RunMigrations can run their InitGenesis normally. Ownership is
			// seeded afterwards: the license namespace has no owner until one
			// is set here.
			versionMap, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			sdkCtx.Logger().Info("Setting license namespace owner", "owner", LicenseNamespaceOwner)
			if err := app.PermissionKeeper.Namespaces.Set(ctx, licensetypes.ModuleName, permissiontypes.Namespace{
				Module: licensetypes.ModuleName,
				Owner:  LicenseNamespaceOwner,
			}); err != nil {
				return nil, err
			}

			// Enable the license precompile for existing chains
			sdkCtx.Logger().Info("Enabling license precompile", "address", licensetypes.PrecompileAddress)
			if err := app.EVMKeeper.EnableStaticPrecompiles(sdkCtx, common.HexToAddress(licensetypes.PrecompileAddress)); err != nil {
				return nil, err
			}

			return versionMap, nil
		},
	)
}

func v3StoreUpgrades() storetypes.StoreUpgrades {
	return storetypes.StoreUpgrades{
		Added: []string{
			licensetypes.StoreKey,
			permissiontypes.StoreKey,
		},
	}
}
