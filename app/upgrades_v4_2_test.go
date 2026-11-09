package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protowire"

	dbm "github.com/cosmos/cosmos-db"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/header"
	"cosmossdk.io/log"
	"cosmossdk.io/store/metrics"
	"cosmossdk.io/store/rootmulti"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	slashingtypes "github.com/cosmos/cosmos-sdk/x/slashing/types"

	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"

	distrotypes "github.com/TrustedSmartChain/tsc/v4/x/distro/types"
	networktypes "github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// setupV4_2 returns the gasless env reshaped into the state v4.2 starts from,
// and a context carrying HeaderInfo (the upgrade keeper records the done
// height from it, as BaseApp sets it in PreBlocker).
//
// The env holds one trust license and two trust nodes active today. On top of
// that: a nano license, and node2 last active yesterday so both window slots
// are used. Then the state is put the way nodelabs v0.2 (license and network
// v1) left it: no denormalized license counts, no recent-activity windows,
// keys under the legacy prefix-13 index, and both modules at version 1.
func setupV4_2(t *testing.T) (*gaslessEnv, sdk.Context) {
	t.Helper()
	env := setupGaslessEnv(t)
	ctx := env.ctx.WithHeaderInfo(header.Info{Height: env.ctx.BlockHeight(), Time: env.ctx.BlockTime()})
	chainApp := env.app

	env.seedLicense(env.operator.String(), nanoLicenseType)
	node2, err := chainApp.NetworkKeeper.Nodes.Get(ctx, env.node2.String())
	require.NoError(t, err)
	node2.LastActiveTime = ctx.BlockTime().Add(-24 * time.Hour)
	require.NoError(t, chainApp.NetworkKeeper.Nodes.Set(ctx, env.node2.String(), node2))

	require.NoError(t, chainApp.LicenseKeeper.ActiveLicenseCounts.Clear(ctx, nil))
	require.NoError(t, chainApp.NetworkKeeper.RecentNodeCounts.Clear(ctx, nil))
	for _, suffix := range []string{"node-a", "node-b"} {
		legacyNetworkStore(ctx, chainApp).Set([]byte(suffix), []byte{})
	}

	vm, err := chainApp.UpgradeKeeper.GetModuleVersionMap(ctx)
	require.NoError(t, err)
	vm[licensetypes.ModuleName] = 1
	vm[networktypes.ModuleName] = 1
	require.NoError(t, chainApp.UpgradeKeeper.SetModuleVersionMap(ctx, vm))

	return env, ctx
}

// legacyNetworkStore is the network store under the v1 RecentNodeActivity
// prefix, which v4.2 must leave empty.
func legacyNetworkStore(ctx sdk.Context, chainApp *ChainApp) prefixStore {
	return prefixStore{ctx.KVStore(chainApp.GetKey(networktypes.StoreKey)), []byte(networktypes.LegacyRecentNodeActivityPrefix)}
}

type prefixStore struct {
	storetypes.KVStore
	prefix []byte
}

func (s prefixStore) Set(key, value []byte) {
	s.KVStore.Set(append(append([]byte(nil), s.prefix...), key...), value)
}

func (s prefixStore) empty() bool {
	iter := storetypes.KVStorePrefixIterator(s.KVStore, s.prefix)
	defer iter.Close()
	return !iter.Valid()
}

func applyV4_2(t *testing.T, chainApp *ChainApp, ctx sdk.Context) {
	t.Helper()
	require.NoError(t, chainApp.UpgradeKeeper.ApplyUpgrade(ctx, upgradetypes.Plan{
		Name:   UpgradeNameV4_2,
		Height: ctx.BlockHeight(),
	}))
}

func TestV4_2UpgradeMigratesNodelabsState(t *testing.T) {
	env, ctx := setupV4_2(t)
	chainApp, operator := env.app, env.operator.String()

	// Before the upgrade the v2 code reads v1 state as unlicensed, which is
	// why nodes must not run this binary before the fork height.
	count, err := chainApp.LicenseKeeper.CountActiveLicenses(ctx, operator, []string{trustLicenseType}, 0)
	require.NoError(t, err)
	require.Zero(t, count)

	applyV4_2(t, chainApp, ctx)

	vm, err := chainApp.UpgradeKeeper.GetModuleVersionMap(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(2), vm[licensetypes.ModuleName])
	require.Equal(t, uint64(2), vm[networktypes.ModuleName])

	// x/license: counts backfilled per license type.
	for _, typeID := range []string{trustLicenseType, nanoLicenseType} {
		count, err := chainApp.LicenseKeeper.CountActiveLicenses(ctx, operator, []string{typeID}, 0)
		require.NoError(t, err)
		require.Equal(t, uint64(1), count, typeID)
	}

	// x/network: window built from the node records, legacy index gone.
	window, err := chainApp.NetworkKeeper.RecentNodeCounts.Get(ctx, collections.Join(operator, trustNodeType))
	require.NoError(t, err)
	require.Equal(t, networktypes.RecentNodeWindow{
		Epoch: networktypes.DayEpoch(ctx.BlockTime()), Today: 1, Yesterday: 1,
	}, window)
	require.True(t, legacyNetworkStore(ctx, chainApp).empty(), "legacy RecentNodeActivity keys survived the upgrade")

	require.NoError(t, chainApp.LicenseKeeper.CheckInvariants(ctx))
	require.NoError(t, chainApp.NetworkKeeper.CheckInvariants(ctx))

	done, err := chainApp.UpgradeKeeper.GetDoneHeight(ctx, UpgradeNameV4_2)
	require.NoError(t, err)
	require.Equal(t, ctx.BlockHeight(), done)
}

