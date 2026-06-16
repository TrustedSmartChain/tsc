package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// SubmitDistributionRoot records a node's merkle root for a day. The signer
// must hold >= 1 active license of the configured distribution license type.
// Re-submission by the same signer overwrites that signer's prior vote.
func (ms msgServer) SubmitDistributionRoot(goCtx context.Context, msg *types.MsgSubmitDistributionRoot) (*types.MsgSubmitDistributionRootResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if _, err := sdk.AccAddressFromBech32(msg.Signer); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid signer address")
	}
	if len(msg.MerkleRoot) == 0 {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "merkle root cannot be empty")
	}

	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	// The submission must target a configured distribution type.
	distroType, ok := params.FindDistributionType(msg.DistroType)
	if !ok {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "unknown distribution type %q", msg.DistroType)
	}

	// Resolve the current distribution day from the x/epochs schedule.
	epochInfo, err := ms.k.epochsKeeper.GetEpochInfo(ctx, params.EpochIdentifier)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "unknown epoch identifier %q", params.EpochIdentifier)
	}
	currentDate, err := dateForEpoch(params.DistributionStartDate, epochInfo.CurrentEpoch)
	if err != nil {
		return nil, err
	}

	// The submitted date must be a valid day on or after the distribution start
	// date (and not after the current day).
	age, err := daysBetween(msg.Date, currentDate)
	if err != nil {
		return nil, err
	}
	if startAge, err := daysBetween(params.DistributionStartDate, msg.Date); err != nil {
		return nil, err
	} else if startAge < 0 {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "date %q is before the distribution start date %q", msg.Date, params.DistributionStartDate)
	}
	// Bound the date to the current day so a licensed signer cannot spam state
	// with votes for arbitrarily distant future days.
	if age < 0 {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "date %q is in the future (current %q)", msg.Date, currentDate)
	}

	// License gate: signer must hold a license of the required type that was valid
	// on the submitted day. Eligibility is judged as of msg.Date, so a license
	// minted after that day cannot be used to vote on it.
	licenses, err := ms.k.licensesValidOnForHolder(ctx, msg.Signer, params.DistributionLicenseTypeId, msg.Date)
	if err != nil {
		return nil, err
	}
	if licenses == 0 {
		return nil, errorsmod.Wrapf(sdkerrors.ErrUnauthorized, "signer holds no license of type %q valid on %q", params.DistributionLicenseTypeId, msg.Date)
	}

	// The header leaf (distro_type, date, version, totals_by_category) must be
	// committed in the submitted root. This binds the root to exactly the totals
	// being validated below: a root cannot pass with totals it does not contain.
	if len(msg.HeaderProof) > types.MaxProofDepth {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "header proof too long: %d exceeds max depth %d", len(msg.HeaderProof), types.MaxProofDepth)
	}
	headerLeaf := types.HeaderLeafHash(msg.DistroType, msg.Date, msg.Version, msg.TotalsByCategory)
	if !types.VerifyProof(msg.MerkleRoot, headerLeaf, msg.HeaderProof) {
		return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "header leaf is not included in the submitted root")
	}

	// The header totals must respect the per-type / per-category budget caps
	// derived from the day's halving budget and the params percentages.
	if err := validateHeaderTotals(params, distroType, msg.Date, msg.TotalsByCategory); err != nil {
		return nil, err
	}

	// Submissions are only accepted while the day is open for voting: either
	// VOTING (pre-consensus) or UNDER_REVIEW (voting reopened by a challenge or a
	// governance revival). A PENDING (consensus reached, in delay), LIVE or
	// EXPIRED day is closed. An already-open day is accepted regardless of age:
	// its voting window is tracked by VotingSinceDate and enforced at epoch end,
	// so a governance-revived day (older than the window) can still be voted on.
	distKey := collections.Join(msg.Date, msg.DistroType)
	existing, err := ms.k.Distributions.Get(ctx, distKey)
	switch {
	case err == nil:
		switch existing.Status {
		case types.DISTRIBUTION_STATUS_VOTING, types.DISTRIBUTION_STATUS_UNDER_REVIEW:
			// open for (re)voting
		default:
			return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "date %q type %q is not open for voting (status %s)", msg.Date, msg.DistroType, existing.Status)
		}
	case errors.Is(err, collections.ErrNotFound):
		// First submission for this (day, type). Reject days older than the voting
		// window: a fresh entry that distant can no longer reach consensus (it would
		// expire at the next epoch end), so opening it only creates dead state.
		// Reviving a lapsed day is the authority-gated MsgReviveDistribution path.
		if age > int64(params.VoteWindowDays) {
			return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "date %q is older than the voting window (current %q, window %d days)", msg.Date, currentDate, params.VoteWindowDays)
		}
		// Open it for voting, stamping the window start at the current day, and
		// add it to the active index so the epoch hook will process it.
		if err := ms.k.Distributions.Set(ctx, distKey, types.Distribution{
			Date:            msg.Date,
			DistroType:      msg.DistroType,
			Status:          types.DISTRIBUTION_STATUS_VOTING,
			VotingSinceDate: currentDate,
		}); err != nil {
			return nil, err
		}
		if err := ms.k.ActiveDistributions.Set(ctx, distKey); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	vote := types.DistributionVote{
		Date:       msg.Date,
		Signer:     msg.Signer,
		MerkleRoot: msg.MerkleRoot,
		DistroType: msg.DistroType,
		Header: &types.DistributionHeader{
			Version:          msg.Version,
			TotalsByCategory: msg.TotalsByCategory,
		},
	}
	if err := ms.k.Votes.Set(ctx, collections.Join3(msg.Date, msg.DistroType, msg.Signer), vote); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeSubmitRoot,
		sdk.NewAttribute(types.AttributeKeyDate, msg.Date),
		sdk.NewAttribute(types.AttributeKeySigner, msg.Signer),
		sdk.NewAttribute(types.AttributeKeyDistroType, msg.DistroType),
	))

	return &types.MsgSubmitDistributionRootResponse{}, nil
}

// validateHeaderTotals enforces the per-type and per-category budget caps on a
// submission's header totals. The type may distribute at most its share
// (TypeBudget) of the day's halving budget. When the type configures category
// percentages, each category's total is additionally capped at its share
// (CategoryCap) of the type budget — and those per-category caps sum to the type
// budget, so they also bound the whole type. When the type has no category
// percentages, only the type-level sum is bounded (categories are uncapped).
// Every named category must be one the type configures.
func validateHeaderTotals(params types.Params, dt types.DistributionType, date string, totals map[string]string) error {
	dayBudget, err := dateBudget(params, date)
	if err != nil {
		return err
	}
	typeBudget := types.TypeBudget(dayBudget, dt.Percentage)
	perCategoryCaps := dt.HasCategoryPercentages()

	sum := math.ZeroInt()
	for name, raw := range totals {
		amount, ok := math.NewIntFromString(raw)
		if !ok || amount.IsNegative() {
			return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "category %q total %q is not a valid non-negative integer", name, raw)
		}
		category, ok := dt.FindCategory(name)
		if !ok {
			return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "category %q is not configured for distribution type %q", name, dt.Id)
		}
		if perCategoryCaps {
			cap := types.CategoryCap(typeBudget, category.Percentage)
			if amount.GT(cap) {
				return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "category %q total %s exceeds its cap %s for type %q on %q", name, amount, cap, dt.Id, date)
			}
		}
		sum = sum.Add(amount)
	}
	if sum.GT(typeBudget) {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "header totals %s exceed the type budget %s for type %q on %q", sum, typeBudget, dt.Id, date)
	}
	return nil
}
