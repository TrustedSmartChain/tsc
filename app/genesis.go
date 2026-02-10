package app

import (
	"encoding/json"
	"sort"

	lockupprecompile "github.com/TrustedSmartChain/tsc/precompiles/lockup"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/x/bank"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	"github.com/cosmos/cosmos-sdk/x/mint"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
	"github.com/cosmos/cosmos-sdk/x/staking"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	erc20types "github.com/cosmos/evm/x/erc20/types"
	feemarkettypes "github.com/cosmos/evm/x/feemarket/types"
	"github.com/cosmos/evm/x/vm"
	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// GenesisState of the blockchain is represented here as a map of raw json
// messages key'd by an identifier string.
// The identifier is used to determine which module genesis information belongs
// to so it may be appropriately routed during init chain.
// Within this application default genesis information is retrieved from
// the ModuleBasicManager which populates json from each BasicModule
// object provided to it during init.
type GenesisState map[string]json.RawMessage

// NewEVMGenesisState returns the default genesis state for the EVM module.
func NewEVMGenesisState() *evmtypes.GenesisState {
	evmGenState := evmtypes.DefaultGenesisState()

	// Set the EVM denom to the chain's base denomination
	evmGenState.Params.EvmDenom = BaseDenom
	evmGenState.Params.ExtendedDenomOptions = &evmtypes.ExtendedDenomOptions{ExtendedDenom: BaseDenom}

	// Include the default precompiles plus the custom lockup precompile
	activePrecompiles := append(evmtypes.AvailableStaticPrecompiles, lockupprecompile.LockupPrecompileAddress) //nolint:gocritic // append to new slice
	sort.Strings(activePrecompiles)
	evmGenState.Params.ActiveStaticPrecompiles = activePrecompiles
	evmGenState.Preinstalls = evmtypes.DefaultPreinstalls

	return evmGenState
}

// NewErc20GenesisState returns the default genesis state for the ERC20 module.
func NewErc20GenesisState() *erc20types.GenesisState {
	erc20GenState := erc20types.DefaultGenesisState()

	return erc20GenState
}

// NewMintGenesisState returns the default genesis state for the mint module.
func NewMintGenesisState() *minttypes.GenesisState {
	mintGenState := minttypes.DefaultGenesisState()

	mintGenState.Params.MintDenom = BaseDenom
	return mintGenState
}

// NewFeeMarketGenesisState returns the default genesis state for the feemarket module.
func NewFeeMarketGenesisState() *feemarkettypes.GenesisState {
	feeMarketGenState := feemarkettypes.DefaultGenesisState()

	return feeMarketGenState
}

// NewStakingGenesisState returns the default genesis state for the staking module.
func NewStakingGenesisState() *stakingtypes.GenesisState {
	stakingGenState := stakingtypes.DefaultGenesisState()
	stakingGenState.Params.BondDenom = BaseDenom

	return stakingGenState
}

// NewBankGenesisState returns the default genesis state for the bank module.
func NewBankGenesisState() *banktypes.GenesisState {
	bankGenState := banktypes.DefaultGenesisState()
	bankGenState.DenomMetadata = []banktypes.Metadata{
		{
			Description: "The native staking token of the TSC network.",
			DenomUnits: []*banktypes.DenomUnit{
				{Denom: BaseDenom, Exponent: 0, Aliases: []string{"atsc"}},
				{Denom: DisplayDenom, Exponent: uint32(BaseDenomUnit), Aliases: []string{}},
			},
			Base:    BaseDenom,
			Display: DisplayDenom,
			Name:    DisplayDenom,
			Symbol:  DisplayDenom,
		},
	}

	return bankGenState
}

// ---------------------------------------------------------------------------
// Custom AppModuleBasic wrappers
// ---------------------------------------------------------------------------
// These embed the upstream AppModuleBasic and override only DefaultGenesis
// so that `tscd init` produces a genesis with the chain's custom defaults
// (e.g. the lockup precompile in ActiveStaticPrecompiles, correct bond denom,
// denom metadata, etc.) without relying on shell-script jq overrides.

// evmAppModuleBasic wraps the EVM module's AppModuleBasic.
type evmAppModuleBasic struct{ vm.AppModuleBasic }

func (evmAppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(NewEVMGenesisState())
}

// stakingAppModuleBasic wraps the staking module's AppModuleBasic.
type stakingAppModuleBasic struct{ staking.AppModuleBasic }

func (stakingAppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(NewStakingGenesisState())
}

// bankAppModuleBasic wraps the bank module's AppModuleBasic.
type bankAppModuleBasic struct{ bank.AppModuleBasic }

func (bankAppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(NewBankGenesisState())
}

// mintAppModuleBasic wraps the mint module's AppModuleBasic.
type mintAppModuleBasic struct{ mint.AppModuleBasic }

func (mintAppModuleBasic) DefaultGenesis(cdc codec.JSONCodec) json.RawMessage {
	return cdc.MustMarshalJSON(NewMintGenesisState())
}
