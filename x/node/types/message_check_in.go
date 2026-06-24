package types

import (
	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var _ sdk.Msg = &MsgCheckIn{}

func NewMsgCheckIn(address string) *MsgCheckIn {
	return &MsgCheckIn{Address: address}
}

func (msg *MsgCheckIn) ValidateBasic() error {
	if msg.Address == "" {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "address cannot be empty")
	}
	if _, err := sdk.AccAddressFromBech32(msg.Address); err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid address: %s", err)
	}
	return nil
}
