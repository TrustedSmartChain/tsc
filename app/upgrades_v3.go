package app

import (
	"context"
	"fmt"

	"cosmossdk.io/collections"
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
	// "license" namespace: it may create license types, and grant or revoke the
	// issue and revoke permissions to other addresses, scoped per license type.
	//
	// Note that owning the namespace does not by itself confer issue or revoke —
	// x/license routes both through the grant table with no owner short-circuit,
	// so the owner needs explicit self-grants, seeded below.
	LicenseNamespaceOwner = "tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"
	// NetworkNamespaceOwner controls grants in the permission module's
	// "network" namespace (currently just wallet.create, the admin fallback
	// for creating operator accounts).
	NetworkNamespaceOwner = LicenseNamespaceOwner
)

// v3NodeLicenseSupply is the supply cap seeded for each node license type.
//
// These bound *lifetime issuance*, not concurrent holdings: x/license compares
// the cap against issued_count, and revoking a license never decrements that —
// it moves the license from active_count to revoked_count. Reissuing after a
// revocation therefore consumes another slot, and the caps can be reached with
// zero licenses currently active.
//
// Every id in attestationtypes.NodeLicenseTypes must appear here; seeding fails
// closed rather than defaulting a missing entry to uncapped.
var v3NodeLicenseSupply = map[string]math.Int{
	attestationtypes.LicenseTypeNodeTrust: math.NewInt(240_000),
	attestationtypes.LicenseTypeNodeNano:  math.NewInt(200_000),
}

// v3NetworkParams returns the network module parameters seeded at the v3
// upgrade. This seeding is load-bearing: the webstack defaults fail closed
// (license_types is empty, so no node could ever activate).
func v3NetworkParams() networktypes.Params {
	params := networktypes.DefaultParams()
	// Two distinct vocabularies, both sourced from x/attestation so neither can
	// drift from the mapping it enforces: license_types are the x/license SKU ids
	// counted toward an operator's activation limit, allowed_node_types are the
	// strings a node may declare for itself. Which SKU backs which node type is
	// attestationtypes.LicenseTypesForNodeType's business — the two lists are not
	// required to be equal, and are not.
	params.LicenseTypes = attestationtypes.NodeLicenseTypes()
	params.AllowedNodeTypes = attestationtypes.NodeTypes
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

			if err := app.seedV3NodeLicensing(ctx); err != nil {
				return nil, err
			}

			return versionMap, nil
		},
	)
}

// seedV3NodeLicensing registers the node license types and grants the license
// namespace owner the ability to issue and revoke them.
//
// Without this the upgrade lands inert. x/license's DefaultGenesis carries no
// license types, and MsgIssueLicense refuses any type that is not registered, so
// no license could be issued; with no license issued x/network's
// EnsureOperatorLicensed rejects every activation, and with no active node
// x/attestation has nothing to accept. The grants are equally load-bearing:
// x/license checks issue and revoke against the permission grant table with no
// owner short-circuit, so seeding the namespace owner alone would leave nobody
// able to issue.
//
// License types are registered before the grants that scope to them, matching
// the order MsgGrant would require — it validates a grant's scope against the
// license type registry.
func (app *ChainApp) seedV3NodeLicensing(ctx context.Context) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	licenseTypes := attestationtypes.NodeLicenseTypes()

	for _, id := range licenseTypes {
		// A missing cap is an error rather than an implicit zero: zero means
		// uncapped in x/license, so defaulting would silently seed unlimited
		// supply for a type someone added to the mapping without deciding one.
		maxSupply, ok := v3NodeLicenseSupply[id]
		if !ok {
			return fmt.Errorf("no max supply declared for node license type %q", id)
		}

		// Non-transferrable: a node license stays with the operator it was issued
		// to, so the activation limit it feeds cannot be traded away.
		sdkCtx.Logger().Info("Registering node license type", "id", id, "max_supply", maxSupply.String())
		if err := app.LicenseKeeper.LicenseTypes.Set(ctx, id, licensetypes.LicenseType{
			Id:            id,
			Transferrable: false,
			MaxSupply:     maxSupply,
			IssuedCount:   math.ZeroInt(),
			ActiveCount:   math.ZeroInt(),
			RevokedCount:  math.ZeroInt(),
		}); err != nil {
			return err
		}
	}

	for _, id := range licenseTypes {
		for _, permission := range []string{licensetypes.PermissionIssue, licensetypes.PermissionRevoke} {
			sdkCtx.Logger().Info("Granting license permission", "grantee", LicenseNamespaceOwner, "permission", permission, "scope", id)
			if err := app.PermissionKeeper.Grants.Set(ctx, collections.Join4(
				licensetypes.ModuleName, LicenseNamespaceOwner, permission, id,
			)); err != nil {
				return err
			}
		}
	}

	return nil
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
