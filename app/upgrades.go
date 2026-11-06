package app

import (
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
)

func (app *ChainApp) RegisterUpgradeHandlers() {
	app.registerV2UpgradeHandler()
	app.registerV3UpgradeHandler()
	app.registerV4UpgradeHandler()
	app.registerV4_1UpgradeHandler()
	app.registerV4_2UpgradeHandler()

	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(err)
	}

	if app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
		return
	}

	if height, storeUpgrades := pendingStoreUpgrades(upgradeInfo, ForkHeightV4_2); storeUpgrades != nil {
		app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(height, storeUpgrades))
	}
}

// pendingStoreUpgrades picks the store changes to stage at startup and the
// height they belong to. UpgradeStoreLoader applies them only when the last
// committed height is exactly one below that height, so every other start is
// a plain load.
//
// A fork upgrade has no plan, so upgrade-info.json does not name it: the file
// still holds whichever plan-based upgrade ran last (v3 on mainnet). A
// scheduled v4.2 fork therefore wins over the file unless the file names v4.2
// itself, in which case v4.2 arrived through a plan and the plan height
// applies.
func pendingStoreUpgrades(upgradeInfo upgradetypes.Plan, forkHeightV4_2 int64) (int64, *storetypes.StoreUpgrades) {
	if forkHeightV4_2 != 0 && upgradeInfo.Name != UpgradeNameV4_2 {
		s := v4_2StoreUpgrades()
		return forkHeightV4_2, &s
	}

	var s storetypes.StoreUpgrades
	switch upgradeInfo.Name {
	case UpgradeNameV2:
		s = v2StoreUpgrades()
	case UpgradeNameV3:
		s = v3StoreUpgrades()
	case UpgradeNameV4_2:
		s = v4_2StoreUpgrades()
	default:
		return 0, nil
	}
	return upgradeInfo.Height, &s
}
