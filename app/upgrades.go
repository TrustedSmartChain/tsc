package app

import (
	"context"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/ethereum/go-ethereum/common"

	wasmtypes "github.com/CosmWasm/wasmd/x/wasm/types"
	lockupprecompile "github.com/TrustedSmartChain/tsc/precompiles/lockup"
	lockuptypes "github.com/TrustedSmartChain/tsc/x/lockup/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	epochstypes "github.com/cosmos/cosmos-sdk/x/epochs/types"
	"github.com/cosmos/cosmos-sdk/x/group"
)

const UpgradeName = "v2"

func (app *ChainApp) RegisterUpgradeHandlers() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeName,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)

			sdkCtx.Logger().Info("Setting denom metadata for upgrade", "denom", BaseDenom)
			app.BankKeeper.SetDenomMetaData(ctx, banktypes.Metadata{
				Description: "The native staking token of Trusted Smart Chain",
				DenomUnits: []*banktypes.DenomUnit{
					{
						Denom:    BaseDenom,
						Exponent: 0,
						Aliases:  []string{"atsc"},
					},
					{
						Denom:    DisplayDenom,
						Exponent: uint32(BaseDenomUnit),
						Aliases:  nil,
					},
				},
				Base:    BaseDenom,
				Display: DisplayDenom,
				Name:    "Trusted Smart Chain",
				Symbol:  DisplayDenom,
				URI:     "",
				URIHash: "",
			})

			sdkCtx.Logger().Info("Initializing EVM coin info from denom metadata")
			if err := app.EVMKeeper.InitEvmCoinInfo(sdkCtx); err != nil {
				return nil, err
			}
			sdkCtx.Logger().Info("EVM coin info initialized successfully")

			// Enable the lockup precompile for existing chains
			sdkCtx.Logger().Info("Enabling lockup precompile", "address", lockupprecompile.LockupPrecompileAddress)
			if err := app.EVMKeeper.EnableStaticPrecompiles(sdkCtx, common.HexToAddress(lockupprecompile.LockupPrecompileAddress)); err != nil {
				return nil, err
			}
			sdkCtx.Logger().Info("Lockup precompile enabled successfully")

			return app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
		},
	)

	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(err)
	}

	if upgradeInfo.Name == UpgradeName && !app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
		storeUpgrades := storetypes.StoreUpgrades{
			Added: []string{
				epochstypes.StoreKey,
				wasmtypes.StoreKey,
				lockuptypes.StoreKey,
			},
			Deleted: []string{
				crisistypes.StoreKey,
				group.StoreKey,
				"circuit",
				"feeibc",
				"capability",
				"tokenfactory",
				"packetforwardmiddleware",
				"08-wasm",
				"ratelimit",
			},
		}
		// configure store loader that checks if version == upgradeHeight and applies store upgrades
		app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, &storeUpgrades))
	}
}
