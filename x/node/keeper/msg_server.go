package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/v3/x/node/types"
)

type msgServer struct{}

var _ types.MsgServer = msgServer{}

func NewMsgServerImpl() types.MsgServer {
	return &msgServer{}
}

func (ms msgServer) CheckIn(ctx context.Context, msg *types.MsgCheckIn) (*types.MsgCheckInResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)
	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"checkin",
		sdk.NewAttribute("address", msg.Address),
	))
	return &types.MsgCheckInResponse{}, nil
}
