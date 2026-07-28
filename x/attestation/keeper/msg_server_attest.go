package keeper

import (
	"context"
	"fmt"

	errorsmod "cosmossdk.io/errors"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
	networktypes "github.com/webstack-sdk/webstack/x/network/types"
)

// AttestRwa records RWA contract-supply attestations. RWA attestation is
// trust-only: nano nodes are rejected. The daily quota is consumed in the
// ante handler; the handler re-reads the counter and never increments.
func (ms msgServer) AttestRwa(ctx context.Context, msg *types.MsgAttestRwa) (*types.MsgAttestRwaResponse, error) {
	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	node, err := ms.checkNode(ctx, msg.NodeAddress, ms.k.RwaCounters, params.RwaDailyLimit)
	if err != nil {
		return nil, err
	}
	// The only type-conditional rule in the flow: RWA attestations must come
	// from trust nodes.
	if node.Type != types.NodeTypeTrust {
		return nil, errorsmod.Wrapf(types.ErrRwaTrustOnly, "node %s has type %q", msg.NodeAddress, node.Type)
	}

	if err := ms.recordAttestations(ctx, node, msg.Attestations, types.EventTypeAttestRwa); err != nil {
		return nil, err
	}

	return &types.MsgAttestRwaResponse{}, nil
}

// AttestRwu records RWU contract-supply attestations, accepted from any
// active node type.
func (ms msgServer) AttestRwu(ctx context.Context, msg *types.MsgAttestRwu) (*types.MsgAttestRwuResponse, error) {
	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	node, err := ms.checkNode(ctx, msg.NodeAddress, ms.k.RwuCounters, params.RwuDailyLimit)
	if err != nil {
		return nil, err
	}

	if err := ms.recordAttestations(ctx, node, msg.Attestations, types.EventTypeAttestRwu); err != nil {
		return nil, err
	}

	return &types.MsgAttestRwuResponse{}, nil
}

// checkNode verifies the signing node exists, is active, is backed by a
// license of the type it declares, and that its daily counter has not already
// gone beyond the limit. The counter is re-read only — the ante admission is
// what increments it.
func (ms msgServer) checkNode(ctx context.Context, nodeAddr string, counters collections.Map[string, types.ActivityCounter], limit uint64) (networktypes.Node, error) {
	node, active, err := ms.k.networkKeeper.IsActiveNode(ctx, nodeAddr)
	if err != nil {
		return networktypes.Node{}, err
	}
	if node.Address == "" {
		return networktypes.Node{}, errorsmod.Wrapf(types.ErrNodeNotFound, "node %s", nodeAddr)
	}
	if !active {
		return networktypes.Node{}, errorsmod.Wrapf(types.ErrNodeNotActive, "node %s", nodeAddr)
	}

	if err := ms.k.ensureNodeTypeLicensed(ctx, node); err != nil {
		return networktypes.Node{}, err
	}

	counter, err := counters.Get(ctx, nodeAddr)
	if err == nil && types.SameDay(counter.LatestTime, sdk.UnwrapSDKContext(ctx).BlockTime()) && counter.DailyCount > limit {
		return networktypes.Node{}, errorsmod.Wrapf(types.ErrQuotaExceeded, "node %s exceeded %d attestations today", nodeAddr, limit)
	}

	return node, nil
}

// recordAttestations touches the node's recent activity and emits one event
// per attestation. Payloads are not written to state.
func (ms msgServer) recordAttestations(ctx context.Context, node networktypes.Node, attestations []types.ContractAttestation, eventType string) error {
	if err := ms.k.networkKeeper.TouchNodeActivity(ctx, node.Address); err != nil {
		return err
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	for _, a := range attestations {
		sdkCtx.EventManager().EmitEvent(sdk.NewEvent(
			eventType,
			sdk.NewAttribute(types.AttributeKeyNodeAddress, node.Address),
			sdk.NewAttribute(types.AttributeKeyOperator, node.Operator),
			sdk.NewAttribute(types.AttributeKeyContractAddress, a.ContractAddress),
			sdk.NewAttribute(types.AttributeKeyCurrentSupply, a.CurrentSupply.String()),
			sdk.NewAttribute(types.AttributeKeyBlockHeight, fmt.Sprintf("%d", a.BlockHeight)),
		))
	}
	return nil
}