func TestV4_2UpgradeRaisesMaxGaslessGas(t *testing.T) {
	env, ctx := setupV4_2(t)
	chainApp := env.app

	before, err := chainApp.NetworkKeeper.GetParams(ctx)
	require.NoError(t, err)
	require.Less(t, before.MaxGaslessGas, v4_2MaxGaslessGas)

	applyV4_2(t, chainApp, ctx)

	after, err := chainApp.NetworkKeeper.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, v4_2MaxGaslessGas, after.MaxGaslessGas)

	// Nothing else moves.
	after.MaxGaslessGas = before.MaxGaslessGas
	require.Equal(t, before, after)
}

func TestV4_2UpgradeKeepsHigherMaxGaslessGas(t *testing.T) {
	env, ctx := setupV4_2(t)
	chainApp := env.app

	params, err := chainApp.NetworkKeeper.GetParams(ctx)
	require.NoError(t, err)
	params.MaxGaslessGas = 900_000
	require.NoError(t, chainApp.NetworkKeeper.Params.Set(ctx, params))

	applyV4_2(t, chainApp, ctx)

	after, err := chainApp.NetworkKeeper.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(900_000), after.MaxGaslessGas)
}

// TestV4_2ForkUpgrade drives v4.2 through the fork path: no plan in state,
// applied by height from PreBlocker.
func TestV4_2ForkUpgrade(t *testing.T) {
	env, ctx := setupV4_2(t)
	chainApp := env.app

	var found bool
	for _, fu := range forkUpgrades() {
		if fu.Name == UpgradeNameV4_2 {
			found = true
			require.Equal(t, ForkHeightV4_2, fu.Height)
		}
	}
	require.True(t, found, "v4.2 missing from forkUpgrades()")

	fu := forkUpgrade{Name: UpgradeNameV4_2, Height: ctx.BlockHeight()}

	// Before the fork height: nothing happens.
	chainApp.applyForkUpgrade(ctx.WithBlockHeight(fu.Height-1), fu)
	done, err := chainApp.UpgradeKeeper.GetDoneHeight(ctx, UpgradeNameV4_2)
	require.NoError(t, err)
	require.Zero(t, done)

	// Fork height: migrations run and the plan is recorded as applied.
	chainApp.applyForkUpgrade(ctx, fu)
	count, err := chainApp.LicenseKeeper.CountActiveLicenses(ctx, env.operator.String(), []string{trustLicenseType}, 0)
	require.NoError(t, err)
	require.Equal(t, uint64(1), count)
	done, err = chainApp.UpgradeKeeper.GetDoneHeight(ctx, UpgradeNameV4_2)
	require.NoError(t, err)
	require.Equal(t, fu.Height, done)

	// Idempotent: a second pass at the same height is a no-op, not a panic.
	require.NotPanics(t, func() { chainApp.applyForkUpgrade(ctx, fu) })
}

// Counters of the retired gasless "authorize" kind are deleted; the live
// activate counters are not.
func TestV4_2UpgradeClearsLegacyAuthorizeCounters(t *testing.T) {
	env, ctx := setupV4_2(t)
	chainApp := env.app
	gc := chainApp.NetworkKeeper.GaslessCounters

	authorizeKey := collections.Join3(legacyGaslessKindAuthorize, env.operator.String(), networktypes.GaslessScopeNone)
	activateKey := collections.Join3(networktypes.GaslessKindActivate, env.key.String(), trustNodeType)
	require.NoError(t, gc.Set(ctx, authorizeKey, networktypes.ActivityCounter{DailyCount: 3}))
	require.NoError(t, gc.Set(ctx, activateKey, networktypes.ActivityCounter{DailyCount: 7}))

	applyV4_2(t, chainApp, ctx)

	has, err := gc.Has(ctx, authorizeKey)
	require.NoError(t, err)
	require.False(t, has, "legacy authorize counter survived")
	got, err := gc.Get(ctx, activateKey)
	require.NoError(t, err)
	require.Equal(t, uint64(7), got.DailyCount)
}

