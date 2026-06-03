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

// EpochDistribution returns the canonical distribution for an epoch.
func (k Querier) EpochDistribution(goCtx context.Context, req *types.QueryEpochDistributionRequest) (*types.QueryEpochDistributionResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	ed, err := k.Keeper.EpochDistributions.Get(ctx, req.Epoch)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "no distribution for epoch %d", req.Epoch)
	}
	return &types.QueryEpochDistributionResponse{EpochDistribution: &ed}, nil
}

// DistributionVotes returns all submitted votes for an epoch.
func (k Querier) DistributionVotes(goCtx context.Context, req *types.QueryDistributionVotesRequest) (*types.QueryDistributionVotesResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	var votes []types.DistributionVote
	rng := collections.NewPrefixedPairRange[int64, string](req.Epoch)
	if err := k.Keeper.Votes.Walk(ctx, rng, func(_ collections.Pair[int64, string], v types.DistributionVote) (bool, error) {
		votes = append(votes, v)
		return false, nil
	}); err != nil {
		return nil, err
	}
	return &types.QueryDistributionVotesResponse{Votes: votes}, nil
}

// Claimed reports whether a (epoch, nonce) reward has been claimed.
func (k Querier) Claimed(goCtx context.Context, req *types.QueryClaimedRequest) (*types.QueryClaimedResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	ctx := sdk.UnwrapSDKContext(goCtx)

	claimed, err := k.Keeper.Claimed.Has(ctx, collections.Join(req.Epoch, req.Nonce))
	if err != nil {
		return nil, err
	}
	return &types.QueryClaimedResponse{Claimed: claimed}, nil
}
