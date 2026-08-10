package app

import (
	"context"

	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	"github.com/ethereum/go-ethereum/common"
	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
	networktypes "github.com/nodelabs-sdk/nodelabs/x/network/types"

	attestationtypes "github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

const (
	UpgradeNameV3 = "v3"
	// LicenseModuleOwner is the x/license owner parameter: it may grant any
	// license action to any address, itself included.
	//
	// Owning the module does not by itself confer any of those actions —
	// x/license routes type.create, issue and revoke through the grant table
	// with no owner short-circuit — and the upgrade seeds no grants at all.
	// Every action is granted by tx after the upgrade lands, so the catalog
	// operator is chosen live rather than fixed here.
	//
	// Ownership itself is seeded, and has to be: MsgGrantAccess is signed by
	// the module owner, and the only way to install an owner without one is
	// MsgUpdateParams, which is gov-gated. Seeding ownership is what makes the
	// tx-driven path reachable without a proposal.
	LicenseModuleOwner = "tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"
	// NetworkModuleOwner is the x/network owner parameter, controlling grants
	// of wallet.create (the admin fallback for creating operator accounts) and
	// nodetype.create.
	//
	// This is deliberately the same address as LicenseModuleOwner: one key
	// administers both modules at launch. Nothing in either module requires
	// it — a node type binds to any registered license type regardless of who
	// created it — so the two can be split later with MsgTransferOwnership,
	// and the catalog operator still only needs the two grants.
	NetworkModuleOwner = LicenseModuleOwner
)

// v3NetworkParams returns the network module parameters seeded at the v3
// upgrade.
//
// license_types and allowed_node_types are deliberately left at their empty
// defaults. The upgrade ships the machinery, not the catalog: which license SKU
// ids count toward an operator's activation limit, and which node types a node
// may declare for itself, are configured after the upgrade alongside the license
// types themselves. Both lists fail closed while empty — no node can activate —
// so nothing is silently permitted in the meantime.
func v3NetworkParams() networktypes.Params {
	params := networktypes.DefaultParams()
	// Deauthorizing an activation key costs 0.01 TSC on top of regular gas,
	// charged by the msg handler as a bank transfer to the fee collector.
	// The fee exists to bound disabled-key churn, not to be punitive: total
	// supply is capped at 21M TSC.
	params.DeauthorizeFee = sdk.NewCoins(sdk.NewCoin(BaseDenom, math.NewIntWithDecimal(1, 16)))
	// Ownership rides in the same parameter set as the rest, so it has to be
	// set here: the seed below writes params wholesale and would otherwise
	// clear an owner installed separately.
	params.Owner = NetworkModuleOwner
	return params
}

func (app *ChainApp) registerV3UpgradeHandler() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeNameV3,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Info("Running v3 upgrade: adding license, network, and attestation modules")

			// Both modules ship an empty DefaultGenesis that passes validation,
			// so RunMigrations can run their InitGenesis normally. Ownership is
			// seeded afterwards: each module's owner parameter is empty until
			// it is set here.
			versionMap, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			sdkCtx.Logger().Info("Setting license module owner", "owner", LicenseModuleOwner)
			if err := app.LicenseKeeper.Params.Set(ctx, licensetypes.Params{
				Owner: LicenseModuleOwner,
			}); err != nil {
				return nil, err
			}

			// Enable the license precompile for existing chains
			sdkCtx.Logger().Info("Enabling license precompile", "address", licensetypes.PrecompileAddress)
			if err := app.EVMKeeper.EnableStaticPrecompiles(sdkCtx, common.HexToAddress(licensetypes.PrecompileAddress)); err != nil {
				return nil, err
			}

			// RunMigrations initialized both modules from their fail-closed
			// defaults; seed the real TSC values on top. The network owner is
			// part of this parameter set (see v3NetworkParams).
			networkParams := v3NetworkParams()
			sdkCtx.Logger().Info("Seeding network params", "deauthorize_fee", networkParams.DeauthorizeFee.String(), "owner", networkParams.Owner)
			if err := app.NetworkKeeper.Params.Set(ctx, networkParams); err != nil {
				return nil, err
			}

			attestationParams := attestationtypes.DefaultParams()
			sdkCtx.Logger().Info("Seeding attestation params", "rwa_daily_limit", attestationParams.RwaDailyLimit, "rwu_daily_limit", attestationParams.RwuDailyLimit)
			if err := app.AttestationKeeper.Params.Set(ctx, attestationParams); err != nil {
				return nil, err
			}

			// No grants are seeded. Every license and network action —
			// type.create, nodetype.create, issue, revoke — is granted by tx
			// after the upgrade, by the module owners set above. The chain
			// therefore lands with no catalog and no way to build one until
			// those txs are sent, which is the intended posture: nothing is
			// activatable or attestable in the meantime.
			return versionMap, nil
		},
	)
}

func v3StoreUpgrades() storetypes.StoreUpgrades {
	return storetypes.StoreUpgrades{
		Added: []string{
			licensetypes.StoreKey,
			networktypes.StoreKey,
			attestationtypes.StoreKey,
		},
	}
}
