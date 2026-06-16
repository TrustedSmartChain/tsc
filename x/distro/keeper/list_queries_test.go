package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"

	"github.com/stretchr/testify/require"

	"github.com/cosmos/cosmos-sdk/types/query"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/keeper"
	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

func TestDistributionsQuery(t *testing.T) {
	f, _ := fourVoterConsensus(t)
	q := keeper.NewQuerier(f.k)

	// Seed several distributions with assorted statuses.
	seed := []types.DistributionStatus{
		types.DISTRIBUTION_STATUS_VOTING,
		types.DISTRIBUTION_STATUS_PENDING,
		types.DISTRIBUTION_STATUS_LIVE,
		types.DISTRIBUTION_STATUS_EXPIRED,
	}
	for i, s := range seed {
		date := dateOf(int64(i + 1))
		require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(date, defaultType), types.Distribution{Date: date, DistroType: defaultType, Status: s, MerkleRoot: []byte("r")}))
	}

	// Unpaginated: returns all, in ascending date order.
	resp, err := q.Distributions(f.ctx, &types.QueryDistributionsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Distributions, 4)
	require.Equal(t, dateOf(1), resp.Distributions[0].Date)
	require.Equal(t, dateOf(4), resp.Distributions[3].Date)

	// Paginated: a limit of 2 returns a page plus a next key.
	page, err := q.Distributions(f.ctx, &types.QueryDistributionsRequest{Pagination: &query.PageRequest{Limit: 2}})
	require.NoError(t, err)
	require.Len(t, page.Distributions, 2)
	require.NotNil(t, page.Pagination)
	require.NotEmpty(t, page.Pagination.NextKey)

	// Nil request is rejected.
	_, err = q.Distributions(f.ctx, nil)
	require.Error(t, err)
}

func TestActiveDistributionsQuery(t *testing.T) {
	f, _ := fourVoterConsensus(t)
	q := keeper.NewQuerier(f.k)

	// Two non-terminal days (indexed) and two terminal days (not indexed).
	active := map[string]types.DistributionStatus{
		dateOf(1): types.DISTRIBUTION_STATUS_VOTING,
		dateOf(2): types.DISTRIBUTION_STATUS_UNDER_REVIEW,
	}
	terminal := map[string]types.DistributionStatus{
		dateOf(3): types.DISTRIBUTION_STATUS_LIVE,
		dateOf(4): types.DISTRIBUTION_STATUS_EXPIRED,
	}
	for date, s := range active {
		require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(date, defaultType), types.Distribution{Date: date, DistroType: defaultType, Status: s, MerkleRoot: []byte("r")}))
		require.NoError(t, f.k.ActiveDistributions.Set(f.ctx, collections.Join(date, defaultType)))
	}
	for date, s := range terminal {
		require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(date, defaultType), types.Distribution{Date: date, DistroType: defaultType, Status: s, MerkleRoot: []byte("r")}))
	}

	resp, err := q.ActiveDistributions(f.ctx, &types.QueryActiveDistributionsRequest{})
	require.NoError(t, err)
	require.Len(t, resp.Distributions, 2)

	got := map[string]types.DistributionStatus{}
	for _, d := range resp.Distributions {
		got[d.Date] = d.Status
	}
	require.Equal(t, active, got, "only non-terminal days are returned")

	// Nil request is rejected.
	_, err = q.ActiveDistributions(f.ctx, nil)
	require.Error(t, err)
}
