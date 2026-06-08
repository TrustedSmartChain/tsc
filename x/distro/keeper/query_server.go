package keeper

import (
	"context"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

var _ types.QueryServer = Querier{}

type Querier struct {
	Keeper
}

func NewQuerier(keeper Keeper) Querier {
	return Querier{Keeper: keeper}
}

func (k Querier) Params(c context.Context, req *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	ctx := sdk.UnwrapSDKContext(c)

	p, err := k.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	return &types.QueryParamsResponse{Params: &p}, nil
}

// Distribution returns the canonical distribution for a day (YYYY-MM-DD).
func (k Querier) Distribution(goCtx context.Context, req *types.QueryDistributionRequest) (*types.QueryDistributionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	d, err := k.Keeper.Distributions.Get(ctx, req.Date)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "no distribution for date %q", req.Date)
	}
	return &types.QueryDistributionResponse{Distribution: &d}, nil
}

// DistributionVotes returns all submitted votes for a day (YYYY-MM-DD).
func (k Querier) DistributionVotes(goCtx context.Context, req *types.QueryDistributionVotesRequest) (*types.QueryDistributionVotesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	var votes []types.DistributionVote
	rng := collections.NewPrefixedPairRange[string, string](req.Date)
	if err := k.Keeper.Votes.Walk(ctx, rng, func(_ collections.Pair[string, string], v types.DistributionVote) (bool, error) {
		votes = append(votes, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryDistributionVotesResponse{Votes: votes}, nil
}

// Claimed reports whether a (date, nonce) reward has been claimed.
func (k Querier) Claimed(goCtx context.Context, req *types.QueryClaimedRequest) (*types.QueryClaimedResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	claimed, err := k.Keeper.Claimed.Has(ctx, collections.Join(req.Date, req.Nonce))
	if err != nil {
		return nil, err
	}
	return &types.QueryClaimedResponse{Claimed: claimed}, nil
}

// ClaimTotalByCategory returns the cumulative claimed amount per category for a
// day's distribution (YYYY-MM-DD).
func (k Querier) ClaimTotalByCategory(goCtx context.Context, req *types.QueryClaimTotalByCategoryRequest) (*types.QueryClaimTotalByCategoryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if req.Date == "" {
		return nil, status.Error(codes.InvalidArgument, "date is required")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	totals := map[string]string{}
	rng := collections.NewPrefixedPairRange[string, string](req.Date)
	if err := k.Keeper.ClaimTotals.Walk(ctx, rng, func(_ collections.Pair[string, string], ct types.CategoryClaimTotal) (bool, error) {
		totals[ct.Category] = ct.Total
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryClaimTotalByCategoryResponse{Date: req.Date, Totals: totals}, nil
}
