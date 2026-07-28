package app

import (
	"context"

	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/ethereum/go-ethereum/common"
	licensetypes "github.com/webstack-sdk/webstack/x/license/types"
	networktypes "github.com/webstack-sdk/webstack/x/network/types"
	permissiontypes "github.com/webstack-sdk/webstack/x/permission/types"

	attestationtypes "github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

const (
	UpgradeNameV3 = "v3"
	// LicenseNamespaceOwner controls grants in the permission module's
	// "license" namespace: it may issue and revoke licenses directly and
	// delegate either permission to other addresses, scoped per license type.
	LicenseNamespaceOwner = "tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"
	// NetworkNamespaceOwner controls grants in the permission module's
	// "network" namespace (currently just wallet.create, the admin fallback
	// for creating operator accounts).
	NetworkNamespaceOwner = LicenseNamespaceOwner
)

// v3NetworkParams returns the network module parameters seeded at the v3
// upgrade. This seeding is load-bearing: the webstack defaults fail closed
// (license_types is empty, so no node could ever activate).
func v3NetworkParams() networktypes.Params {
	params := networktypes.DefaultParams()
	// The counted node license types, and the node types activations may
	// declare. The two vocabularies deliberately share identifiers.
	params.LicenseTypes = []string{attestationtypes.NodeTypeTrust, attestationtypes.NodeTypeNano}
	params.AllowedNodeTypes = []string{attestationtypes.NodeTypeTrust, attestationtypes.NodeTypeNano}
	// Deauthorizing an activation key costs 0.01 TSC on top of regular gas,
	// charged by the msg handler as a bank transfer to the fee collector.
	// The fee exists to bound disabled-key churn, not to be punitive: total
	// supply is capped at 21M TSC.
	params.DeauthorizeFee = sdk.NewCoins(sdk.NewCoin(BaseDenom, math.NewIntWithDecimal(1, 16)))
	return params
}

func (app *ChainApp) registerV3UpgradeHandler() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeNameV3,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Info("Running v3 upgrade: adding license, permission, network, and attestation modules")

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

			sdkCtx.Logger().Info("Setting network namespace owner", "owner", NetworkNamespaceOwner)
			if err := app.PermissionKeeper.Namespaces.Set(ctx, networktypes.ModuleName, permissiontypes.Namespace{
				Module: networktypes.ModuleName,
				Owner:  NetworkNamespaceOwner,
			}); err != nil {
				return nil, err
			}

			// RunMigrations initialized both modules from their fail-closed
			// defaults; seed the real TSC values on top.
			networkParams := v3NetworkParams()
			sdkCtx.Logger().Info("Seeding network params", "license_types", networkParams.LicenseTypes, "deauthorize_fee", networkParams.DeauthorizeFee.String())
			if err := app.NetworkKeeper.Params.Set(ctx, networkParams); err != nil {
				return nil, err
			}

			attestationParams := attestationtypes.DefaultParams()
			sdkCtx.Logger().Info("Seeding attestation params", "rwa_daily_limit", attestationParams.RwaDailyLimit, "rwu_daily_limit", attestationParams.RwuDailyLimit)
			if err := app.AttestationKeeper.Params.Set(ctx, attestationParams); err != nil {
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
			networktypes.StoreKey,
			attestationtypes.StoreKey,
		},
	}
}
