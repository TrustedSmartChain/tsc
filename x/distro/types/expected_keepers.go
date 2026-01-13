package types

import (
	"context"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// AccountKeeper defines the expected interface for the Account module.
type AccountKeeper interface {
	GetModuleAddress(moduleName string) sdk.AccAddress
	GetModuleAccount(ctx context.Context, moduleName string) sdk.ModuleAccountI
	SetModuleAccount(context.Context, sdk.ModuleAccountI)
}

// BankKeeper defines the expected interface for the Bank module.
type BankKeeper interface {
	SpendableCoins(context.Context, sdk.AccAddress) sdk.Coins
	MintCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	GetSupply(ctx context.Context, denom string) sdk.Coin
}

// AccountKeeper defines the expected interface for the Account module.
type ViewKeeper interface {
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
}

// ParamSubspace defines the expected Subspace interface for parameters.
type ParamSubspace interface {
	Get(context.Context, []byte, interface{})
	Set(context.Context, []byte, interface{})
}
