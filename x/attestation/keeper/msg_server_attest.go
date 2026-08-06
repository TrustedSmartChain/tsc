package keeper

import (
	"context"
	"fmt"
	"slices"

	errorsmod "cosmossdk.io/errors"

	"cosmossdk.io/collections"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
	networktypes "github.com/webstack-sdk/webstack/x/network/types"
)

// AttestRwa records RWA contract-supply attestations. Which node types may do
// so is declared on the message itself, so adding a tier is a binary change
// rather than chain configuration. The daily quota is consumed in the ante
// handler; the handler re-reads the counter and never increments.
//
// There is no type-conditional logic here: the node types allowed to send this
// message are declared on the message itself, and checkNode enforces that the
// same way it does for every other attestation.
func (ms msgServer) AttestRwa(ctx context.Context, msg *types.MsgAttestRwa) (*types.MsgAttestRwaResponse, error) {
	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	node, err := ms.checkNode(ctx, msg, ms.k.RwaCounters, params.RwaDailyLimit)
	if err != nil {
		return nil, err
	}

	if err := ms.recordAttestations(ctx, node, msg.Attestations, types.EventTypeAttestRwa); err != nil {
		return nil, err
	}

	return &types.MsgAttestRwaResponse{}, nil
}

// AttestRwu records RWU contract-supply attestations. Like RWA it admits only
// the node types the message names, so a tier not on that list reports nothing
// even though it is active and licensed.
func (ms msgServer) AttestRwu(ctx context.Context, msg *types.MsgAttestRwu) (*types.MsgAttestRwuResponse, error) {
	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	node, err := ms.checkNode(ctx, msg, ms.k.RwuCounters, params.RwuDailyLimit)
	if err != nil {
		return nil, err
	}

	if err := ms.recordAttestations(ctx, node, msg.Attestations, types.EventTypeAttestRwu); err != nil {
		return nil, err
	}

	return &types.MsgAttestRwuResponse{}, nil
}

// checkNode verifies the signing node exists, is active, is backed by a
// license of the type it declares, that its type is one the msg admits, and
// that its daily counter has not already gone beyond the limit. The counter is
// re-read only — the ante admission is what increments it.
//
// Taking the message rather than an address is what keeps the node type rule in
// one place: every attestation names its own allowed tiers, so adding a third
// kind needs no change here and cannot accidentally skip the check.
func (ms msgServer) checkNode(ctx context.Context, msg types.AttestationMsg, counters collections.Map[string, types.ActivityCounter], limit uint64) (networktypes.Node, error) {
	nodeAddr := msg.GetNodeAddress()
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

	// Re-checked per attestation rather than trusted from activation, so a
	// revoked operator's nodes stop attesting immediately. x/network resolves
	// the node type's bound license type itself; this module no longer keeps a
	// mapping of its own.
	if err := ms.k.networkKeeper.EnsureOperatorLicensedForNodeType(ctx, node.Operator, node.Type); err != nil {
		return networktypes.Node{}, err
	}

	// Being licensed for a node type says the operator paid for the tier; it
	// does not say the tier may make this particular claim. The allowed tiers
	// are compiled in, so a node type absent from the list is denied even when
	// x/network has it registered and licensed.
	allowed := msg.AllowedNodeTypes()
	if !slices.Contains(allowed, node.Type) {
		return networktypes.Node{}, errorsmod.Wrapf(types.ErrNodeTypeNotAllowed,
			"node %s has type %q; %v may send this attestation", nodeAddr, node.Type, allowed)
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
