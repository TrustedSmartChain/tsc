package keeper

import (
	"bytes"
	"context"
	"strconv"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	epochstypes "github.com/cosmos/cosmos-sdk/x/epochs/types"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// EpochHooks wires the distro tally into the x/epochs lifecycle.
type EpochHooks struct {
	k Keeper
}

var _ epochstypes.EpochHooks = EpochHooks{}

// EpochHooks returns the epochs hooks implementation for this keeper.
func (k Keeper) EpochHooks() EpochHooks {
	return EpochHooks{k: k}
}

// BeforeEpochStart is a no-op for the distro module.
func (h EpochHooks) BeforeEpochStart(ctx context.Context, identifier string, epochNumber int64) error {
	return nil
}

// AfterEpochEnd advances the distribution lifecycle for every epoch up to and
// including the epoch that just ended:
//   - VOTING: tallied; on consensus moves to PENDING (start of the review delay).
//   - UNDER_REVIEW: re-tallied; on consensus the challenge bond is resolved and
//     it moves back to PENDING with the (possibly corrected) root.
//   - PENDING: auto-promoted to LIVE once the review delay has elapsed.
//
// Processing all such epochs (not only the just-ended one) tolerates nodes that
// submit their root a day late. Votes are retained (never pruned).
func (h EpochHooks) AfterEpochEnd(ctx context.Context, identifier string, epochNumber int64) error {
	params, err := h.k.Params.Get(ctx)
	if err != nil {
		return err
	}
	if identifier != params.EpochIdentifier {
		return nil
	}
	return h.k.advanceDistributions(ctx, epochNumber, params)
}

func (k Keeper) advanceDistributions(ctx context.Context, upTo int64, params types.Params) error {
	// Snapshot the candidate epochs first (ascending key order) so we don't
	// mutate the map while iterating it.
	var candidates []types.EpochDistribution
	if err := k.EpochDistributions.Walk(ctx, nil, func(epoch int64, ed types.EpochDistribution) (bool, error) {
		if epoch <= upTo {
			candidates = append(candidates, ed)
		}
		return false, nil
	}); err != nil {
		return err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	reviewDelay := int64(params.DistributionReviewDelay)

	for _, ed := range candidates {
		// Isolate each epoch in its own cache context: a single failing epoch
		// (e.g. an un-refundable challenger, a transient keeper error) must not
		// roll back the other epochs' transitions, and must not wedge the module
		// by failing the whole epoch-end forever. On error we log and skip; the
		// epoch is retried on the next epoch boundary.
		cacheCtx, write := sdkCtx.CacheContext()
		if err := k.advanceOne(cacheCtx, ed, upTo, reviewDelay, params); err != nil {
			sdkCtx.Logger().Error("distro: failed to advance epoch distribution",
				"epoch", ed.Epoch, "status", ed.Status.String(), "error", err)
			continue
		}
		write()
		sdkCtx.EventManager().EmitEvents(cacheCtx.EventManager().Events())
	}
	return nil
}

// advanceOne performs the lifecycle transition for a single epoch distribution.
// It is run inside a per-epoch cache context by advanceDistributions.
func (k Keeper) advanceOne(ctx sdk.Context, ed types.EpochDistribution, upTo, reviewDelay int64, params types.Params) error {
	switch ed.Status {
	case types.DISTRIBUTION_STATUS_VOTING:
		result, err := k.tallyEpoch(ctx, ed.Epoch, params)
		if err != nil {
			return err
		}
		if result == nil {
			// No consensus yet. Expire the epoch once its voting window has
			// elapsed so it stops being (expensively) re-tallied every epoch end.
			if upTo-ed.Epoch >= int64(params.VoteWindowEpochs) {
				ed.Status = types.DISTRIBUTION_STATUS_EXPIRED
				if err := k.EpochDistributions.Set(ctx, ed.Epoch, ed); err != nil {
					return err
				}
				ctx.EventManager().EmitEvent(sdk.NewEvent(
					types.EventTypeEpochExpired,
					sdk.NewAttribute(types.AttributeKeyEpoch, strconv.FormatInt(ed.Epoch, 10)),
				))
			}
			return nil
		}
		ed.MerkleRoot = result.root
		ed.LicenseTally = result.licenseTally.String()
		ed.StakeTally = result.stakeTally.String()
		ed.Status = types.DISTRIBUTION_STATUS_PENDING
		ed.PendingSinceEpoch = upTo
		if err := k.EpochDistributions.Set(ctx, ed.Epoch, ed); err != nil {
			return err
		}
		emitPending(ctx, ed)

	case types.DISTRIBUTION_STATUS_UNDER_REVIEW:
		result, err := k.tallyEpoch(ctx, ed.Epoch, params)
		if err != nil {
			return err
		}
		if result == nil {
			return nil // re-vote has not reached consensus; stay under review
		}
		// The re-vote is the judge of the challenge: if it re-confirms the
		// challenged root, the challenge was frivolous and the bond is burned;
		// otherwise the challenge surfaced a real problem and the bond is refunded.
		frivolous := bytes.Equal(result.root, ed.MerkleRoot)
		if err := k.resolveChallengeBond(ctx, ed, params.Denom, frivolous); err != nil {
			return err
		}
		ed.MerkleRoot = result.root
		ed.LicenseTally = result.licenseTally.String()
		ed.StakeTally = result.stakeTally.String()
		ed.Status = types.DISTRIBUTION_STATUS_PENDING
		// NOTE: PendingSinceEpoch is intentionally NOT reset here. Resuming the
		// original review timer (rather than restarting it) prevents repeated
		// challenges from indefinitely delaying finalization.
		ed.Challenger = ""
		ed.ChallengeBond = ""
		if err := k.EpochDistributions.Set(ctx, ed.Epoch, ed); err != nil {
			return err
		}
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			types.EventTypeChallengeResolved,
			sdk.NewAttribute(types.AttributeKeyEpoch, strconv.FormatInt(ed.Epoch, 10)),
			sdk.NewAttribute(types.AttributeKeyFrivolous, strconv.FormatBool(frivolous)),
		))
		emitPending(ctx, ed)

	case types.DISTRIBUTION_STATUS_PENDING:
		if upTo-ed.PendingSinceEpoch < reviewDelay {
			return nil // still within the review-delay window
		}
		ed.Status = types.DISTRIBUTION_STATUS_LIVE
		ed.FinalizedHeight = ctx.BlockHeight()
		if err := k.EpochDistributions.Set(ctx, ed.Epoch, ed); err != nil {
			return err
		}
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			types.EventTypeEpochFinalized,
			sdk.NewAttribute(types.AttributeKeyEpoch, strconv.FormatInt(ed.Epoch, 10)),
			sdk.NewAttribute(types.AttributeKeyLicenseTally, ed.LicenseTally),
			sdk.NewAttribute(types.AttributeKeyStakeTally, ed.StakeTally),
		))
	}
	return nil
}

