package keeper

import (
	"context"

	errorsmod "cosmossdk.io/errors"

	networktypes "github.com/webstack-sdk/webstack/x/network/types"

	"github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

// ensureNodeTypeLicensed rejects a node whose operator does not hold at least
// one active license of the type the node declares.
//
// This is the check that makes node type mean something. x/network is
// deliberately type-agnostic: MsgActivateNode's node_type is an opaque string
// validated only against the allowed_node_types param, and the activation
// limit counts licenses in aggregate across every counted type, so activation
// alone never establishes that the operator holds a license of the declared
// type. Without this check, an operator holding only a cheaper tier (e.g.
// node.nano) could activate a node declaring trust and then exercise trust-tier
// rights — the RWA attestation privilege enforced in AttestRwa would be gated on
// a string the operator chose for itself.
//
// Checking here rather than at activation also means the binding is
// re-evaluated on every attestation, so revoking an operator's trust licenses
// immediately stops its trust-declared nodes from attesting, instead of
// leaving them permanently labelled from the moment of activation.
//
// Which license types back a given node type is declared by
// types.LicenseTypesForNodeType, not by string identity between the two
// vocabularies. A node type this module has no mapping for is rejected rather
// than admitted: allowed_node_types is an x/network param, so a type can be made
// activatable before anything here says what licenses it.
func (k Keeper) ensureNodeTypeLicensed(ctx context.Context, node networktypes.Node) error {
	licenseTypes, ok := types.LicenseTypesForNodeType(node.Type)
	if !ok {
		return errorsmod.Wrapf(types.ErrNodeTypeUnlicensed,
			"node %s declares unknown type %q", node.Address, node.Type)
	}

	count, err := k.licenseKeeper.CountActiveLicenses(ctx, node.Operator, licenseTypes, 1)
	if err != nil {
		return err
	}
	if count == 0 {
		return errorsmod.Wrapf(types.ErrNodeTypeUnlicensed,
			"operator %s holds no active %v license backing %q node %s", node.Operator, licenseTypes, node.Type, node.Address)
	}
	return nil
}
