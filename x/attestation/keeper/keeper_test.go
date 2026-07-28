package keeper_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"

	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil/integration"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	moduletestutil "github.com/cosmos/cosmos-sdk/types/module/testutil"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	networktypes "github.com/webstack-sdk/webstack/x/network/types"

	module "github.com/TrustedSmartChain/tsc/v3/x/attestation"
	"github.com/TrustedSmartChain/tsc/v3/x/attestation/keeper"
	"github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

// fixtureBlockTime is the fixture context's initial block time; tests that
// roll days advance from here via WithBlockTime.
var fixtureBlockTime = time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

// fakeNetworkKeeper implements types.NetworkKeeper for keeper tests.
type fakeNetworkKeeper struct {
	nodes      map[string]networktypes.Node
	unlicensed map[string]bool // operators without licenses
	touched    []string        // TouchNodeActivity calls, in order
}

func newFakeNetworkKeeper() *fakeNetworkKeeper {
	return &fakeNetworkKeeper{
		nodes:      make(map[string]networktypes.Node),
		unlicensed: make(map[string]bool),
	}
}

func (f *fakeNetworkKeeper) IsActiveNode(_ context.Context, nodeAddr string) (networktypes.Node, bool, error) {
	node, ok := f.nodes[nodeAddr]
	if !ok {
		return networktypes.Node{}, false, nil
	}
	return node, node.Status == networktypes.NodeActive, nil
}

func (f *fakeNetworkKeeper) TouchNodeActivity(_ context.Context, nodeAddr string) error {
	f.touched = append(f.touched, nodeAddr)
	return nil
}

func (f *fakeNetworkKeeper) EnsureOperatorLicensed(_ context.Context, operator string) error {
	if f.unlicensed[operator] {
		return networktypes.ErrNoActiveLicenses.Wrapf("operator %s", operator)
	}
	return nil
}

// fakeLicenseKeeper implements types.LicenseKeeper for keeper tests. It
// records which license types each holder holds, so a test can model an
// operator that holds a nano license while declaring a trust node.
type fakeLicenseKeeper struct {
	// held maps holder -> license type -> count.
	held map[string]map[string]uint64
}

func newFakeLicenseKeeper() *fakeLicenseKeeper {
	return &fakeLicenseKeeper{held: make(map[string]map[string]uint64)}
}

func (f *fakeLicenseKeeper) issue(holder, licenseType string, count uint64) {
	if f.held[holder] == nil {
		f.held[holder] = make(map[string]uint64)
	}
	f.held[holder][licenseType] += count
}

func (f *fakeLicenseKeeper) revoke(holder, licenseType string) {
	if f.held[holder] != nil {
		delete(f.held[holder], licenseType)
	}
}

func (f *fakeLicenseKeeper) CountActiveLicenses(_ context.Context, holder string, licenseTypes []string, stopAt uint64) (uint64, error) {
	var count uint64
	for _, t := range licenseTypes {
		count += f.held[holder][t]
		if stopAt != 0 && count >= stopAt {
			return count, nil
		}
	}
	return count, nil
}

// addNode registers a node with the fake network keeper and returns its
// address.
func (f *fakeNetworkKeeper) addNode(nodeType string, status networktypes.NodeStatus) networktypes.Node {
	addrs := simtestutil.CreateIncrementalAccounts(len(f.nodes) + 2)
	node := networktypes.Node{
		Address:  addrs[len(addrs)-1].String(),
		Operator: addrs[len(addrs)-2].String(),
		Type:     nodeType,
		Status:   status,
	}
	f.nodes[node.Address] = node
	return node
}

type testFixture struct {
	ctx         sdk.Context
	k           keeper.Keeper
	msgServer   types.MsgServer
	queryServer types.QueryServer

	network    *fakeNetworkKeeper
	license    *fakeLicenseKeeper
	govModAddr string
}

func SetupTest(t *testing.T) *testFixture {
	t.Helper()
	f := new(testFixture)

	logger := log.NewTestLogger(t)
	encCfg := moduletestutil.MakeTestEncodingConfig()
	types.RegisterInterfaces(encCfg.InterfaceRegistry)

	f.govModAddr = authtypes.NewModuleAddress(govtypes.ModuleName).String()
	f.network = newFakeNetworkKeeper()
	f.license = newFakeLicenseKeeper()

	keys := storetypes.NewKVStoreKeys(types.ModuleName)
	f.ctx = sdk.NewContext(integration.CreateMultiStore(keys, logger), cmtproto.Header{Height: 1, Time: fixtureBlockTime}, false, logger)

	f.k = keeper.NewKeeper(encCfg.Codec, runtime.NewKVStoreService(keys[types.ModuleName]), logger, f.govModAddr, f.network, f.license)
	f.msgServer = keeper.NewMsgServerImpl(f.k)
	f.queryServer = keeper.NewQuerier(f.k)

	require.NoError(t, f.k.InitGenesis(f.ctx, types.DefaultGenesis()))

	_ = module.NewAppModule(encCfg.Codec, f.k)

	return f
}

// addNode registers an active-or-not node AND issues its operator one
// license of the node's declared type, which is the licensed, non-escalated
// case. Tests that model an operator declaring a type it is not licensed for
// call f.network.addNode directly and manage f.license themselves.
func (f *testFixture) addNode(nodeType string, status networktypes.NodeStatus) networktypes.Node {
	node := f.network.addNode(nodeType, status)
	f.license.issue(node.Operator, nodeType, 1)
	return node
}

// WithBlockTime returns the fixture context advanced to the given block
// time.
func (f *testFixture) WithBlockTime(t time.Time) sdk.Context {
	header := f.ctx.BlockHeader()
	header.Time = t
	header.Height++
	return f.ctx.WithBlockHeader(header)
}
