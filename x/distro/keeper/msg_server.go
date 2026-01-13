package keeper

import (
	"context"

	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	"cosmossdk.io/errors"
	"github.com/TrustedSmartChain/tsc/x/distro/types"
)

type msgServer struct {
	k Keeper
}

var _ types.MsgServer = msgServer{}

// NewMsgServerImpl returns an implementation of the module MsgServer interface.
func NewMsgServerImpl(keeper Keeper) types.MsgServer {
	return &msgServer{k: keeper}
}

func (ms msgServer) UpdateParams(ctx context.Context, msg *types.MsgUpdateParams) (*types.MsgUpdateParamsResponse, error) {
	if ms.k.authority != msg.Authority {
		return nil, errors.Wrapf(govtypes.ErrInvalidSigner, "invalid authority; expected %s, got %s", ms.k.authority, msg.Authority)
	}

	err := msg.Params.Validate()
	if err != nil {
		return &types.MsgUpdateParamsResponse{}, err
	}

	err = ms.k.Params.Set(ctx, msg.Params)
	if err != nil {
		return &types.MsgUpdateParamsResponse{}, err
	}

	return &types.MsgUpdateParamsResponse{}, nil
}
