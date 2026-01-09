package keeper

import (
	"context"

	"cosmossdk.io/math"
	"github.com/TrustedSmartChain/tsc/x/lockup/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

func (k msgServer) MultiSendDelegateAndLock(goCtx context.Context, msg *types.MsgMultiSendDelegateAndLock) (*types.MsgMultiSendDelegateAndLockResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	_, err := sdk.AccAddressFromBech32(msg.FromAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid from address: %s", err)
	}

	totalOutputs := math.ZeroInt()
	for _, output := range msg.Outputs {
		if !output.Amount.IsPositive() {
			return nil, sdkerrors.ErrInvalidCoins.Wrapf("invalid amount in output to %s: %s", output.ToAddress, output.Amount.String())
		}
		totalOutputs = totalOutputs.Add(output.Amount.Amount)
	}

	if !msg.TotalAmount.Amount.Equal(totalOutputs) {
		return nil, sdkerrors.ErrInvalidRequest.Wrapf("input %s does not match sum of outputs %s", msg.TotalAmount.String(), totalOutputs.String())
	}

	for _, output := range msg.Outputs {
		sendMsg := &types.MsgSendDelegateAndLock{
			FromAddress:      msg.FromAddress,
			ToAddress:        output.ToAddress,
			ValidatorAddress: output.ValidatorAddress,
			UnlockDate:       output.UnlockDate,
			Amount:           output.Amount,
		}
		_, err := k.SendDelegateAndLock(ctx, sendMsg)
		if err != nil {
			return nil, err
		}
	}

	return &types.MsgMultiSendDelegateAndLockResponse{}, nil
}
