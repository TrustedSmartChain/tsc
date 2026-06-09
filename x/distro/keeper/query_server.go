package keeper

import (
	"context"
	"errors"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/query"
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

// ClaimsByDate lists the reward nonces claimed for a day (YYYY-MM-DD), in
// ascending order.
func (k Querier) ClaimsByDate(goCtx context.Context, req *types.QueryClaimsByDateRequest) (*types.QueryClaimsByDateResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if req.Date == "" {
		return nil, status.Error(codes.InvalidArgument, "date is required")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	var nonces []uint64
	rng := collections.NewPrefixedPairRange[string, uint64](req.Date)
	if err := k.Keeper.Claimed.Walk(ctx, rng, func(key collections.Pair[string, uint64]) (bool, error) {
		nonces = append(nonces, key.K2())
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryClaimsByDateResponse{Nonces: nonces}, nil
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

// Audit runs the module's invariants against current state and reports the
// result, so operators can verify module health on a live node.
func (k Querier) Audit(goCtx context.Context, req *types.QueryAuditRequest) (*types.QueryAuditResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	results := k.Keeper.CheckInvariants(ctx)
	resp := &types.QueryAuditResponse{Results: make([]types.AuditResult, 0, len(results))}
	for _, r := range results {
		if r.Broken {
			resp.Broken = true
		}
		resp.Results = append(resp.Results, types.AuditResult{
			Name:    r.Name,
			Broken:  r.Broken,
			Message: r.Message,
		})
	}
	return resp, nil
}

// Distributions lists all distributions, date-ordered and paginated.
func (k Querier) Distributions(goCtx context.Context, req *types.QueryDistributionsRequest) (*types.QueryDistributionsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	distributions, pageResp, err := query.CollectionPaginate(
		ctx, k.Keeper.Distributions, req.Pagination,
		func(_ string, d types.Distribution) (types.Distribution, error) {
			return d, nil
		},
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryDistributionsResponse{Distributions: distributions, Pagination: pageResp}, nil
}

// ActiveDistributions lists the non-terminal (VOTING/PENDING/UNDER_REVIEW)
// distributions by resolving the active index. The set is bounded by the open
// voting/review windows, so it is returned unpaginated.
func (k Querier) ActiveDistributions(goCtx context.Context, req *types.QueryActiveDistributionsRequest) (*types.QueryActiveDistributionsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	var distributions []types.Distribution
	if err := k.Keeper.ActiveDistributions.Walk(ctx, nil, func(date string) (bool, error) {
		d, err := k.Keeper.Distributions.Get(ctx, date)
		if err != nil {
			if errors.Is(err, collections.ErrNotFound) {
				// Stale index entry (no backing distribution); skip it.
				return false, nil
			}
			return true, err
		}
		distributions = append(distributions, d)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryActiveDistributionsResponse{Distributions: distributions}, nil
}
