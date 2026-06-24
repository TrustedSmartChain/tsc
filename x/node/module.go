package module

import (
	"encoding/json"

	"github.com/gorilla/mux"
	"github.com/grpc-ecosystem/grpc-gateway/runtime"

	abci "github.com/cometbft/cometbft/abci/types"

	"github.com/spf13/cobra"
	"cosmossdk.io/core/appmodule"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/codec"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/module"

	nodecli "github.com/TrustedSmartChain/tsc/v3/x/node/client/cli"
	"github.com/TrustedSmartChain/tsc/v3/x/node/keeper"
	"github.com/TrustedSmartChain/tsc/v3/x/node/types"
)

const ConsensusVersion = 1

var (
	_ module.AppModuleBasic   = AppModuleBasic{}
	_ module.AppModuleGenesis = AppModule{}
	_ module.AppModule        = AppModule{}
	_ appmodule.AppModule     = AppModule{}
)

type AppModuleBasic struct {
	cdc codec.Codec
}

type AppModule struct {
	AppModuleBasic
}

func NewAppModule(cdc codec.Codec) *AppModule {
	return &AppModule{
		AppModuleBasic: AppModuleBasic{cdc: cdc},
	}
}

func (a AppModuleBasic) Name() string {
	return types.ModuleName
}

func (a AppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(types.DefaultGenesis())
}

func (a AppModuleBasic) ValidateGenesis(cdc codec.JSONCodec, _ client.TxEncodingConfig, bz json.RawMessage) error {
	var gs types.GenesisState
	if err := cdc.UnmarshalJSON(bz, &gs); err != nil {
		return err
	}
	return gs.Validate()
}

func (a AppModuleBasic) GetTxCmd() *cobra.Command {
	return nodecli.NewTxCmd()
}

func (a AppModuleBasic) RegisterRESTRoutes(_ client.Context, _ *mux.Router) {}

func (a AppModuleBasic) RegisterGRPCGatewayRoutes(_ client.Context, _ *runtime.ServeMux) {}

func (AppModuleBasic) RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	types.RegisterLegacyAminoCodec(cdc)
}

func (a AppModuleBasic) RegisterInterfaces(r codectypes.InterfaceRegistry) {
	types.RegisterInterfaces(r)
}

func (a AppModule) InitGenesis(_ sdk.Context, _ codec.JSONCodec, _ json.RawMessage) []abci.ValidatorUpdate {
	return nil
}

func (a AppModule) ExportGenesis(_ sdk.Context, cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(types.DefaultGenesis())
}

func (a AppModule) RegisterInvariants(_ sdk.InvariantRegistry) {}

func (a AppModule) QuerierRoute() string {
	return types.QuerierRoute
}

func (a AppModule) RegisterServices(cfg module.Configurator) {
	types.RegisterMsgServer(cfg.MsgServer(), keeper.NewMsgServerImpl())
}

func (a AppModule) ConsensusVersion() uint64 {
	return ConsensusVersion
}

func (am AppModule) IsOnePerModuleType() {}

func (am AppModule) IsAppModule() {}
