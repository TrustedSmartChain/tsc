package types

import (
	"context"

	"cosmossdk.io/core/address"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

type BankKeeper interface {
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
	SpendableCoin(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin

	SendCoins(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) error
}

type AccountKeeper interface {
	HasAccount(ctx context.Context, addr sdk.AccAddress) bool
	GetAccount(ctx context.Context, addr sdk.AccAddress) sdk.AccountI
	SetAccount(context.Context, sdk.AccountI)
	AddressCodec() address.Codec
}

type ParamSubspace interface {
	Get(context.Context, []byte, interface{})
	Set(context.Context, []byte, interface{})
}
