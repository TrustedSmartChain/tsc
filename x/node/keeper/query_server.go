package keeper

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/TrustedSmartChain/tsc/v3/x/node/types"
)

type queryServer struct {
	Keeper
}

var _ types.QueryServer = queryServer{}

func NewQueryServerImpl(k Keeper) types.QueryServer {
	return &queryServer{k}
}

func (qs queryServer) CheckIn(ctx context.Context, req *types.QueryCheckInRequest) (*types.QueryCheckInResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if req.Address == "" {
		return nil, status.Error(codes.InvalidArgument, "address is required")
	}
	if req.Date == "" {
		return nil, status.Error(codes.InvalidArgument, "date is required")
	}

	store := qs.storeService.OpenKVStore(ctx)

	key := types.CheckInKey(req.Address, req.Date)
	val, err := store.Get(key)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	if val == nil {
		return nil, status.Errorf(codes.NotFound, "no check-in found for %s on %s", req.Address, req.Date)
	}

	blockHeight := sdk.BigEndianToUint64(val)
	return &types.QueryCheckInResponse{BlockHeight: blockHeight}, nil
}
