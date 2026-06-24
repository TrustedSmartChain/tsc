package keeper

import (
	"context"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/v3/x/node/types"
)

type msgServer struct {
	Keeper
}

var _ types.MsgServer = msgServer{}

func NewMsgServerImpl(k Keeper) types.MsgServer {
	return &msgServer{k}
}

func (ms msgServer) CheckIn(ctx context.Context, msg *types.MsgCheckIn) (*types.MsgCheckInResponse, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	date := sdkCtx.BlockTime().UTC().Format(time.DateOnly)
	key := types.CheckInKey(msg.Address, date)

	store := ms.storeService.OpenKVStore(ctx)

	// Record block height as the value so we know exactly when the check-in landed.
	heightBytes := sdk.Uint64ToBigEndian(uint64(sdkCtx.BlockHeight()))
	if err := store.Set(key, heightBytes); err != nil {
		return nil, err
	}

	sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
		"checkin",
		sdk.NewAttribute("address", msg.Address),
		sdk.NewAttribute("date", date),
	))

	return &types.MsgCheckInResponse{}, nil
}
