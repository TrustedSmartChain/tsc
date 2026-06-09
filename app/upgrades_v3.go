package app

import (
	"context"

	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"
	licenses "github.com/webstack-sdk/webstack/x/licenses"
	licensestypes "github.com/webstack-sdk/webstack/x/licenses/types"

	distrotypes "github.com/TrustedSmartChain/tsc/v3/x/distro/types"
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

			// Seed the decentralized-distribution params on the existing distro
			// module. The pre-v3 Params predate these fields, so they decode as
			// empty/zero; without defaults the epoch tally hook and submit handler
			// would fail (e.g. GetEpochInfo("")).
			sdkCtx.Logger().Info("Seeding distro decentralized-distribution params")
			if err := migrateDistroParams(ctx, app); err != nil {
				return nil, err
			}

			// Backfill the active-distribution index from any existing non-terminal
			// days so the epoch hook can find them. No-op on a fresh introduction
			// (the decentralized-distribution state starts empty).
			sdkCtx.Logger().Info("Rebuilding distro active-distribution index")
			if err := app.DistroKeeper.RebuildActiveDistributions(ctx); err != nil {
				return nil, err
			}

			return versionMap, nil
		},
	)
}

// migrateDistroParams backfills the decentralized-distribution params added in
// v3, preserving the existing minting/halving params. Each field is only set
// when empty/zero so the migration is idempotent and never clobbers a value an
// operator may already have configured.
func migrateDistroParams(ctx context.Context, app *ChainApp) error {
	params, err := app.DistroKeeper.Params.Get(ctx)
	if err != nil {
		return err
	}

	if params.DistributionLicenseTypeId == "" {
		params.DistributionLicenseTypeId = distrotypes.DefaultDistributionLicenseTypeID
	}
	if params.LicenseTallyThreshold == "" {
		params.LicenseTallyThreshold = distrotypes.DefaultLicenseTallyThreshold
	}
	if params.StakeTallyThreshold == "" {
		params.StakeTallyThreshold = distrotypes.DefaultStakeTallyThreshold
	}
	if params.EpochIdentifier == "" {
		params.EpochIdentifier = distrotypes.DefaultEpochIdentifier
	}
	if params.ReviewDelayDays == 0 {
		params.ReviewDelayDays = distrotypes.DefaultReviewDelayDays
	}
	if params.ChallengeBond == "" {
		params.ChallengeBond = distrotypes.DefaultChallengeBond
	}
	if params.VoteWindowDays == 0 {
		params.VoteWindowDays = distrotypes.DefaultVoteWindowDays
	}

	return app.DistroKeeper.Params.Set(ctx, params)
}

func v3StoreUpgrades() storetypes.StoreUpgrades {
	return storetypes.StoreUpgrades{
		Added: []string{
			licensestypes.StoreKey,
		},
	}
}
