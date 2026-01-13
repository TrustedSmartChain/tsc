package types

import (
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

var _ sdk.Msg = &MsgMint{}

func NewMsgMint(fromAddress string, amount string, minter string) *MsgMint {
	return &MsgMint{
		Amount: amount,
		Minter: minter,
	}
}

// Route returns the name of the module
func (msg MsgMint) Route() string { return ModuleName }

// Type returns the the action
func (msg MsgMint) Type() string { return "mint" }

// GetSignBytes implements the LegacyMsg interface.
func (msg MsgMint) GetSignBytes() []byte {
	return sdk.MustSortJSON(AminoCdc.MustMarshalJSON(&msg))
}

// GetSigners returns the signer address for a MsgMint message.
func (msg *MsgMint) GetSigners() []sdk.AccAddress {
	addr, _ := sdk.AccAddressFromBech32(msg.Minter)
	return []sdk.AccAddress{addr}
}

func (msg *MsgMint) Validate() (math.Int, error) {
	amount, ok := math.NewIntFromString(msg.Amount)
	if !ok {
		return math.ZeroInt(), errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "amount cannot be converted to int")
	}

	if amount.IsZero() {
		return math.ZeroInt(), errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "amount cannot be zero")
	}

	if amount.IsNegative() {
		return math.ZeroInt(), errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "amount cannot be negative")
	}

	if msg.Minter == "" {
		return math.ZeroInt(), errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "minter cannot be empty")
	}

	_, err := sdk.AccAddressFromBech32(msg.Minter)
	if err != nil {
		return math.ZeroInt(), errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "minter is not a valid address")
	}

	return amount, nil
}
