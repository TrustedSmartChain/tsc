package keeper

import (
	"context"
	"errors"
	"strconv"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// SubmitDistributionRoot records a node's merkle root for an epoch. The signer
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
	if msg.Epoch <= 0 {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "epoch must be positive")
	}

	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	// Bound the epoch to the current epoch so a licensed signer cannot spam
	// state with votes for arbitrarily distant future epochs.
	epochInfo, err := ms.k.epochsKeeper.GetEpochInfo(ctx, params.EpochIdentifier)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "unknown epoch identifier %q", params.EpochIdentifier)
	}
	if msg.Epoch > epochInfo.CurrentEpoch {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "epoch %d is in the future (current %d)", msg.Epoch, epochInfo.CurrentEpoch)
	}
	// Reject epochs older than the voting window: they can no longer reach
	// consensus (they would be expired at the next epoch end), so opening them
	// for voting only creates dead state.
	if epochInfo.CurrentEpoch-msg.Epoch > int64(params.VoteWindowEpochs) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "epoch %d is older than the voting window (current %d, window %d)", msg.Epoch, epochInfo.CurrentEpoch, params.VoteWindowEpochs)
	}

	// License gate: signer must hold >= 1 active license of the required type.
	licenses, err := ms.k.activeLicenseCount(ctx, msg.Signer, params.DistributionLicenseTypeId)
	if err != nil {
		return nil, err
	}
	if licenses == 0 {
		return nil, errorsmod.Wrapf(sdkerrors.ErrUnauthorized, "signer holds no active license of type %q", params.DistributionLicenseTypeId)
	}

	// Submissions are only accepted while the epoch is open for voting: either
	// VOTING (pre-consensus) or UNDER_REVIEW (voting reopened by a challenge).
	// A PENDING (consensus reached, in delay) or LIVE epoch is closed.
	existing, err := ms.k.EpochDistributions.Get(ctx, msg.Epoch)
	switch {
	case err == nil:
		switch existing.Status {
		case types.DISTRIBUTION_STATUS_VOTING, types.DISTRIBUTION_STATUS_UNDER_REVIEW:
			// open for (re)voting
		default:
			return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "epoch %d is not open for voting (status %s)", msg.Epoch, existing.Status)
		}
	case errors.Is(err, collections.ErrNotFound):
		// First submission for this epoch: open it for voting.
		if err := ms.k.EpochDistributions.Set(ctx, msg.Epoch, types.EpochDistribution{
			Epoch:  msg.Epoch,
			Status: types.DISTRIBUTION_STATUS_VOTING,
		}); err != nil {
			return nil, err
		}
	default:
		return nil, err
	}

	vote := types.DistributionVote{
		Epoch:      msg.Epoch,
		Signer:     msg.Signer,
		MerkleRoot: msg.MerkleRoot,
	}
	if err := ms.k.Votes.Set(ctx, collections.Join(msg.Epoch, msg.Signer), vote); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeSubmitRoot,
		sdk.NewAttribute(types.AttributeKeyEpoch, strconv.FormatInt(msg.Epoch, 10)),
		sdk.NewAttribute(types.AttributeKeySigner, msg.Signer),
	))

	return &types.MsgSubmitDistributionRootResponse{}, nil
}
