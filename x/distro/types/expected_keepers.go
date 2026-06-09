package types

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	epochstypes "github.com/cosmos/cosmos-sdk/x/epochs/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	licensetypes "github.com/webstack-sdk/webstack/x/licenses/types"
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
	BurnCoins(ctx context.Context, moduleName string, amt sdk.Coins) error
	SendCoinsFromModuleToAccount(ctx context.Context, senderModule string, recipientAddr sdk.AccAddress, amt sdk.Coins) error
	SendCoinsFromAccountToModule(ctx context.Context, senderAddr sdk.AccAddress, recipientModule string, amt sdk.Coins) error
	GetSupply(ctx context.Context, denom string) sdk.Coin
	GetBalance(ctx context.Context, addr sdk.AccAddress, denom string) sdk.Coin
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

// StakingKeeper defines the expected interface for the Staking module, used for
// the stake-weighted distribution tally. Weight is measured per delegator
// address; a validator-operator address counts only its self-delegation.
type StakingKeeper interface {
	TotalBondedTokens(ctx context.Context) (math.Int, error)
	GetValidator(ctx context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error)
	GetDelegation(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) (stakingtypes.Delegation, error)
	GetDelegatorDelegations(ctx context.Context, delegator sdk.AccAddress, maxRetrieve uint16) ([]stakingtypes.Delegation, error)
}

// LicensesKeeper defines the expected interface for the x/licenses module. It is
// satisfied by licenseskeeper.NewQuerier(licensesKeeper). distro counts only
// licenses whose status is "active".
type LicensesKeeper interface {
	LicensesByHolderAndType(ctx context.Context, req *licensetypes.QueryLicensesByHolderAndTypeRequest) (*licensetypes.QueryLicensesByHolderAndTypeResponse, error)
	LicensesByType(ctx context.Context, req *licensetypes.QueryLicensesByTypeRequest) (*licensetypes.QueryLicensesByTypeResponse, error)
}

// EpochsKeeper defines the expected interface for the x/epochs module, used to
// bound the epoch a node may submit a root for to the current epoch.
type EpochsKeeper interface {
	GetEpochInfo(ctx sdk.Context, identifier string) (epochstypes.EpochInfo, error)
}
