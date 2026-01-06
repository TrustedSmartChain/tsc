package v2

import (
	"context"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"

	"github.com/cosmos/cosmos-sdk/types/module"
	crisistypes "github.com/cosmos/cosmos-sdk/x/crisis/types"
	epochstypes "github.com/cosmos/cosmos-sdk/x/epochs/types"
	"github.com/cosmos/cosmos-sdk/x/group"

	precisebanktypes "github.com/cosmos/evm/x/precisebank/types"

	"github.com/TrustedSmartChain/tsc/app/upgrades"
)

// Stores removed during upgrade from v0.50.x to v0.53.5:
// - crisistypes.StoreKey (crisis) - module removed in v0.53.x
// - circuittypes.StoreKey (circuit) - module removed from project
// - group.StoreKey (group) - module removed from project
// - capabilitytypes.StoreKey (capability) - replaced by ibc-go capability
// - ibcfeetypes.StoreKey (ibc-fee) - module removed from project
// - tokenfactorytypes.StoreKey (tokenfactory) - module removed from project
// - packetforwardtypes.StoreKey (packet-forward) - module removed from project
// - wasmlctypes.StoreKey (wasm-lightclient) - module removed from project
// - ratelimittypes.StoreKey (rate-limit) - module removed from project
//
// Stores kept (no change needed):
// - paramstypes.StoreKey (params) - still required in v0.53.x
// - icahosttypes.StoreKey (ica-host) - kept in upgrade
// - icacontrollertypes.StoreKey (ica-controller) - kept in upgrade
//
// Stores added:
// - precisebanktypes.StoreKey (precisebank) - new module from cosmos/evm
// - epochstypes.StoreKey (epochs) - new module from cosmos-sdk v0.53.x
//
// Optional modules NOT added (can be added later if needed):
// - protocolpooltypes.StoreKey (protocolpool) - optional in v0.53.x

// NewUpgrade constructor
func NewUpgrade(semver string) upgrades.Upgrade {
	return upgrades.Upgrade{
		UpgradeName:          semver,
		CreateUpgradeHandler: CreateUpgradeHandler,
		StoreUpgrades: storetypes.StoreUpgrades{
			Added: []string{
				precisebanktypes.StoreKey,
				epochstypes.StoreKey,
			},
			Deleted: []string{
				crisistypes.StoreKey,
				group.StoreKey,
				// The following modules were completely removed from the project,
				// so we use string literals for their store keys
				"circuit",
				"feeibc",
				"capability",
				"tokenfactory",
				"packetfowardmiddleware",
				"08-wasm",
				"ratelimit",
			},
		},
	}
}

func CreateUpgradeHandler(
	mm upgrades.ModuleManager,
	configurator module.Configurator,
	ak *upgrades.AppKeepers,
) upgradetypes.UpgradeHandler {
	return func(ctx context.Context, plan upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
		return mm.RunMigrations(ctx, configurator, fromVM)
	}
}
