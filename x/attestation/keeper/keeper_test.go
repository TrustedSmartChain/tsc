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

	networktypes "github.com/nodelabs-sdk/nodelabs/x/network/types"

	module "github.com/TrustedSmartChain/tsc/v4/x/attestation"
	"github.com/TrustedSmartChain/tsc/v4/x/attestation/keeper"
	"github.com/TrustedSmartChain/tsc/v4/x/attestation/types"
)

// fixtureBlockTime is the fixture context's initial block time; tests that
// roll days advance from here via WithBlockTime.
var fixtureBlockTime = time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

// Node types used by the fixture, aliased to the module constants the attest
// messages name. nodeTypeTrust is the tier allowed to RWA-attest; nodeTypeNano
// is allowed RWU only.
const (
	nodeTypeTrust = types.NodeTypeTrust
	nodeTypeNano  = types.NodeTypeNano
)

// fakeNetworkKeeper implements types.NetworkKeeper for keeper tests.
//
// It models the two license gates separately because x/network does: the
// any-type gate gets a per-operator flag, the per-node-type gate a per
// (operator, node type) one. A node type absent from registered is not
// registered at all, which is a different rejection from an operator holding
// none of its licenses.
type fakeNetworkKeeper struct {
	nodes      map[string]networktypes.Node
	unlicensed map[string]bool // operators without licenses of any type
	registered map[string]bool // node types registered in the network registry
	// licensed maps operator -> node type -> holds a license of the type bound
	// to that node type.
	licensed map[string]map[string]bool
	touched  []string // TouchNodeActivity calls, in order
}

func newFakeNetworkKeeper() *fakeNetworkKeeper {
	return &fakeNetworkKeeper{
		nodes:      make(map[string]networktypes.Node),
		unlicensed: make(map[string]bool),
		registered: make(map[string]bool),
		licensed:   make(map[string]map[string]bool),
	}
}

// license marks operator as holding a license of the type backing nodeType,
// and registers nodeType if it was not already.
func (f *fakeNetworkKeeper) license(operator, nodeType string) {
	f.register(nodeType)
	if f.licensed[operator] == nil {
		f.licensed[operator] = make(map[string]bool)
	}
	f.licensed[operator][nodeType] = true
}

// register records nodeType as existing in x/network. Registration and
// licensing are separate: a node type can be registered with nobody licensed
// for it.
func (f *fakeNetworkKeeper) register(nodeType string) {
	f.registered[nodeType] = true
}

// revoke drops operator's license for nodeType, leaving the node type
// registered — the shape a revocation takes on a live chain.
func (f *fakeNetworkKeeper) revoke(operator, nodeType string) {
	if f.licensed[operator] != nil {
		delete(f.licensed[operator], nodeType)
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

// EnsureOperatorLicensedForNodeType mirrors x/network: an unregistered node
// type and an operator holding none of its licenses are distinct failures.
func (f *fakeNetworkKeeper) EnsureOperatorLicensedForNodeType(_ context.Context, operator, nodeType string) error {
	if !f.registered[nodeType] {
		return networktypes.ErrInvalidNodeType.Wrapf("node type %q is not registered", nodeType)
	}
	if !f.licensed[operator][nodeType] {
		return networktypes.ErrNoActiveLicenses.Wrapf("operator %s holds no active licenses backing %q", operator, nodeType)
	}
	return nil
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

	keys := storetypes.NewKVStoreKeys(types.ModuleName)
	f.ctx = sdk.NewContext(integration.CreateMultiStore(keys, logger), cmtproto.Header{Height: 1, Time: fixtureBlockTime}, false, logger)

	f.k = keeper.NewKeeper(encCfg.Codec, runtime.NewKVStoreService(keys[types.ModuleName]), logger, f.govModAddr, f.network)
	f.msgServer = keeper.NewMsgServerImpl(f.k)
	f.queryServer = keeper.NewQuerier(f.k)

	require.NoError(t, f.k.InitGenesis(f.ctx, types.DefaultGenesis()))

	// Both fixture tiers exist in x/network. Which of them may attest is not
	// state at all — the messages name it — so there is nothing else to seed.
	f.network.register(nodeTypeTrust)
	f.network.register(nodeTypeNano)

	_ = module.NewAppModule(encCfg.Codec, f.k)

	return f
}

// addNode registers an active-or-not node AND licenses its operator for the
// node's declared type, which is the licensed, non-escalated case.
//
// Tests that model an unregistered node type, or an operator declaring a type
// it is not licensed for, call f.network.addNode directly and leave
// f.network.license unset for it.
func (f *testFixture) addNode(nodeType string, status networktypes.NodeStatus) networktypes.Node {
	node := f.network.addNode(nodeType, status)
	f.network.license(node.Operator, nodeType)
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
