package app

import (
	"encoding/json"
	"testing"

	cmttypes "github.com/cometbft/cometbft/types"

	"cosmossdk.io/math"

	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	"github.com/cosmos/cosmos-sdk/testutil/mock"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
	evmtypes "github.com/cosmos/evm/x/vm/types"
	"github.com/cosmos/evm/testutil/integration/evm/network"

	"github.com/TrustedSmartChain/tsc/v4/app/hooks"

	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cosmos/cosmos-sdk/baseapp"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"

	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func TestAppExport(t *testing.T) {
	db := dbm.NewMemDB()
	logger := log.NewTestLogger(t)
	gapp := NewChainApp(
		logger.With("instance", "first"),
		db,
		nil,
		true,
		simtestutil.NewAppOptionsWithFlagHome(t.TempDir()),
		baseapp.SetChainID(ChainID),
	)

	// init chain from the default genesis plus one validator: cosmos/evm v0.7
	// requires genesis to have run (it seeds the process-global evm coin info)
	// before any block is finalized, and InitChain rejects an empty val set.
	privVal := mock.NewPV()
	pubKey, err := privVal.GetPubKey()
	require.NoError(t, err)
	valSet := cmttypes.NewValidatorSet([]*cmttypes.Validator{cmttypes.NewValidator(pubKey, 1)})
	senderPrivKey := secp256k1.GenPrivKey()
	acc := authtypes.NewBaseAccount(senderPrivKey.PubKey().Address().Bytes(), senderPrivKey.PubKey(), 0, 0)
	balance := banktypes.Balance{
		Address: acc.GetAddress().String(),
		Coins:   sdk.NewCoins(sdk.NewCoin(evmtypes.DefaultEVMExtendedDenom, math.NewInt(100000000000000))),
	}

	genesisState, err := simtestutil.GenesisStateWithValSet(gapp.AppCodec(), gapp.DefaultGenesis(), valSet, []authtypes.GenesisAccount{acc}, balance)
	require.NoError(t, err)

	var bankGenesis banktypes.GenesisState
	gapp.AppCodec().MustUnmarshalJSON(genesisState[banktypes.ModuleName], &bankGenesis)
	bankGenesis.DenomMetadata = network.GenerateBankGenesisMetadata(9001)
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
	genesisState[banktypes.ModuleName] = gapp.AppCodec().MustMarshalJSON(&bankGenesis)

	var stakingGenesis stakingtypes.GenesisState
	gapp.AppCodec().MustUnmarshalJSON(genesisState[stakingtypes.ModuleName], &stakingGenesis)
	for i := range stakingGenesis.Validators {
		stakingGenesis.Validators[i].MinSelfDelegation = hooks.MinSelfDelegation
	}
	genesisState[stakingtypes.ModuleName] = gapp.AppCodec().MustMarshalJSON(&stakingGenesis)

	stateBytes, err := json.MarshalIndent(genesisState, "", " ")
	require.NoError(t, err)
	_, err = gapp.InitChain(&abci.RequestInitChain{
		ChainId:         ChainID,
		Validators:      []abci.ValidatorUpdate{},
		AppStateBytes:   stateBytes,
		ConsensusParams: simtestutil.DefaultConsensusParams,
	})
	require.NoError(t, err)

	// finalize block so we have CheckTx state set
	_, err = gapp.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: 1,
	})
	require.NoError(t, err)

	_, err = gapp.Commit()
	require.NoError(t, err)

	// Making a new app object with the db, so that initchain hasn't been called
	newGapp := NewChainApp(
		logger, db, nil, true, simtestutil.NewAppOptionsWithFlagHome(t.TempDir()),
		baseapp.SetChainID(ChainID),
	)
	_, err = newGapp.ExportAppStateAndValidators(false, []string{}, nil)
	require.NoError(t, err, "ExportAppStateAndValidators should not have an error")
}

// ensure that blocked addresses are properly set in bank keeper
func TestBlockedAddrs(t *testing.T) {
	gapp := Setup(t, "chain-test", 9001)

	for acc := range BlockedAddresses() {
		t.Run(acc, func(t *testing.T) {
			var addr sdk.AccAddress
			if modAddr, err := sdk.AccAddressFromBech32(acc); err == nil {
				addr = modAddr
			} else {
				addr = gapp.AccountKeeper.GetModuleAddress(acc)
			}
			require.True(t, gapp.BankKeeper.BlockedAddr(addr), "ensure that blocked addresses are properly set in bank keeper")
		})
	}
}

func TestGetMaccPerms(t *testing.T) {
	dup := GetMaccPerms()
	require.Equal(t, maccPerms, dup, "duplicated module account permissions differed from actual module account permissions")
}

// TestMergedRegistry tests that fetching the gogo/protov2 merged registry
// doesn't fail after loading all file descriptors.
func TestMergedRegistry(t *testing.T) {
	r, err := proto.MergedRegistry()
	require.NoError(t, err)
	require.Greater(t, r.NumFiles(), 0)
}

func TestProtoAnnotations(t *testing.T) {
	r, err := proto.MergedRegistry()
	require.NoError(t, err)
	err = msgservice.ValidateProtoAnnotations(r)
	require.NoError(t, err)
}
