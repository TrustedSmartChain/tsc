package keeper_test

import (
	"testing"
	"time"

	"github.com/cosmos/cosmos-sdk/types/query"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	networktypes "github.com/nodelabs-sdk/nodelabs/x/network/types"

	"github.com/TrustedSmartChain/tsc/v4/x/attestation/types"
)

func requireTime(t *testing.T, want time.Time, got *time.Time) {
	t.Helper()
	require.NotNil(t, got)
	require.True(t, want.Equal(*got), "want %s, got %s", want, *got)
}

// The last-attested times are set by the handlers, not the ante admission, so
// a msg counts whether or not it went through the gasless quota. The handler
// is called directly here with no admission run, which is exactly the
// fee-paying path: the daily counter stays untouched while the time is set.
func TestNodeStatsSetRegardlessOfGasPath(t *testing.T) {
	f := SetupTest(t)
	node := f.addNode(nodeTypeTrust, networktypes.NodeActive)

	_, err := f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{NodeAddress: node.Address, Attestations: attestations(1)})
	require.NoError(t, err)

	_, err = f.k.RwaCounters.Get(f.ctx, node.Address)
	require.Error(t, err, "the paid path must not consume the gasless daily quota")

	stats, err := f.k.GetNodeStats(f.ctx, node.Address)
	require.NoError(t, err)
	requireTime(t, fixtureBlockTime, stats.LastRwaTime)
	require.Nil(t, stats.LastRwuTime, "RWU is tracked separately and untouched")

	// Gasless path: admission consumes quota, then the handler runs and moves
	// the time forward.
	ctx := f.WithBlockTime(fixtureBlockTime.Add(time.Minute))
	msg := &types.MsgAttestRwa{NodeAddress: node.Address, Attestations: attestations(1)}
	require.NoError(t, f.k.CheckAndConsumeGaslessQuota(ctx, msg))
	_, err = f.msgServer.AttestRwa(ctx, msg)
	require.NoError(t, err)

	stats, err = f.k.GetNodeStats(ctx, node.Address)
	require.NoError(t, err)
	requireTime(t, ctx.BlockTime(), stats.LastRwaTime)
	require.Nil(t, stats.LastRwuTime)
}

func TestNodeStatsKindsAreSeparate(t *testing.T) {
	f := SetupTest(t)
	node := f.addNode(nodeTypeTrust, networktypes.NodeActive)

	_, err := f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{NodeAddress: node.Address, Attestations: attestations(1)})
	require.NoError(t, err)

	later := f.WithBlockTime(fixtureBlockTime.Add(time.Hour))
	_, err = f.msgServer.AttestRwu(later, &types.MsgAttestRwu{NodeAddress: node.Address, Attestations: attestations(1)})
	require.NoError(t, err)

	stats, err := f.k.GetNodeStats(later, node.Address)
	require.NoError(t, err)
	requireTime(t, fixtureBlockTime, stats.LastRwaTime)
	requireTime(t, later.BlockTime(), stats.LastRwuTime)
}

// Only msgs that execute count: a rejected attestation leaves no entry.
func TestNodeStatsIgnoreRejectedMsgs(t *testing.T) {
	f := SetupTest(t)
	nano := f.addNode(nodeTypeNano, networktypes.NodeActive)
	deactivated := f.addNode(nodeTypeTrust, networktypes.NodeDeactivated)

	_, err := f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{NodeAddress: nano.Address, Attestations: attestations(1)})
	require.ErrorIs(t, err, types.ErrNodeTypeNotAllowed)
	_, err = f.msgServer.AttestRwu(f.ctx, &types.MsgAttestRwu{NodeAddress: deactivated.Address, Attestations: attestations(1)})
	require.ErrorIs(t, err, types.ErrNodeNotActive)

	for _, addr := range []string{nano.Address, deactivated.Address} {
		has, err := f.k.NodeStats.Has(f.ctx, addr)
		require.NoError(t, err)
		require.False(t, has)
	}
}

func TestQueryNodeStats(t *testing.T) {
	f := SetupTest(t)
	node := f.addNode(nodeTypeTrust, networktypes.NodeActive)
	idle := f.addNode(nodeTypeTrust, networktypes.NodeActive)

	_, err := f.msgServer.AttestRwu(f.ctx, &types.MsgAttestRwu{NodeAddress: node.Address, Attestations: attestations(1)})
	require.NoError(t, err)

	res, err := f.queryServer.NodeStats(f.ctx, &types.QueryNodeStatsRequest{NodeAddress: node.Address})
	require.NoError(t, err)
	require.Equal(t, node.Address, res.Stats.NodeAddress)
	requireTime(t, fixtureBlockTime, res.Stats.LastRwuTime)
	require.Nil(t, res.Stats.LastRwaTime)

	// A node that never attested reads as unset times, not an error.
	res, err = f.queryServer.NodeStats(f.ctx, &types.QueryNodeStatsRequest{NodeAddress: idle.Address})
	require.NoError(t, err)
	require.Equal(t, types.AttestationStats{NodeAddress: idle.Address}, res.Stats)

	_, err = f.queryServer.NodeStats(f.ctx, &types.QueryNodeStatsRequest{NodeAddress: "not-bech32"})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
	_, err = f.queryServer.NodeStats(f.ctx, nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestQueryAllNodeStats(t *testing.T) {
	f := SetupTest(t)

	want := map[string]time.Time{}
	for i := range 3 {
		ctx := f.WithBlockTime(fixtureBlockTime.Add(time.Duration(i) * time.Minute))
		node := f.addNode(nodeTypeTrust, networktypes.NodeActive)
		_, err := f.msgServer.AttestRwa(ctx, &types.MsgAttestRwa{NodeAddress: node.Address, Attestations: attestations(1)})
		require.NoError(t, err)
		want[node.Address] = ctx.BlockTime()
	}
	// An active node that never attested is not listed.
	f.addNode(nodeTypeTrust, networktypes.NodeActive)

	// Pagination is optional: no PageRequest returns everything that fits
	// the default page.
	res, err := f.queryServer.AllNodeStats(f.ctx, &types.QueryAllNodeStatsRequest{})
	require.NoError(t, err)
	require.Len(t, res.Stats, 3)
	for _, s := range res.Stats {
		requireTime(t, want[s.NodeAddress], s.LastRwaTime)
		require.Nil(t, s.LastRwuTime)
	}

	// Paging walks every node exactly once.
	seen := map[string]bool{}
	var next []byte
	for {
		page, err := f.queryServer.AllNodeStats(f.ctx, &types.QueryAllNodeStatsRequest{
			Pagination: &query.PageRequest{Key: next, Limit: 2},
		})
		require.NoError(t, err)
		require.LessOrEqual(t, len(page.Stats), 2)
		for _, s := range page.Stats {
			require.False(t, seen[s.NodeAddress])
			seen[s.NodeAddress] = true
		}
		next = page.Pagination.NextKey
		if next == nil {
			break
		}
	}
	require.Len(t, seen, 3)

	// Offset paging with a total count works too.
	page, err := f.queryServer.AllNodeStats(f.ctx, &types.QueryAllNodeStatsRequest{
		Pagination: &query.PageRequest{Offset: 2, Limit: 2, CountTotal: true},
	})
	require.NoError(t, err)
	require.Len(t, page.Stats, 1)
	require.Equal(t, uint64(3), page.Pagination.Total)

	_, err = f.queryServer.AllNodeStats(f.ctx, nil)
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}
