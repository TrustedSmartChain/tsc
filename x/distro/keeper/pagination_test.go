package keeper_test

import (
	"testing"

	"cosmossdk.io/collections"
	"github.com/stretchr/testify/require"

	simtestutil "github.com/cosmos/cosmos-sdk/testutil/sims"
	"github.com/cosmos/cosmos-sdk/types/query"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/keeper"
	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

func TestClaimsByDatePagination(t *testing.T) {
	f, _ := fourVoterConsensus(t)
	q := keeper.NewQuerier(f.k)

	// Seed five claimed nonces for one day.
	for nonce := uint64(0); nonce < 5; nonce++ {
		require.NoError(t, f.k.Claimed.Set(f.ctx, collections.Join3(dateOf(1), defaultType, nonce)))
	}

	// Unpaginated returns all five, ascending.
	all, err := q.ClaimsByDate(f.ctx, &types.QueryClaimsByDateRequest{Date: dateOf(1), DistroType: defaultType})
	require.NoError(t, err)
	require.Equal(t, []uint64{0, 1, 2, 3, 4}, all.Nonces)

	// A limit of 2 returns the first page plus a next key.
	page1, err := q.ClaimsByDate(f.ctx, &types.QueryClaimsByDateRequest{Date: dateOf(1), DistroType: defaultType, Pagination: &query.PageRequest{Limit: 2}})
	require.NoError(t, err)
	require.Equal(t, []uint64{0, 1}, page1.Nonces)
	require.NotEmpty(t, page1.Pagination.NextKey)

	// Following the next key returns the subsequent page.
	page2, err := q.ClaimsByDate(f.ctx, &types.QueryClaimsByDateRequest{Date: dateOf(1), DistroType: defaultType, Pagination: &query.PageRequest{Key: page1.Pagination.NextKey, Limit: 2}})
	require.NoError(t, err)
	require.Equal(t, []uint64{2, 3}, page2.Nonces)
}

func TestDistributionVotesPagination(t *testing.T) {
	f, _ := fourVoterConsensus(t)
	q := keeper.NewQuerier(f.k)
	signers := simtestutil.CreateIncrementalAccounts(3)

	for _, s := range signers {
		require.NoError(t, f.k.Votes.Set(f.ctx, collections.Join3(dateOf(1), defaultType, s.String()), types.DistributionVote{
			Date: dateOf(1), DistroType: defaultType, Signer: s.String(), MerkleRoot: []byte("root"),
		}))
	}

	all, err := q.DistributionVotes(f.ctx, &types.QueryDistributionVotesRequest{Date: dateOf(1), DistroType: defaultType})
	require.NoError(t, err)
	require.Len(t, all.Votes, 3)

	page, err := q.DistributionVotes(f.ctx, &types.QueryDistributionVotesRequest{Date: dateOf(1), DistroType: defaultType, Pagination: &query.PageRequest{Limit: 2}})
	require.NoError(t, err)
	require.Len(t, page.Votes, 2)
	require.NotEmpty(t, page.Pagination.NextKey)
}

func TestActiveDistributionsPagination(t *testing.T) {
	f, _ := fourVoterConsensus(t)
	q := keeper.NewQuerier(f.k)

	for i := int64(1); i <= 3; i++ {
		date := dateOf(i)
		require.NoError(t, f.k.Distributions.Set(f.ctx, collections.Join(date, defaultType), types.Distribution{Date: date, DistroType: defaultType, Status: types.DISTRIBUTION_STATUS_VOTING}))
		require.NoError(t, f.k.ActiveDistributions.Set(f.ctx, collections.Join(date, defaultType)))
	}

	page, err := q.ActiveDistributions(f.ctx, &types.QueryActiveDistributionsRequest{Pagination: &query.PageRequest{Limit: 2}})
	require.NoError(t, err)
	require.Len(t, page.Distributions, 2)
	require.NotEmpty(t, page.Pagination.NextKey)
}