// signed_blocks_window goes 1,000 -> 10,000 with every other slashing param
// unchanged, and missed-block records kept under the old window are reset.
func TestV4_2UpgradeRaisesSignedBlocksWindow(t *testing.T) {
	env, ctx := setupV4_2(t)
	sk := env.app.SlashingKeeper

	params, err := sk.GetParams(ctx)
	require.NoError(t, err)
	params.SignedBlocksWindow = 1000 // mainnet's value
	require.NoError(t, sk.SetParams(ctx, params))

	cons := sdk.ConsAddress(make([]byte, 20))
	cons[19] = 7
	require.NoError(t, sk.SetValidatorSigningInfo(ctx, cons, slashingtypes.NewValidatorSigningInfo(cons, 5, 1234, time.Time{}, false, 2)))
	require.NoError(t, sk.SetMissedBlockBitmapValue(ctx, cons, 234, true))
	require.NoError(t, sk.SetMissedBlockBitmapValue(ctx, cons, 233, true))

	applyV4_2(t, env.app, ctx)

	after, err := sk.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, v4_2SignedBlocksWindow, after.SignedBlocksWindow)
	after.SignedBlocksWindow = params.SignedBlocksWindow
	require.Equal(t, params, after, "only signed_blocks_window may change")

	info, err := sk.GetValidatorSigningInfo(ctx, cons)
	require.NoError(t, err)
	require.Zero(t, info.MissedBlocksCounter)
	require.Zero(t, info.IndexOffset)
	require.Equal(t, int64(5), info.StartHeight, "start height is kept")
	for _, idx := range []int64{233, 234} {
		missed, err := sk.GetMissedBlockBitmapValue(ctx, cons, idx)
		require.NoError(t, err)
		require.False(t, missed)
	}
}

// A window already at or above the target is kept, and signing records are
// left alone.
func TestV4_2UpgradeKeepsLargerSignedBlocksWindow(t *testing.T) {
	env, ctx := setupV4_2(t)
	sk := env.app.SlashingKeeper

	params, err := sk.GetParams(ctx)
	require.NoError(t, err)
	params.SignedBlocksWindow = 20_000
	require.NoError(t, sk.SetParams(ctx, params))
	cons := sdk.ConsAddress(make([]byte, 20))
	cons[19] = 8
	require.NoError(t, sk.SetValidatorSigningInfo(ctx, cons, slashingtypes.NewValidatorSigningInfo(cons, 5, 1234, time.Time{}, false, 2)))

	applyV4_2(t, env.app, ctx)

	after, err := sk.GetParams(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(20_000), after.SignedBlocksWindow)
	info, err := sk.GetValidatorSigningInfo(ctx, cons)
	require.NoError(t, err)
	require.Equal(t, int64(2), info.MissedBlocksCounter)
	require.Equal(t, int64(1234), info.IndexOffset)
}

// v4.2 removes x/mint: no module, no store, no permissions, and its module
// account stays blocked.
func TestV4_2RemovesMint(t *testing.T) {
	env, _ := setupV4_2(t)
	chainApp := env.app

	_, wired := chainApp.ModuleManager.Modules[mintStoreKey]
	require.False(t, wired, "x/mint must not be in the module manager")
	require.Nil(t, chainApp.GetKey(mintStoreKey), "the mint store must not be mounted")
	_, inPerms := GetMaccPerms()[mintStoreKey]
	require.False(t, inPerms)
	require.True(t, BlockedAddresses()[authtypes.NewModuleAddress(mintStoreKey).String()])
}

func TestPendingStoreUpgrades(t *testing.T) {
	v42 := v4_2StoreUpgrades()
	v3 := v3StoreUpgrades()
	tests := []struct {
		name       string
		info       upgradetypes.Plan
		forkHeight int64
		wantHeight int64
		want       *storetypes.StoreUpgrades
	}{
		{name: "no upgrade info, no fork", info: upgradetypes.Plan{}},
		{name: "v4.2 through a plan", info: upgradetypes.Plan{Name: UpgradeNameV4_2, Height: 900}, wantHeight: 900, want: &v42},
		{name: "fork wins over mainnet's stale v3 plan file", info: upgradetypes.Plan{Name: UpgradeNameV3, Height: 680000}, forkHeight: 1000, wantHeight: 1000, want: &v42},
		{name: "fork with no plan file", info: upgradetypes.Plan{}, forkHeight: 1000, wantHeight: 1000, want: &v42},
		{name: "plan file naming v4.2 beats the fork height", info: upgradetypes.Plan{Name: UpgradeNameV4_2, Height: 900}, forkHeight: 1000, wantHeight: 900, want: &v42},
		{name: "v3 plan file without a fork", info: upgradetypes.Plan{Name: UpgradeNameV3, Height: 680000}, wantHeight: 680000, want: &v3},
		{name: "v4.1 had no store changes", info: upgradetypes.Plan{Name: UpgradeNameV4_1, Height: 817700}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			height, got := pendingStoreUpgrades(tc.info, tc.forkHeight)
			require.Equal(t, tc.want, got)
			require.Equal(t, tc.wantHeight, height)
		})
	}
}

