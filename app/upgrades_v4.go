package app

import (
	"context"

	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	distrotypes "github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

const UpgradeNameV4 = "v4"

func (app *ChainApp) registerV4UpgradeHandler() {
	app.UpgradeKeeper.SetUpgradeHandler(
		UpgradeNameV4,
		func(ctx context.Context, _ upgradetypes.Plan, fromVM module.VersionMap) (module.VersionMap, error) {
			sdkCtx := sdk.UnwrapSDKContext(ctx)
			sdkCtx.Logger().Info("Running v4 upgrade: decentralized distribution")

			versionMap, err := app.ModuleManager.RunMigrations(ctx, app.Configurator(), fromVM)
			if err != nil {
				return nil, err
			}

			// Seed the decentralized-distribution params on the existing distro
			// module. The pre-v4 Params predate these fields, so they decode as
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
// v4, preserving the existing minting/halving params. Each field is only set
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
	// Per-type distribution config (with per-type consensus thresholds) replaced
	// the global license/stake thresholds. Seed the default single type when none
	// are configured.
	if len(params.DistributionTypes) == 0 {
		params.DistributionTypes = distrotypes.DefaultDistributionTypes()
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

// v4 introduces no new module stores — the decentralized-distribution state
// reuses the existing distro store key — so there is no v4StoreUpgrades and no
// store-upgrades case in RegisterUpgradeHandlers.