// resolveChallengeBond burns the escrowed bond when a challenge was frivolous,
// or refunds it to the challenger otherwise. A no-op when nothing is escrowed.
func (k Keeper) resolveChallengeBond(ctx context.Context, ed types.EpochDistribution, denom string, frivolous bool) error {
	if ed.Challenger == "" || ed.ChallengeBond == "" {
		return nil
	}
	bond, ok := math.NewIntFromString(ed.ChallengeBond)
	if !ok || !bond.IsPositive() {
		return nil
	}
	coins := sdk.NewCoins(sdk.NewCoin(denom, bond))
	if frivolous {
		return k.bankKeeper.BurnCoins(ctx, types.ModuleName, coins)
	}
	challenger, err := sdk.AccAddressFromBech32(ed.Challenger)
	if err != nil {
		return err
	}
	return k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, challenger, coins)
}

func emitPending(sdkCtx sdk.Context, ed types.EpochDistribution) {
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeEpochPending,
		sdk.NewAttribute(types.AttributeKeyEpoch, strconv.FormatInt(ed.Epoch, 10)),
		sdk.NewAttribute(types.AttributeKeyLicenseTally, ed.LicenseTally),
		sdk.NewAttribute(types.AttributeKeyStakeTally, ed.StakeTally),
	))
}
