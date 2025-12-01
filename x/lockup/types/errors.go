package types

import "cosmossdk.io/errors"

var (
	ErrInvalidAccount = errors.Register(ModuleName, 1, "invalid account type for lockup")
)
