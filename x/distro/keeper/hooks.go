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

// AfterEpochEnd advances the distribution lifecycle for every day up to and
// including the day that just ended:
//   - VOTING: tallied; on consensus moves to PENDING (start of the review delay).
//   - UNDER_REVIEW: re-tallied; on consensus the challenge bond is resolved and
//     it moves back to PENDING with the (possibly corrected) root.
//   - PENDING: auto-promoted to LIVE once the review delay has elapsed.
//
// The x/epochs epoch number is translated to its calendar day (the module keys
// all state by date). Processing all such days (not only the just-ended one)
// tolerates nodes that submit their root a day late. Votes are retained.
func (h EpochHooks) AfterEpochEnd(ctx context.Context, identifier string, epochNumber int64) error {
	params, err := h.k.Params.Get(ctx)
	if err != nil {
		return err
	}
	if identifier != params.EpochIdentifier {
		return nil
	}
	upToDate, err := dateForEpoch(params.DistributionStartDate, epochNumber)
	if err != nil {
		return err
	}
	return h.k.advanceDistributions(ctx, upToDate, params)
}

func (k Keeper) advanceDistributions(ctx context.Context, upTo string, params types.Params) error {
	// Snapshot the candidate days first (ascending date order — YYYY-MM-DD sorts
	// chronologically) so we don't mutate the map while iterating it.
	var candidates []types.Distribution
	if err := k.Distributions.Walk(ctx, nil, func(date string, d types.Distribution) (bool, error) {
		if date <= upTo {
			candidates = append(candidates, d)
		}
		return false, nil
	}); err != nil {
		return err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	reviewDelay := int64(params.ReviewDelayDays)

	for _, d := range candidates {
		// Isolate each day in its own cache context: a single failing day
		// (e.g. an un-refundable challenger, a transient keeper error) must not
		// roll back the other days' transitions, and must not wedge the module
		// by failing the whole epoch-end forever. On error we log and skip; the
		// day is retried on the next epoch boundary.
		cacheCtx, write := sdkCtx.CacheContext()
		if err := k.advanceOne(cacheCtx, d, upTo, reviewDelay, params); err != nil {
			sdkCtx.Logger().Error("distro: failed to advance distribution",
				"date", d.Date, "status", d.Status.String(), "error", err)
			continue
		}
		write()
		sdkCtx.EventManager().EmitEvents(cacheCtx.EventManager().Events())
	}
	return nil
}

// advanceOne performs the lifecycle transition for a single day's distribution.
// It is run inside a per-day cache context by advanceDistributions. upTo is the
// day (YYYY-MM-DD) that just ended; reviewDelay is in days.
func (k Keeper) advanceOne(ctx sdk.Context, d types.Distribution, upTo string, reviewDelay int64, params types.Params) error {
	switch d.Status {
	case types.DISTRIBUTION_STATUS_VOTING:
		result, err := k.tallyDistribution(ctx, d.Date, params)
		if err != nil {
			return err
		}
		if result == nil {
			// No consensus yet. Expire the day once its voting window has
			// elapsed so it stops being (expensively) re-tallied every epoch end.
			// The window is measured from when voting (re)opened (VotingSinceDate),
			// so a governance-revived day gets a fresh full window; fall back to the
			// distribution's own date for entries predating that field.
			windowStart := d.VotingSinceDate
			if windowStart == "" {
				windowStart = d.Date
			}
			age, err := daysBetween(windowStart, upTo)
			if err != nil {
				return err
			}
			if age >= int64(params.VoteWindowDays) {
				d.Status = types.DISTRIBUTION_STATUS_EXPIRED
				if err := k.Distributions.Set(ctx, d.Date, d); err != nil {
					return err
				}
				ctx.EventManager().EmitEvent(sdk.NewEvent(
					types.EventTypeDistributionExpired,
					sdk.NewAttribute(types.AttributeKeyDate, d.Date),
				))
			}
			return nil
		}
		d.MerkleRoot = result.root
		d.LicenseTally = result.licenseTally.String()
		d.StakeTally = result.stakeTally.String()
		d.Status = types.DISTRIBUTION_STATUS_PENDING
		d.PendingSinceDate = upTo
		if err := k.Distributions.Set(ctx, d.Date, d); err != nil {
			return err
		}
		emitPending(ctx, d)

	case types.DISTRIBUTION_STATUS_UNDER_REVIEW:
		result, err := k.tallyDistribution(ctx, d.Date, params)
		if err != nil {
			return err
		}
		if result == nil {
			// The re-vote has not reached consensus. Bound how long a challenge can
			// hold a day UNDER_REVIEW: once the re-vote window (vote_window_days)
			// elapses, resolve deterministically rather than locking the bond and
			// the distribution forever. The pre-challenge consensus root is
			// unchanged, so this is treated like a frivolous challenge — the bond is
			// burned and the day returns to PENDING, resuming its original review
			// timer (which has typically already elapsed, so it promotes to LIVE at
			// the next epoch).
			windowStart := d.VotingSinceDate
			if windowStart == "" {
				windowStart = d.Date
			}
			age, err := daysBetween(windowStart, upTo)
			if err != nil {
				return err
			}
			if age < int64(params.VoteWindowDays) {
				return nil // still within the re-vote window; stay under review
			}
			if err := k.resolveChallengeBond(ctx, d, params.Denom, true); err != nil {
				return err
			}
			d.Status = types.DISTRIBUTION_STATUS_PENDING
			d.Challenger = ""
			d.ChallengeBond = ""
			if err := k.Distributions.Set(ctx, d.Date, d); err != nil {
				return err
			}
			ctx.EventManager().EmitEvent(sdk.NewEvent(
				types.EventTypeChallengeResolved,
				sdk.NewAttribute(types.AttributeKeyDate, d.Date),
				sdk.NewAttribute(types.AttributeKeyFrivolous, strconv.FormatBool(true)),
				sdk.NewAttribute(types.AttributeKeyTimedOut, strconv.FormatBool(true)),
			))
			emitPending(ctx, d)
			return nil
		}
		// The re-vote is the judge of the challenge: if it re-confirms the
		// challenged root, the challenge was frivolous and the bond is burned;
		// otherwise the challenge surfaced a real problem and the bond is refunded.
		frivolous := bytes.Equal(result.root, d.MerkleRoot)
		if err := k.resolveChallengeBond(ctx, d, params.Denom, frivolous); err != nil {
			return err
		}
		d.MerkleRoot = result.root
		d.LicenseTally = result.licenseTally.String()
		d.StakeTally = result.stakeTally.String()
		d.Status = types.DISTRIBUTION_STATUS_PENDING
		// NOTE: PendingSinceDate is intentionally NOT reset here. Resuming the
		// original review timer (rather than restarting it) prevents repeated
		// challenges from indefinitely delaying finalization.
		d.Challenger = ""
		d.ChallengeBond = ""
		if err := k.Distributions.Set(ctx, d.Date, d); err != nil {
			return err
		}
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			types.EventTypeChallengeResolved,
			sdk.NewAttribute(types.AttributeKeyDate, d.Date),
			sdk.NewAttribute(types.AttributeKeyFrivolous, strconv.FormatBool(frivolous)),
		))
		emitPending(ctx, d)

	case types.DISTRIBUTION_STATUS_PENDING:
		age, err := daysBetween(d.PendingSinceDate, upTo)
		if err != nil {
			return err
		}
		if age < reviewDelay {
			return nil // still within the review-delay window
		}
		d.Status = types.DISTRIBUTION_STATUS_LIVE
		d.FinalizedHeight = ctx.BlockHeight()
		if err := k.Distributions.Set(ctx, d.Date, d); err != nil {
			return err
		}
		ctx.EventManager().EmitEvent(sdk.NewEvent(
			types.EventTypeDistributionFinalized,
			sdk.NewAttribute(types.AttributeKeyDate, d.Date),
			sdk.NewAttribute(types.AttributeKeyLicenseTally, d.LicenseTally),
			sdk.NewAttribute(types.AttributeKeyStakeTally, d.StakeTally),
		))
	}
	return nil
}

// resolveChallengeBond burns the escrowed bond when a challenge was frivolous,
// or refunds it to the challenger otherwise. A no-op when nothing is escrowed.
func (k Keeper) resolveChallengeBond(ctx context.Context, d types.Distribution, denom string, frivolous bool) error {
	if d.Challenger == "" || d.ChallengeBond == "" {
		return nil
	}
	bond, ok := math.NewIntFromString(d.ChallengeBond)
	if !ok || !bond.IsPositive() {
		return nil
	}
	coins := sdk.NewCoins(sdk.NewCoin(denom, bond))
	if frivolous {
		return k.bankKeeper.BurnCoins(ctx, types.ModuleName, coins)
	}
	challenger, err := sdk.AccAddressFromBech32(d.Challenger)
	if err != nil {
		return err
	}
	return k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, challenger, coins)
}

func emitPending(sdkCtx sdk.Context, d types.Distribution) {
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeDistributionPending,
		sdk.NewAttribute(types.AttributeKeyDate, d.Date),
		sdk.NewAttribute(types.AttributeKeyLicenseTally, d.LicenseTally),
		sdk.NewAttribute(types.AttributeKeyStakeTally, d.StakeTally),
	))
}
