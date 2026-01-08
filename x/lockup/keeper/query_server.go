package keeper

import (
	"context"

	// sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/x/lockup/types"
)

var _ types.QueryServer = Querier{}

type Querier struct {
	Keeper
}

func NewQuerier(keeper Keeper) Querier {
	return Querier{Keeper: keeper}
}

// ActiveLocks implements types.QueryServer.
func (k Querier) ActiveLocks(goCtx context.Context, req *types.QueryActiveLocksRequest) (*types.QueryActiveLocksResponse, error) {
	// ctx := sdk.UnwrapSDKContext(goCtx)
	panic("ActiveLocks is unimplemented")
	return &types.QueryActiveLocksResponse{}, nil
}

// TotalLockedAmount implements types.QueryServer.
func (k Querier) TotalLockedAmount(goCtx context.Context, req *types.QueryTotalLockedAmountRequest) (*types.QueryTotalLockedAmountResponse, error) {
	// ctx := sdk.UnwrapSDKContext(goCtx)
	panic("TotalLockedAmount is unimplemented")
	return &types.QueryTotalLockedAmountResponse{}, nil
}

// AccountLocks implements types.QueryServer.
func (k Querier) AccountLocks(goCtx context.Context, req *types.QueryAccountLocksRequest) (*types.QueryAccountLocksResponse, error) {
	// ctx := sdk.UnwrapSDKContext(goCtx)
	panic("AccountLocks is unimplemented")
	return &types.QueryAccountLocksResponse{}, nil
}

// Locks implements types.QueryServer.
func (k Querier) Locks(goCtx context.Context, req *types.QueryLocksRequest) (*types.QueryLocksResponse, error) {
	// ctx := sdk.UnwrapSDKContext(goCtx)
	panic("Locks is unimplemented")
	return &types.QueryLocksResponse{}, nil
}
