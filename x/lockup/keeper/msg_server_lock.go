package keeper

import (
	"context"
	"time"

	errorsmod "cosmossdk.io/errors"

	"github.com/TrustedSmartChain/tsc/x/lockup/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

func (k msgServer) Lock(goCtx context.Context, msg *types.MsgLock) (*types.MsgLockResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	if msg.Amount.Denom != bondDenom {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid denom: %s, expected: %s", msg.Amount.Denom, bondDenom)
	}

	address, err := sdk.AccAddressFromBech32(msg.Address)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid lockup address: %s", err)
	}

	if !msg.Amount.IsPositive() {
		return nil, sdkerrors.ErrInvalidCoins.Wrapf("invalid lock amount: %s", msg.Amount.String())
	}

	unlockDate, err := time.Parse(time.DateOnly, msg.UnlockDate)
	if err != nil {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid unlock date format: %s", msg.UnlockDate)
	}

	blockTime := ctx.BlockTime()
	blockDay := time.Date(blockTime.Year(), blockTime.Month(), blockTime.Day(), 0, 0, 0, 0, time.UTC)

	if blockDay.After(unlockDate) || blockDay.Equal(unlockDate) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("unlock date must be in the future")
	}

	if blockDay.AddDate(2, 0, 0).Before(unlockDate) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("unlock date cannot be more than 2 years from now")
	}

	currentLockedAmount, err := k.GetLockedAmountByAddress(ctx, address)
	if err != nil {
		return nil, err
	}

	totalDelegatedAmount, err := k.GetTotalDelegatedAmount(ctx, address)
	if err != nil {
		return nil, err
	}

	if totalDelegatedAmount.LT(currentLockedAmount.Add(msg.Amount.Amount)) {
		return nil, errorsmod.Wrapf(
			types.ErrInsufficientDelegations,
			"insufficient delegated tokens to create new locks by the requested amount: %s < %s",
			totalDelegatedAmount.String(),
			currentLockedAmount.Add(msg.Amount.Amount).String(),
		)
	}

	exisitingLock, idx, found := k.GetLockByAddressAndDate(ctx, address, msg.UnlockDate)
	if found {
		newAmount := exisitingLock.Amount.Add(msg.Amount.Amount)
		if err = k.UpdateLockByAddressAndIndex(ctx, address, idx, &types.Lock{UnlockDate: exisitingLock.UnlockDate, Amount: newAmount}); err != nil {
			return nil, err
		}
	} else {

		if err = k.SetLockByAddress(ctx, address, &types.Lock{UnlockDate: msg.UnlockDate, Amount: msg.Amount.Amount}); err != nil {
			return nil, err
		}
	}

	if err := k.AddToExpirationQueue(ctx, unlockDate, address, msg.Amount.Amount); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvents([]sdk.Event{
		sdk.NewEvent(
			types.EventTypeLock,
			sdk.NewAttribute(types.AttributeKeyLockAddress, msg.Address),
			sdk.NewAttribute(types.AttributeKeyUnlockDate, msg.UnlockDate),
			sdk.NewAttribute(sdk.AttributeKeyAmount, msg.Amount.Amount.String()),
		),
	})

	return &types.MsgLockResponse{}, nil
}
