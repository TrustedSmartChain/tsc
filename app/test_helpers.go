package app

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/cosmos/evm/config"
	"github.com/cosmos/evm/testutil/integration/evm/network"
	"github.com/cosmos/evm/x/vm/types"

	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"
	cmttypes "github.com/cometbft/cometbft/types"

	dbm "github.com/cosmos/cosmos-db"
	feemarkettypes "github.com/cosmos/evm/x/feemarket/types"
	ibctesting "github.com/cosmos/ibc-go/v10/testing"

	"cosmossdk.io/log"
	"cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/server"
	servertypes "github.com/cosmos/cosmos-sdk/server/types"
	"github.com/cosmos/cosmos-sdk/testutil/mock"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/TrustedSmartChain/tsc/v3/app/hooks"
)

// SetupOptions defines arguments that are passed into `Simapp` constructor.
type SetupOptions struct {
	Logger  log.Logger
	DB      *dbm.MemDB
	AppOpts servertypes.AppOptions
}

func init() {
	// we're setting the minimum gas price to 0 to simplify the tests
	feemarkettypes.DefaultMinGasPrice = math.LegacyZeroDec()

	// Set the global SDK config for the tests. The bech32 prefixes are this
	// chain's, not cosmos/evm's example ones — module defaults carry tsc1
	// addresses that must parse under the global config.
	cfg := sdk.GetConfig()
	cfg.SetBech32PrefixForAccount(Bech32PrefixAccAddr, Bech32PrefixAccPub)
	cfg.SetBech32PrefixForValidator(Bech32PrefixValAddr, Bech32PrefixValPub)
	cfg.SetBech32PrefixForConsensusNode(Bech32PrefixConsAddr, Bech32PrefixConsPub)
	config.SetBip44CoinType(cfg)
}

func setup(withGenesis bool, invCheckPeriod uint, chainID string, evmChainID uint64) (*ChainApp, GenesisState) {
	db := dbm.NewMemDB()

	appOptions := make(simtestutil.AppOptionsMap, 0)
	appOptions[flags.FlagHome] = DefaultNodeHome
	appOptions[server.FlagInvCheckPeriod] = invCheckPeriod

	app := NewChainApp(log.NewNopLogger(), db, nil, true, appOptions, baseapp.SetChainID(chainID))
	if withGenesis {
		return app, app.DefaultGenesis()
	}

	return app, GenesisState{}
}

// Setup initializes a new EVMD. A Nop logger is set in EVMD.
func Setup(t *testing.T, chainID string, evmChainID uint64) *ChainApp {
	t.Helper()

	privVal := mock.NewPV()
	pubKey, err := privVal.GetPubKey()
	require.NoError(t, err)

	// create validator set with single validator
	validator := cmttypes.NewValidator(pubKey, 1)
	valSet := cmttypes.NewValidatorSet([]*cmttypes.Validator{validator})

	// generate genesis account
	senderPrivKey := secp256k1.GenPrivKey()
	acc := authtypes.NewBaseAccount(senderPrivKey.PubKey().Address().Bytes(), senderPrivKey.PubKey(), 0, 0)
	balance := banktypes.Balance{
		Address: acc.GetAddress().String(),
		Coins:   sdk.NewCoins(sdk.NewCoin(types.DefaultEVMExtendedDenom, math.NewInt(100000000000000))),
	}

	app := SetupWithGenesisValSet(t, chainID, evmChainID, valSet, []authtypes.GenesisAccount{acc}, balance)

	return app
}

// SetupWithGenesisValSet initializes a new EVMD with a validator set and genesis accounts
// that also act as delegators. For simplicity, each validator is bonded with a delegation
// of one consensus engine unit in the default token of the simapp from first genesis
// account. A Nop logger is set in EVMD.
func SetupWithGenesisValSet(t *testing.T, chainID string, evmChainID uint64, valSet *cmttypes.ValidatorSet, genAccs []authtypes.GenesisAccount, balances ...banktypes.Balance) *ChainApp {
	t.Helper()

	app, genesisState := setup(true, 5, chainID, evmChainID)
	genesisState, err := simtestutil.GenesisStateWithValSet(app.AppCodec(), genesisState, valSet, genAccs, balances...)
	var bankGenesis banktypes.GenesisState
	app.AppCodec().MustUnmarshalJSON(genesisState[banktypes.ModuleName], &bankGenesis)
	require.NoError(t, err)
	bankGenesis.DenomMetadata = network.GenerateBankGenesisMetadata(evmChainID)
	// The generated metadata covers cosmos/evm's example chain ids, not this
	// chain's denom; the evm module's InitGenesis loads its coin info from
	// the bank metadata of the evm denom (aTSC) and fails without it.
	bankGenesis.DenomMetadata = append(bankGenesis.DenomMetadata, banktypes.Metadata{
		Description: "The native token of TSC",
		Base:        BaseDenom,
		Display:     DisplayDenom,
		Name:        DisplayDenom,
		Symbol:      DisplayDenom,
		DenomUnits: []*banktypes.DenomUnit{
			{Denom: BaseDenom, Exponent: 0},
			{Denom: DisplayDenom, Exponent: 18},
		},
	})
	genesisState[banktypes.ModuleName] = app.AppCodec().MustMarshalJSON(&bankGenesis)
	require.NoError(t, err)

	// GenesisStateWithValSet seeds validators with a zero min_self_delegation,
	// which InitChainer's ValidateStakingGenesis rejects. Raise the field to
	// the chain floor.
	var stakingGenesis stakingtypes.GenesisState
	app.AppCodec().MustUnmarshalJSON(genesisState[stakingtypes.ModuleName], &stakingGenesis)
	for i := range stakingGenesis.Validators {
		stakingGenesis.Validators[i].MinSelfDelegation = hooks.MinSelfDelegation
	}
	genesisState[stakingtypes.ModuleName] = app.AppCodec().MustMarshalJSON(&stakingGenesis)

	stateBytes, err := json.MarshalIndent(genesisState, "", " ")
	require.NoError(t, err)

	// init chain will set the validator set and initialize the genesis accounts
	if _, err = app.InitChain(
		&abci.RequestInitChain{
			Validators:      []abci.ValidatorUpdate{},
			ConsensusParams: simtestutil.DefaultConsensusParams,
			AppStateBytes:   stateBytes,
			ChainId:         chainID,
		},
	); err != nil {
		panic(fmt.Sprintf("app.InitChain failed: %v", err))
	}

	// NOTE: we are NOT committing the changes here as opposed to the function from simapp
	// because that would already adjust e.g. the base fee in the params.
	// We want to keep the genesis state as is for the tests unless we commit the changes manually.

	return app
}

// SetupTestingApp initializes the IBC-go testing application
// need to keep this design to comply with the ibctesting SetupTestingApp func
// and be able to set the chainID for the tests properly
func SetupTestingApp(chainID string) func() (ibctesting.TestingApp, map[string]json.RawMessage) {
	return func() (ibctesting.TestingApp, map[string]json.RawMessage) {
		db := dbm.NewMemDB()
		app := NewChainApp(
			log.NewNopLogger(),
			db, nil, true,
			simtestutil.NewAppOptionsWithFlagHome(DefaultNodeHome),
			baseapp.SetChainID(chainID),
		)
		return app, app.DefaultGenesis()
	}
}
