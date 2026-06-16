package keeper

import (
	"context"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// ReviveDistribution reopens an EXPIRED day's distribution for voting. It is
// governance-gated (the authority must be the module's governance account)
// because reviving un-forfeits a day whose rewards had lapsed. The day is moved
// back to VOTING with a fresh full voting window measured from the current day
// (VotingSinceDate), so it can re-reach consensus and, if it does not, simply
// re-expire after the window — at which point it can be revived again.
func (ms msgServer) ReviveDistribution(goCtx context.Context, msg *types.MsgReviveDistribution) (*types.MsgReviveDistributionResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if ms.k.authority != msg.Authority {
		return nil, errorsmod.Wrapf(govtypes.ErrInvalidSigner, "invalid authority; expected %s, got %s", ms.k.authority, msg.Authority)
	}

	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	// Resolve the current distribution day from the x/epochs schedule; the fresh
	// voting window is measured from it.
	epochInfo, err := ms.k.epochsKeeper.GetEpochInfo(ctx, params.EpochIdentifier)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "unknown epoch identifier %q", params.EpochIdentifier)
	}
	currentDate, err := dateForEpoch(params.DistributionStartDate, epochInfo.CurrentEpoch)
	if err != nil {
		return nil, err
	}

	if _, ok := params.FindDistributionType(msg.DistroType); !ok {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "unknown distribution type %q", msg.DistroType)
	}

	distKey := collections.Join(msg.Date, msg.DistroType)
	d, err := ms.k.Distributions.Get(ctx, distKey)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "no distribution for date %q type %q", msg.Date, msg.DistroType)
	}
	if d.Status != types.DISTRIBUTION_STATUS_EXPIRED {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "date %q type %q is not expired (status %s)", msg.Date, msg.DistroType, d.Status)
	}

	// Reopen for voting with a fresh window. An expired day never reached
	// consensus, so there is no canonical root to preserve; clear any transient
	// challenge bookkeeping defensively. Retained votes (if any) remain and will
	// be re-tallied with current license/stake weights.
	d.Status = types.DISTRIBUTION_STATUS_VOTING
	d.VotingSinceDate = currentDate
	d.MerkleRoot = nil
	d.LicenseTally = ""
	d.StakeTally = ""
	d.PendingSinceDate = ""
	d.Challenger = ""
	d.ChallengeBond = ""
	d.ValidatorTally = ""
	d.Header = nil
	if err := ms.k.Distributions.Set(ctx, distKey, d); err != nil {
		return nil, err
	}
	// Reopened: re-add to the active index so the epoch hook processes it again.
	if err := ms.k.ActiveDistributions.Set(ctx, distKey); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeDistributionRevived,
		sdk.NewAttribute(types.AttributeKeyDate, msg.Date),
		sdk.NewAttribute(types.AttributeKeyDistroType, msg.DistroType),
	))

	return &types.MsgReviveDistributionResponse{}, nil
}
