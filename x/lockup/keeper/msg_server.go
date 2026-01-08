package keeper

import (
	"context"

	"github.com/TrustedSmartChain/tsc/x/lockup/types"
)

type msgServer struct {
	k Keeper
}

var _ types.MsgServer = msgServer{}

// NewMsgServerImpl returns an implementation of the module MsgServer interface.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{k: keeper}
}

// Lock implements types.MsgServer.
func (ms msgServer) Lock(ctx context.Context, msg *types.MsgLock) (*types.MsgLockResponse, error) {
	// ctx := sdk.UnwrapSDKContext(goCtx)
	panic("Lock is unimplemented")
	return &types.MsgLockResponse{}, nil
}

// Extend implements types.MsgServer.
func (ms msgServer) Extend(ctx context.Context, msg *types.MsgExtend) (*types.MsgExtendResponse, error) {
	// ctx := sdk.UnwrapSDKContext(goCtx)
	panic("Extend is unimplemented")
	return &types.MsgExtendResponse{}, nil
}

// SendDelegateAndLock implements types.MsgServer.
func (ms msgServer) SendDelegateAndLock(ctx context.Context, msg *types.MsgSendDelegateAndLock) (*types.MsgSendDelegateAndLockResponse, error) {
	// ctx := sdk.UnwrapSDKContext(goCtx)
	panic("SendDelegateAndLock is unimplemented")
	return &types.MsgSendDelegateAndLockResponse{}, nil
}

// MultiSendDelegateAndLock implements types.MsgServer.
func (ms msgServer) MultiSendDelegateAndLock(ctx context.Context, msg *types.MsgMultiSendDelegateAndLock) (*types.MsgMultiSendDelegateAndLockResponse, error) {
	// ctx := sdk.UnwrapSDKContext(goCtx)
	panic("MultiSendDelegateAndLock is unimplemented")
	return &types.MsgMultiSendDelegateAndLockResponse{}, nil
}
