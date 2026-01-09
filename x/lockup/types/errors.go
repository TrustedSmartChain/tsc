package types

import (
	sdkerrors "cosmossdk.io/errors"
)

var (
	ErrInvalidSigner           = sdkerrors.Register(ModuleName, 1100, "expected gov account as only signer for proposal message")
	ErrInvalidAccount          = sdkerrors.Register(ModuleName, 1101, "invalid account type for lockup")
	ErrInsufficientDelegations = sdkerrors.Register(ModuleName, 1102, "insufficient delegations amount to cover locked amount")
	ErrLockupNotFound          = sdkerrors.Register(ModuleName, 1103, "lockup not found")
	ErrInvalidDate             = sdkerrors.Register(ModuleName, 1104, "invalid date")
	ErrInvalidAmount           = sdkerrors.Register(ModuleName, 1105, "invalid amount")
)
