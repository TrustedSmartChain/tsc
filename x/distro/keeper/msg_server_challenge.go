package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// ChallengeDistribution places a PENDING day's distribution UNDER_REVIEW,
// reopening voting. The challenger must hold an active license of the configured
// type and escrows the configured bond. The re-vote resolves the challenge: if
// it re-confirms the same root the bond is burned (frivolous), otherwise the
// bond is refunded and the corrected root is adopted.
func (ms msgServer) ChallengeDistribution(goCtx context.Context, msg *types.MsgChallengeDistribution) (*types.MsgChallengeDistributionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	challenger, err := sdk.AccAddressFromBech32(msg.Challenger)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid challenger address")
	}

	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve the current day so the re-vote window can be bounded from here: a
	// challenge reopens voting, and if the re-vote never reaches consensus the day
	// must not stay UNDER_REVIEW (bond + distribution locked) forever.
	epochInfo, err := ms.k.epochsKeeper.GetEpochInfo(ctx, params.EpochIdentifier)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "unknown epoch identifier %q", params.EpochIdentifier)
	}
	currentDate, err := dateForEpoch(params.DistributionStartDate, epochInfo.CurrentEpoch)
	if err != nil {
		return nil, err
	}

	// Challenger must hold >= 1 active license of the required type.
	licenses, err := ms.k.activeLicenseCount(ctx, msg.Challenger, params.DistributionLicenseTypeId)
	if err != nil {
		return nil, err
	}
	if licenses == 0 {
		return nil, errorsmod.Wrapf(sdkerrors.ErrUnauthorized, "challenger holds no active license of type %q", params.DistributionLicenseTypeId)
	}

	// Only a PENDING distribution (consensus reached, in the review window) can
	// be challenged.
	ed, err := ms.k.Distributions.Get(ctx, msg.Date)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "no distribution for date %q", msg.Date)
	}
	if ed.Status != types.DISTRIBUTION_STATUS_PENDING {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "date %q is not pending (status %s)", msg.Date, ed.Status)
	}

	// Escrow the challenge bond (if configured) from the challenger.
	bond, ok := math.NewIntFromString(params.ChallengeBond)
	if !ok {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "invalid challenge bond param")
	}
	if bond.IsPositive() {
		coins := sdk.NewCoins(sdk.NewCoin(params.Denom, bond))
		if err := ms.k.bankKeeper.SendCoinsFromAccountToModule(ctx, challenger, types.ModuleName, coins); err != nil {
			return nil, err
		}
	}

	ed.Status = types.DISTRIBUTION_STATUS_UNDER_REVIEW
	ed.Challenger = msg.Challenger
	ed.ChallengeBond = bond.String()
	// Mark the re-vote's opening day so the review window can time out (see
	// advanceOne's UNDER_REVIEW handling). The original PENDING root is left in
	// ed.MerkleRoot so it can be restored if the challenge does not overturn it.
	ed.VotingSinceDate = currentDate
	if err := ms.k.Distributions.Set(ctx, msg.Date, ed); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeDistributionUnderReview,
		sdk.NewAttribute(types.AttributeKeyDate, msg.Date),
		sdk.NewAttribute(types.AttributeKeyChallenger, msg.Challenger),
		sdk.NewAttribute(types.AttributeKeyBond, bond.String()),
	))

	return &types.MsgChallengeDistributionResponse{}, nil
}
