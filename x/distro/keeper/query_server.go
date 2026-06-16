package keeper

import (
	"context"

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

// Distribution returns the canonical distribution for a (day, type).
func (k Querier) Distribution(goCtx context.Context, req *types.QueryDistributionRequest) (*types.QueryDistributionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	d, err := k.Keeper.Distributions.Get(ctx, collections.Join(req.Date, req.DistroType))
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "no distribution for date %q type %q", req.Date, req.DistroType)
	}
	return &types.QueryDistributionResponse{Distribution: &d}, nil
}

// DistributionVotes returns all submitted votes for a (day, type).
func (k Querier) DistributionVotes(goCtx context.Context, req *types.QueryDistributionVotesRequest) (*types.QueryDistributionVotesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	votes, pageResp, err := query.CollectionPaginate(
		ctx, k.Keeper.Votes, req.Pagination,
		func(_ collections.Triple[string, string, string], v types.DistributionVote) (types.DistributionVote, error) {
			return v, nil
		},
		func(o *query.CollectionsPaginateOptions[collections.Triple[string, string, string]]) {
			prefix := collections.TripleSuperPrefix[string, string, string](req.Date, req.DistroType)
			o.Prefix = &prefix
		},
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryDistributionVotesResponse{Votes: votes, Pagination: pageResp}, nil
}

// Claimed reports whether a (date, type, nonce) reward has been claimed.
func (k Querier) Claimed(goCtx context.Context, req *types.QueryClaimedRequest) (*types.QueryClaimedResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	claimed, err := k.Keeper.Claimed.Has(ctx, collections.Join3(req.Date, req.DistroType, req.Nonce))
	if err != nil {
		return nil, err
	}
	return &types.QueryClaimedResponse{Claimed: claimed}, nil
}

// ClaimsByDate lists the reward nonces claimed for a (day, type), in ascending
// order.
func (k Querier) ClaimsByDate(goCtx context.Context, req *types.QueryClaimsByDateRequest) (*types.QueryClaimsByDateResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if req.Date == "" {
		return nil, status.Error(codes.InvalidArgument, "date is required")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	nonces, pageResp, err := query.CollectionPaginate(
		ctx, k.Keeper.Claimed, req.Pagination,
		func(key collections.Triple[string, string, uint64], _ collections.NoValue) (uint64, error) {
			return key.K3(), nil
		},
		func(o *query.CollectionsPaginateOptions[collections.Triple[string, string, uint64]]) {
			prefix := collections.TripleSuperPrefix[string, string, uint64](req.Date, req.DistroType)
			o.Prefix = &prefix
		},
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryClaimsByDateResponse{Nonces: nonces, Pagination: pageResp}, nil
}

// ClaimTotalByCategory returns the cumulative claimed amount per category for a
// (day, type)'s distribution.
func (k Querier) ClaimTotalByCategory(goCtx context.Context, req *types.QueryClaimTotalByCategoryRequest) (*types.QueryClaimTotalByCategoryResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if req.Date == "" {
		return nil, status.Error(codes.InvalidArgument, "date is required")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	totals := map[string]string{}
	rng := collections.NewSuperPrefixedTripleRange[string, string, string](req.Date, req.DistroType)
	if err := k.Keeper.ClaimTotals.Walk(ctx, rng, func(_ collections.Triple[string, string, string], ct types.CategoryClaimTotal) (bool, error) {
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
		func(_ collections.Pair[string, string], d types.Distribution) (types.Distribution, error) {
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

	distributions, pageResp, err := query.CollectionFilteredPaginate(
		ctx, k.Keeper.ActiveDistributions, req.Pagination,
		// Filter out any stale index entry with no backing distribution.
		func(key collections.Pair[string, string], _ collections.NoValue) (bool, error) {
			has, err := k.Keeper.Distributions.Has(ctx, key)
			if err != nil {
				return false, err
			}
			return has, nil
		},
		func(key collections.Pair[string, string], _ collections.NoValue) (types.Distribution, error) {
			return k.Keeper.Distributions.Get(ctx, key)
		},
	)
	if err != nil {
		return nil, err
	}
	return &types.QueryActiveDistributionsResponse{Distributions: distributions, Pagination: pageResp}, nil
}
