package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/keeper"
	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// Queries over a day with no state return empty results (not errors), except
// Distribution which reports NotFound for an unknown day.
func TestQueryEmptyAndNotFound(t *testing.T) {
	f, _ := fourVoterConsensus(t)
	q := keeper.NewQuerier(f.k)

	_, err := q.Distribution(f.ctx, &types.QueryDistributionRequest{Date: dateOf(1), DistroType: defaultType})
	require.Equal(t, codes.NotFound, status.Code(err))

	votes, err := q.DistributionVotes(f.ctx, &types.QueryDistributionVotesRequest{Date: dateOf(1), DistroType: defaultType})
	require.NoError(t, err)
	require.Empty(t, votes.Votes)

	claimed, err := q.Claimed(f.ctx, &types.QueryClaimedRequest{Date: dateOf(1), DistroType: defaultType, Nonce: 0})
	require.NoError(t, err)
	require.False(t, claimed.Claimed)

	claims, err := q.ClaimsByDate(f.ctx, &types.QueryClaimsByDateRequest{Date: dateOf(1), DistroType: defaultType})
	require.NoError(t, err)
	require.Empty(t, claims.Nonces)

	totals, err := q.ClaimTotalByCategory(f.ctx, &types.QueryClaimTotalByCategoryRequest{Date: dateOf(1), DistroType: defaultType})
	require.NoError(t, err)
	require.Empty(t, totals.Totals)
}

func TestQueryRejectsEmptyDate(t *testing.T) {
	f, _ := fourVoterConsensus(t)
	q := keeper.NewQuerier(f.k)

	_, err := q.ClaimsByDate(f.ctx, &types.QueryClaimsByDateRequest{Date: ""})
	require.Equal(t, codes.InvalidArgument, status.Code(err))

	_, err = q.ClaimTotalByCategory(f.ctx, &types.QueryClaimTotalByCategoryRequest{Date: ""})
	require.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestQueryRejectsNilRequest(t *testing.T) {
	f, _ := fourVoterConsensus(t)
	q := keeper.NewQuerier(f.k)

	for name, call := range map[string]func() error{
		"Distribution":         func() error { _, err := q.Distribution(f.ctx, nil); return err },
		"DistributionVotes":    func() error { _, err := q.DistributionVotes(f.ctx, nil); return err },
		"Claimed":              func() error { _, err := q.Claimed(f.ctx, nil); return err },
		"ClaimsByDate":         func() error { _, err := q.ClaimsByDate(f.ctx, nil); return err },
		"ClaimTotalByCategory": func() error { _, err := q.ClaimTotalByCategory(f.ctx, nil); return err },
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, codes.InvalidArgument, status.Code(call()))
		})
	}
}
