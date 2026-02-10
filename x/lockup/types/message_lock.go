package types

import (
	"time"

	errorsmod "cosmossdk.io/errors"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var _ sdk.Msg = &MsgLock{}

func NewMsgLock(lockupAddress string, unlockDate string, amount sdk.Coin) *MsgLock {
	return &MsgLock{
		Address:    lockupAddress,
		UnlockDate: unlockDate,
		Amount:     amount,
	}
}

func (msg *MsgLock) ValidateBasic() error {
	_, err := sdk.AccAddressFromBech32(msg.Address)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid lockupAddress address (%s)", err)
	}

	if msg.UnlockDate == "" {
		return errorsmod.Wrapf(ErrInvalidDate, "invalid unlock date")
	}
	_, err = time.Parse(time.DateOnly, msg.UnlockDate)
	if err != nil {
		return errorsmod.Wrapf(ErrInvalidDate, "invalid unlock date format: %s", err)
	}

	if !msg.Amount.IsPositive() {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidCoins, "invalid lock amount: %s", msg.Amount.String())
	}

	return nil
}
