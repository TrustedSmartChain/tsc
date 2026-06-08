package module

import (
	"os"

	"github.com/cosmos/cosmos-sdk/codec"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	epochskeeper "github.com/cosmos/cosmos-sdk/x/epochs/keeper"
	epochstypes "github.com/cosmos/cosmos-sdk/x/epochs/types"
	stakingkeeper "github.com/cosmos/cosmos-sdk/x/staking/keeper"

	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	licenseskeeper "github.com/webstack-sdk/webstack/x/licenses/keeper"

	"cosmossdk.io/core/address"
	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log"

	modulev1 "github.com/TrustedSmartChain/tsc/v3/api/distro/module/v1"
	"github.com/TrustedSmartChain/tsc/v3/x/distro/keeper"
)

var _ appmodule.AppModule = AppModule{}

// IsOnePerModuleType implements the depinject.OnePerModuleType interface.
func (am AppModule) IsOnePerModuleType() {}

// IsAppModule implements the appmodule.AppModule interface.
func (am AppModule) IsAppModule() {}

func init() {
	appmodule.Register(
		&modulev1.Module{},
		appmodule.Provide(ProvideModule),
	)
}

type ModuleInputs struct {
	depinject.In

	Cdc          codec.Codec
	StoreService store.KVStoreService
	AddressCodec address.Codec

	BankKeeper     bankkeeper.Keeper
	AccountKeeper  authkeeper.AccountKeeper
	StakingKeeper  *stakingkeeper.Keeper
	LicensesKeeper licenseskeeper.Keeper
	EpochsKeeper   *epochskeeper.Keeper
}

type ModuleOutputs struct {
	depinject.Out

	Module appmodule.AppModule
	Keeper keeper.Keeper
	// EpochHooks registers the distro tally with x/epochs when wired via
	// depinject (x/epochs collects EpochHooksWrapper outputs via InvokeSetHooks).
	EpochHooks epochstypes.EpochHooksWrapper
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	govAddr := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	k := keeper.NewKeeper(in.Cdc, in.StoreService, log.NewLogger(os.Stderr), govAddr, in.AccountKeeper, in.BankKeeper, in.StakingKeeper, licenseskeeper.NewQuerier(in.LicensesKeeper), in.EpochsKeeper)
	m := NewAppModule(in.Cdc, k)

	return ModuleOutputs{
		Module:     m,
		Keeper:     k,
		EpochHooks: epochstypes.EpochHooksWrapper{EpochHooks: k.EpochHooks()},
		Out:        depinject.Out{},
	}
}
