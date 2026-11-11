package keeper

import (
	"context"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/cosmos/cosmos-sdk/types/query"

	"github.com/TrustedSmartChain/tsc/v4/x/attestation/types"
	networktypes "github.com/nodelabs-sdk/nodelabs/x/network/types"
)

var _ types.QueryServer = Querier{}

type Querier struct {
	Keeper
}

func NewQuerier(keeper Keeper) Querier {
	return Querier{Keeper: keeper}
}

func (q Querier) Params(ctx context.Context, _ *types.QueryParamsRequest) (*types.QueryParamsResponse, error) {
	params, err := q.Keeper.Params.Get(ctx)
	if err != nil {
		return nil, err
	}
	return &types.QueryParamsResponse{Params: params}, nil
}

// NodeStats returns when one node last attested each kind. A node that has
// never attested (or does not exist) returns unset times.
func (q Querier) NodeStats(ctx context.Context, req *types.QueryNodeStatsRequest) (*types.QueryNodeStatsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}
	if err := networktypes.ValidateCanonicalAddress("node", req.NodeAddress); err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}

	stats, err := q.Keeper.GetNodeStats(ctx, req.NodeAddress)
	if err != nil {
		return nil, status.Error(codes.Internal, err.Error())
	}
	return &types.QueryNodeStatsResponse{Stats: stats}, nil
}

// AllNodeStats returns when every node that has attested last attested each
// kind, ordered by node address. Pagination is optional: a nil PageRequest
// gets the default page size.
func (q Querier) AllNodeStats(ctx context.Context, req *types.QueryAllNodeStatsRequest) (*types.QueryAllNodeStatsResponse, error) {
	if req == nil {
		return nil, status.Error(codes.InvalidArgument, "empty request")
	}

	stats, pageRes, err := query.CollectionPaginate(ctx, q.Keeper.NodeStats, req.Pagination,
		func(_ string, s types.AttestationStats) (types.AttestationStats, error) {
			return s, nil
		})
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	return &types.QueryAllNodeStatsResponse{Stats: stats, Pagination: pageRes}, nil
}
