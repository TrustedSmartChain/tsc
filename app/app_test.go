package app

import (
	"testing"

	abci "github.com/cometbft/cometbft/abci/types"
	dbm "github.com/cosmos/cosmos-db"
	"github.com/cosmos/gogoproto/proto"
	"github.com/stretchr/testify/require"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

func TestAppExport(t *testing.T) {
	const (
		chainID    = "chain-test"
		evmChainID = uint64(9001)
	)

	// Initialize a chain (InitChain seeds the consensus params and genesis
	// state), then finalize + commit the first block so that genesis state is
	// flushed to the committed store, and export it.
	//
	// NOTE: InitChain writes genesis to the finalize state; Commit alone does
	// not flush it — a FinalizeBlock is required first. And because cosmos/evm
	// keeps a process-global EVM chainConfig that is set once per process (the
	// non-`test` build), we cannot spin up a second ChainApp to export from a
	// cold reload, so we export from this same committed app.
	db := dbm.NewMemDB()
	gapp := SetupWithDB(t, db, chainID, evmChainID)

	_, err := gapp.FinalizeBlock(&abci.RequestFinalizeBlock{Height: gapp.LastBlockHeight() + 1})
	require.NoError(t, err)
	_, err = gapp.Commit()
	require.NoError(t, err)

	_, err = gapp.ExportAppStateAndValidators(false, []string{}, nil)
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
