package keeper_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	networktypes "github.com/webstack-sdk/webstack/x/network/types"

	"github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

// TestNanoLicenseCannotDeclareTrustNode is the regression test for the tier
// escalation found in security review: x/network stores Node.Type as the
// opaque string its operator supplied and counts licenses in aggregate, so an
// operator holding only a nano license could activate a node declaring trust
// and thereby pass the RWA trust-only gate. Attestation now requires a license
// of a type that backs the declared node type.
func TestNanoLicenseCannotDeclareTrustNode(t *testing.T) {
	f := SetupTest(t)

	// A node that declares trust, whose operator holds only a nano license —
	// exactly what aggregate license counting admits at activation time.
	node := f.network.addNode(types.NodeTypeTrust, networktypes.NodeActive)
	f.license.issue(node.Operator, types.LicenseTypeNodeNano, 1)

	_, err := f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.ErrorIs(t, err, types.ErrNodeTypeUnlicensed)

	// RWU is bound the same way: the declared type must be backed, whatever
	// the type is.
	_, err = f.msgServer.AttestRwu(f.ctx, &types.MsgAttestRwu{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.ErrorIs(t, err, types.ErrNodeTypeUnlicensed)

	// No state was mutated by the rejected attestations.
	require.Empty(t, f.network.touched)

	// Issuing a license of the type that backs trust admits it.
	f.license.issue(node.Operator, types.LicenseTypeNodeTrust, 1)
	_, err = f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.NoError(t, err)
}

// TestUnknownNodeTypeIsUnlicensable: allowed_node_types is an x/network param,
// so a node type can be made activatable before this module has been taught
// which license backs it. Such a node must be rejected rather than admitted —
// under the old identity-based check it would instead have been admitted by
// holding a license whose id happened to equal the new type's string.
func TestUnknownNodeTypeIsUnlicensable(t *testing.T) {
	f := SetupTest(t)

	const unmapped = "quantum"
	_, ok := types.LicenseTypesForNodeType(unmapped)
	require.False(t, ok, "fixture assumes %q has no mapping", unmapped)

	node := f.network.addNode(unmapped, networktypes.NodeActive)
	// Licensed generously, including a license id equal to the node type — the
	// rejection is about the type being unmapped, not about holding nothing.
	f.license.issue(node.Operator, types.LicenseTypeNodeTrust, 5)
	f.license.issue(node.Operator, unmapped, 5)

	_, err := f.msgServer.AttestRwu(f.ctx, &types.MsgAttestRwu{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.ErrorIs(t, err, types.ErrNodeTypeUnlicensed)
	require.Empty(t, f.network.touched)
}

// TestNodeTypeBindingIsRecheckedPerAttestation: because the binding is
// verified on every attestation rather than frozen at activation, revoking
// the backing license immediately stops the node — it does not stay
// trust-labelled forever.
func TestNodeTypeBindingIsRecheckedPerAttestation(t *testing.T) {
	f := SetupTest(t)
	node := f.addNode(types.NodeTypeTrust, networktypes.NodeActive)

	_, err := f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.NoError(t, err)

	f.license.revoke(node.Operator, types.LicenseTypeNodeTrust)

	_, err = f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.ErrorIs(t, err, types.ErrNodeTypeUnlicensed)
}

// TestTrustLicenseDoesNotPromoteNanoNode: holding a trust license does not
// let a node the operator declared as nano exercise the RWA privilege. The
// declared type still gates RWA; the license check only ensures the
// declaration is backed.
func TestTrustLicenseDoesNotPromoteNanoNode(t *testing.T) {
	f := SetupTest(t)

	node := f.addNode(types.NodeTypeNano, networktypes.NodeActive)
	f.license.issue(node.Operator, types.LicenseTypeNodeTrust, 1)

	_, err := f.msgServer.AttestRwa(f.ctx, &types.MsgAttestRwa{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.ErrorIs(t, err, types.ErrRwaTrustOnly)

	// RWU is accepted from the nano node, which is licensed for its type.
	_, err = f.msgServer.AttestRwu(f.ctx, &types.MsgAttestRwu{
		NodeAddress:  node.Address,
		Attestations: attestations(1),
	})
	require.NoError(t, err)
}
