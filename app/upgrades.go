package app

import (
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
)

func (app *ChainApp) RegisterUpgradeHandlers() {
	app.registerV2UpgradeHandler()
	app.registerV3UpgradeHandler()

	upgradeInfo, err := app.UpgradeKeeper.ReadUpgradeInfoFromDisk()
	if err != nil {
		panic(err)
	}

	if app.UpgradeKeeper.IsSkipHeight(upgradeInfo.Height) {
		return
	}

	var storeUpgrades *storetypes.StoreUpgrades
	switch upgradeInfo.Name {
	case UpgradeNameV2:
		s := v2StoreUpgrades()
		storeUpgrades = &s
	case UpgradeNameV3:
		s := v3StoreUpgrades()
		storeUpgrades = &s
	}

	if storeUpgrades != nil {
		app.SetStoreLoader(upgradetypes.UpgradeStoreLoader(upgradeInfo.Height, storeUpgrades))
	}
}
