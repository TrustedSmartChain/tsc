package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
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
	// date (and not after the current day). daysBetween validates the format and
	// rejects dates before the start date implicitly via the bounds below.
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
	// Reject days older than the voting window: they can no longer reach
	// consensus (they would be expired at the next epoch end), so opening them
	// for voting only creates dead state.
	if age > int64(params.VoteWindowDays) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "date %q is older than the voting window (current %q, window %d days)", msg.Date, currentDate, params.VoteWindowDays)
	}

	// License gate: signer must hold >= 1 active license of the required type.
	licenses, err := ms.k.activeLicenseCount(ctx, msg.Signer, params.DistributionLicenseTypeId)
	if err != nil {
		return nil, err
	}
	if licenses == 0 {
		return nil, errorsmod.Wrapf(sdkerrors.ErrUnauthorized, "signer holds no active license of type %q", params.DistributionLicenseTypeId)
	}

	// Submissions are only accepted while the day is open for voting: either
	// VOTING (pre-consensus) or UNDER_REVIEW (voting reopened by a challenge).
	// A PENDING (consensus reached, in delay) or LIVE day is closed.
	existing, err := ms.k.Distributions.Get(ctx, msg.Date)
	switch {
	case err == nil:
		switch existing.Status {
		case types.DISTRIBUTION_STATUS_VOTING, types.DISTRIBUTION_STATUS_UNDER_REVIEW:
			// open for (re)voting
		default:
			return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "date %q is not open for voting (status %s)", msg.Date, existing.Status)
		}
	case errors.Is(err, collections.ErrNotFound):
		// First submission for this day: open it for voting.
		if err := ms.k.Distributions.Set(ctx, msg.Date, types.Distribution{
			Date:   msg.Date,
			Status: types.DISTRIBUTION_STATUS_VOTING,
		}); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	vote := types.DistributionVote{
		Date:       msg.Date,
		Signer:     msg.Signer,
		MerkleRoot: msg.MerkleRoot,
	}
	if err := ms.k.Votes.Set(ctx, collections.Join(msg.Date, msg.Signer), vote); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeSubmitRoot,
		sdk.NewAttribute(types.AttributeKeyDate, msg.Date),
		sdk.NewAttribute(types.AttributeKeySigner, msg.Signer),
	))

	return &types.MsgSubmitDistributionRootResponse{}, nil
}
