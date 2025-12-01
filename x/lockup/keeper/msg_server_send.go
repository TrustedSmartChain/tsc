package keeper

import (
	"context"

	// errorsmod "cosmossdk.io/errors"
	// "cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/TrustedSmartChain/tsc/x/lockup/types"
)

// SendAndLock implements types.MsgServer.
func (ms msgServer) SendAndLock(goCtx context.Context, msg *types.MsgSendAndLock) (*types.MsgSendAndLockResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	// should be able to just send the coins because the LockedBalanceDecorator will check for locked funds and ensure the send is valid
	fromAddr, err := sdk.AccAddressFromBech32(msg.FromAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid from address: %s", err)
	}

	toAddr, err := sdk.AccAddressFromBech32(msg.SendAndLock.ToAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid to address: %s", err)
	}
	err = ms.k.bankKeeper.SendCoins(ctx, fromAddr, toAddr, sdk.NewCoins(msg.SendAndLock.Lock.Amount))
	if err != nil {
		return nil, err
	}

	// now lock the funds
	lockupMsg := &types.MsgLock{
		LockupAddress: msg.SendAndLock.ToAddress,
		Lockups:       []*types.Lock{msg.SendAndLock.Lock},
	}

	_, err = ms.Lock(goCtx, lockupMsg)
	if err != nil {
		return nil, err
	}

	return &types.MsgSendAndLockResponse{}, nil
}

// MultiSendAndLock implements types.MsgServer.
func (ms msgServer) MultiSendAndLock(ctx context.Context, msg *types.MsgMultiSendAndLock) (*types.MsgMultiSendAndLockResponse, error) {
	_, err := sdk.AccAddressFromBech32(msg.FromAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid from address: %s", err)
	}

	for _, sendAndLock := range msg.SendAndLocks {
		_, err = ms.SendAndLock(ctx,
			&types.MsgSendAndLock{
				FromAddress: msg.FromAddress,
				SendAndLock: sendAndLock,
			},
		)
		if err != nil {
			return nil, err
		}
	}
	return &types.MsgMultiSendAndLockResponse{}, nil
}
