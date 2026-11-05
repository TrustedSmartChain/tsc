package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/collections"
	"cosmossdk.io/core/header"
	storetypes "cosmossdk.io/store/types"
	upgradetypes "cosmossdk.io/x/upgrade/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	licensetypes "github.com/nodelabs-sdk/nodelabs/x/license/types"
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
