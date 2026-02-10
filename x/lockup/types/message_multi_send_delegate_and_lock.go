package types

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var _ sdk.Msg = &MsgMultiSendDelegateAndLock{}

func NewMsgMultiSendDelegateAndLock(fromAddress string, TotalAmount sdk.Coin, outputs []*MultiSendDelegateAndLockOutput) *MsgMultiSendDelegateAndLock {
	return &MsgMultiSendDelegateAndLock{
		FromAddress: fromAddress,
		TotalAmount: TotalAmount,
		Outputs:     outputs,
	}
}

func (msg *MsgMultiSendDelegateAndLock) ValidateBasic() error {
	_, err := sdk.AccAddressFromBech32(msg.FromAddress)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid fromAddress address (%s)", err)
	}
	return nil
}
