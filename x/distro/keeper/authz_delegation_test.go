package keeper_test

import (
	"context"
	"testing"
	"time"

	"cosmossdk.io/collections"
	coreaddress "cosmossdk.io/core/address"
	"cosmossdk.io/log"
	"cosmossdk.io/math"
	storetypes "cosmossdk.io/store/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/baseapp"
	"github.com/cosmos/cosmos-sdk/codec"
	sdkaddress "github.com/cosmos/cosmos-sdk/codec/address"
	codectestutil "github.com/cosmos/cosmos-sdk/codec/testutil"
	"github.com/cosmos/cosmos-sdk/runtime"
	"github.com/cosmos/cosmos-sdk/testutil/integration"
	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	authzkeeper "github.com/cosmos/cosmos-sdk/x/authz/keeper"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"
	licensetypes "github.com/webstack-sdk/webstack/x/licenses/types"

	"github.com/TrustedSmartChain/tsc/v3/app"
	"github.com/TrustedSmartChain/tsc/v3/x/distro/keeper"
	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// mockAuthzAccountKeeper satisfies authz's expected AccountKeeper. DispatchActions
// only needs the address codec; the account methods are unused here.
type mockAuthzAccountKeeper struct{}

func (mockAuthzAccountKeeper) AddressCodec() coreaddress.Codec {
	return sdkaddress.NewBech32Codec(app.Bech32PrefixAccAddr)
}
func (mockAuthzAccountKeeper) GetAccount(context.Context, sdk.AccAddress) sdk.AccountI { return nil }
func (mockAuthzAccountKeeper) NewAccountWithAddress(context.Context, sdk.AccAddress) sdk.AccountI {
	return nil
}
func (mockAuthzAccountKeeper) SetAccount(context.Context, sdk.AccountI) {}

// TestVoteDelegationViaAuthz proves the node-key delegation pattern end-to-end
// through the real x/authz keeper and message router: a node, authorized by an
// owner, submits a distribution root with its own key; the vote is recorded
// under the OWNER (the inner-message signer), and the tally counts the OWNER's
// license and stake weight. The node holds no license, stake, or owner key.
func TestVoteDelegationViaAuthz(t *testing.T) {
	distroKey := storetypes.NewKVStoreKey(types.StoreKey)
	authzKey := storetypes.NewKVStoreKey("authz")
	storeKeys := map[string]*storetypes.KVStoreKey{types.StoreKey: distroKey, "authz": authzKey}

	logger := log.NewNopLogger()
	ctx := sdk.NewContext(integration.CreateMultiStore(storeKeys, logger), cmtproto.Header{Height: 10, Time: time.Unix(1, 0)}, false, logger)

	// Build a codec whose signing context uses the chain's "tsc" address prefix,
	// so authz can decode the inner message's signer.
	ir := codectestutil.CodecOptions{
		AccAddressPrefix: app.Bech32PrefixAccAddr,
		ValAddressPrefix: app.Bech32PrefixValAddr,
	}.NewInterfaceRegistry()
	types.RegisterInterfaces(ir)
	authz.RegisterInterfaces(ir)
	cdc := codec.NewProtoCodec(ir)

	accs := simtestutil.CreateIncrementalAccounts(2)
	owner, valOwner := accs[0], accs[1]
	node := sdk.AccAddress([]byte("distro-node-key-0001")) // own key, nothing else

	// Owner is the sole licensed staker (100% of both tallies); node has neither.
	staking := mockStakingKeeper{
		totalBonded: math.NewInt(100),
		valAddr:     sdk.ValAddress(valOwner),
		validator:   unitValidator(sdk.ValAddress(valOwner)),
		stake:       map[string]math.Int{owner.String(): math.NewInt(100)},
	}
	licenses := mockLicensesKeeper{licenses: []licensetypes.License{activeLicense(owner.String())}}

	k := keeper.NewKeeper(
		cdc,
		runtime.NewKVStoreService(distroKey),
		logger,
		authtypes.NewModuleAddress(govtypes.ModuleName).String(),
		mockAccountKeeper{},
		newMockBank(),
		staking,
		licenses,
		mockEpochsKeeper{current: 1},
	)
	params := types.DefaultParams()
	params.Denom = testDenom
	params.DistributionLicenseTypeId = testLicenseType
	params.ChallengeBond = testBond
	require.NoError(t, k.Params.Set(ctx, params))

	// Wire the real authz keeper to a router that routes the distro Msg service.
	router := baseapp.NewMsgServiceRouter()
	router.SetInterfaceRegistry(ir)
	types.RegisterMsgServer(router, keeper.NewMsgServerImpl(k))
	authzKeeper := authzkeeper.NewKeeper(runtime.NewKVStoreService(authzKey), cdc, router, mockAuthzAccountKeeper{})

	const epoch = int64(1)
	root := []byte("authz-delegated-root------------")
	submit := &types.MsgSubmitDistributionRoot{Signer: owner.String(), Epoch: epoch, MerkleRoot: root}
	msgURL := sdk.MsgTypeURL(submit)

	// 1. Without a grant the node cannot vote for the owner.
	_, err := authzKeeper.DispatchActions(ctx, node, []sdk.Msg{submit})
	require.Error(t, err)

	// 2. Owner grants the node; the node executes the vote with its own key.
	require.NoError(t, authzKeeper.SaveGrant(ctx, node, owner, authz.NewGenericAuthorization(msgURL), nil))
	_, err = authzKeeper.DispatchActions(ctx, node, []sdk.Msg{submit})
	require.NoError(t, err)

	// 3. The vote is attributed to the OWNER, not the node.
	_, err = k.Votes.Get(ctx, collections.Join(epoch, owner.String()))
	require.NoError(t, err, "vote should be recorded under the owner")
	hasNodeVote, err := k.Votes.Has(ctx, collections.Join(epoch, node.String()))
	require.NoError(t, err)
	require.False(t, hasNodeVote, "no vote should be recorded under the node key")

	// 4. Revoking the grant blocks further voting (the epoch is still open).
	require.NoError(t, authzKeeper.DeleteGrant(ctx, node, owner, msgURL))
	_, err = authzKeeper.DispatchActions(ctx, node, []sdk.Msg{submit})
	require.Error(t, err)

	// 5. The tally counts the OWNER's license + stake weight: the epoch reaches
	//    consensus and enters PENDING with the submitted root.
	require.NoError(t, k.EpochHooks().AfterEpochEnd(ctx, params.EpochIdentifier, epoch))
	ed, err := k.EpochDistributions.Get(ctx, epoch)
	require.NoError(t, err)
	require.Equal(t, types.DISTRIBUTION_STATUS_PENDING, ed.Status)
	require.Equal(t, root, ed.MerkleRoot)
}