// Replays the restart at the v4.2 height against a real multistore: the last
// commit holds a mint store and v4.2 no longer mounts one. Loading succeeds,
// other stores keep their data, and mint is gone from the commit. The app
// hash is the same with or without the loader: consensus never depends on it.
func TestV4_2StoreLoadDeletesMint(t *testing.T) {
	commitAfterUpgrade := func(t *testing.T, withLoader bool) []byte {
		t.Helper()
		db := dbm.NewMemDB()
		bankKey := storetypes.NewKVStoreKey("bank")
		mintKey := storetypes.NewKVStoreKey(mintStoreKey)

		pre := rootmulti.NewStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
		pre.MountStoreWithDB(bankKey, storetypes.StoreTypeIAVL, nil)
		pre.MountStoreWithDB(mintKey, storetypes.StoreTypeIAVL, nil)
		require.NoError(t, pre.LoadLatestVersion())
		pre.GetKVStore(mintKey).Set([]byte("minter"), []byte{1})
		pre.GetKVStore(bankKey).Set([]byte("balance"), []byte{2})
		last := pre.Commit()

		post := rootmulti.NewStore(db, log.NewNopLogger(), metrics.NewNoOpMetrics())
		post.MountStoreWithDB(bankKey, storetypes.StoreTypeIAVL, nil)
		if withLoader {
			height, upgrades := pendingStoreUpgrades(upgradetypes.Plan{}, last.Version+1)
			require.NoError(t, upgradetypes.UpgradeStoreLoader(height, upgrades)(post))
		} else {
			require.NoError(t, post.LoadLatestVersion())
		}
		require.Equal(t, []byte{2}, post.GetKVStore(bankKey).Get([]byte("balance")))
		next := post.Commit()
		info, err := post.GetCommitInfo(next.Version)
		require.NoError(t, err)
		for _, si := range info.StoreInfos {
			require.NotEqual(t, mintStoreKey, si.Name, "mint store is still in the commit")
		}
		return next.Hash
	}
	require.Equal(t, commitAfterUpgrade(t, true), commitAfterUpgrade(t, false))
}

// x/distro v1 params carried max_supply (field 4). The v4.2 upgrade re-writes
// them without it, keeping every other value, and moves x/distro to v2.
func TestV4_2MigratesDistroParams(t *testing.T) {
	env, ctx := setupV4_2(t)
	chainApp := env.app
	store := ctx.KVStore(chainApp.GetKey(distrotypes.StoreKey))
	paramsKey := []byte(distrotypes.ParamsKey)

	params, err := chainApp.DistroKeeper.Params.Get(ctx)
	require.NoError(t, err)
	bz, err := params.Marshal()
	require.NoError(t, err)
	v1 := protowire.AppendTag(append([]byte(nil), bz...), 4, protowire.BytesType)
	v1 = protowire.AppendString(v1, "21000000000000000000000000")
	store.Set(paramsKey, v1)
	require.True(t, hasProtoField(t, store.Get(paramsKey), 4))

	vm, err := chainApp.UpgradeKeeper.GetModuleVersionMap(ctx)
	require.NoError(t, err)
	vm[distrotypes.ModuleName] = 1
	require.NoError(t, chainApp.UpgradeKeeper.SetModuleVersionMap(ctx, vm))

	applyV4_2(t, chainApp, ctx)

	require.False(t, hasProtoField(t, store.Get(paramsKey), 4), "max_supply survived the upgrade")
	after, err := chainApp.DistroKeeper.Params.Get(ctx)
	require.NoError(t, err)
	require.Equal(t, params, after)
	vm, err = chainApp.UpgradeKeeper.GetModuleVersionMap(ctx)
	require.NoError(t, err)
	require.Equal(t, uint64(2), vm[distrotypes.ModuleName])
}

func hasProtoField(t *testing.T, bz []byte, field protowire.Number) bool {
	t.Helper()
	for len(bz) > 0 {
		num, typ, n := protowire.ConsumeTag(bz)
		require.GreaterOrEqual(t, n, 0)
		bz = bz[n:]
		if num == field {
			return true
		}
		m := protowire.ConsumeFieldValue(num, typ, bz)
		require.GreaterOrEqual(t, m, 0)
		bz = bz[m:]
	}
	return false
}
