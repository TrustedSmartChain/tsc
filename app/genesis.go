package app

import (
	"encoding/json"

	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	erc20types "github.com/cosmos/evm/x/erc20/types"
	feemarkettypes "github.com/cosmos/evm/x/feemarket/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"

	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"
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
	evmGenState.Params.ActiveStaticPrecompiles = evmtypes.AvailableStaticPrecompiles
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
