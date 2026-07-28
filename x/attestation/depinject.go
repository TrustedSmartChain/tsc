package module

import (
	"os"

	"github.com/cosmos/cosmos-sdk/codec"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	"cosmossdk.io/core/appmodule"
	"cosmossdk.io/core/store"
	"cosmossdk.io/depinject"
	"cosmossdk.io/log"

	networkkeeper "github.com/webstack-sdk/webstack/x/network/keeper"

	modulev1 "github.com/TrustedSmartChain/tsc/v3/api/attestation/module/v1"
	"github.com/TrustedSmartChain/tsc/v3/x/attestation/keeper"
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

	NetworkKeeper networkkeeper.Keeper
}

type ModuleOutputs struct {
	depinject.Out

	Module appmodule.AppModule
	Keeper keeper.Keeper
}

func ProvideModule(in ModuleInputs) ModuleOutputs {
	govAddr := authtypes.NewModuleAddress(govtypes.ModuleName).String()

	k := keeper.NewKeeper(in.Cdc, in.StoreService, log.NewLogger(os.Stderr), govAddr, in.NetworkKeeper)
	m := NewAppModule(in.Cdc, k)

	return ModuleOutputs{Module: m, Keeper: k, Out: depinject.Out{}}
}
