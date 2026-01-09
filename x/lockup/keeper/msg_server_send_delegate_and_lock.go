package keeper

import (
	"context"

	"github.com/TrustedSmartChain/tsc/x/lockup/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

func (k msgServer) SendDelegateAndLock(goCtx context.Context, msg *types.MsgSendDelegateAndLock) (*types.MsgSendDelegateAndLockResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if !msg.Amount.IsPositive() {
		return nil, sdkerrors.ErrInvalidCoins.Wrapf("invalid amount: %s", msg.Amount.String())
	}

	fromAddr, err := sdk.AccAddressFromBech32(msg.FromAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid from address: %s", err)
	}

	toAddr, err := sdk.AccAddressFromBech32(msg.ToAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid to address: %s", err)
	}

	bondDenom, err := k.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	if msg.Amount.Denom != bondDenom {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid denom: %s, expected: %s", msg.Amount.Denom, bondDenom)
	}

	err = k.bankKeeper.SendCoins(ctx, fromAddr, toAddr, sdk.NewCoins(msg.Amount))
	if err != nil {
		return nil, err
	}

	valAddr, err := sdk.ValAddressFromBech32(msg.ValidatorAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid validator address: %s", err)
	}

	validator, err := k.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return nil, err
	}

	newShares, err := k.stakingKeeper.Delegate(ctx, toAddr, msg.Amount.Amount, stakingtypes.Unbonded, validator, true)
	if err != nil {
		return nil, err
	}

	// The stakingKepper.Delegate call above does not emit events.
	// The staking module emits the event in the msgServer.Delegate method,
	// which calls the stakingKeeper.Delegate method. So we manually emit the event here.
	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
			stakingtypes.EventTypeDelegate,
			sdk.NewAttribute(stakingtypes.AttributeKeyValidator, msg.ValidatorAddress),
			sdk.NewAttribute(stakingtypes.AttributeKeyDelegator, msg.ToAddress),
			sdk.NewAttribute(sdk.AttributeKeyAmount, msg.Amount.String()),
			sdk.NewAttribute(stakingtypes.AttributeKeyNewShares, newShares.String()),
		),
	})

	_, err = k.Lock(
		goCtx,
		&types.MsgLock{
			Address:    msg.ToAddress,
			UnlockDate: msg.UnlockDate,
			Amount:     msg.Amount,
		},
	)

	if err != nil {
		return nil, err
	}

	return &types.MsgSendDelegateAndLockResponse{}, nil
}
