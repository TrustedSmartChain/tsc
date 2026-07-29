package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

// TestNodeTypesAndLicenseTypesAreDistinct pins the property the mapping exists
// to provide: the node type vocabulary and the license type vocabulary are
// separate namespaces. A change that collapses them back into string identity
// re-introduces the coupling this mapping replaced, where renaming a license SKU
// silently unlicensed every running node of the corresponding type.
func TestNodeTypesAndLicenseTypesAreDistinct(t *testing.T) {
	nodeTypes := make(map[string]struct{}, len(types.NodeTypes))
	for _, nt := range types.NodeTypes {
		nodeTypes[nt] = struct{}{}
	}

	for _, id := range types.NodeLicenseTypes() {
		_, collides := nodeTypes[id]
		require.False(t, collides, "license type %q collides with a node type", id)
	}
}

// TestEveryNodeTypeIsBacked: a node type with no mapping cannot attest at all
// (ensureNodeTypeLicensed fails closed), so shipping one would silently disable
// it. Every declared node type must name at least one license type.
func TestEveryNodeTypeIsBacked(t *testing.T) {
	for _, nodeType := range types.NodeTypes {
		licenseTypes, ok := types.LicenseTypesForNodeType(nodeType)
		require.True(t, ok, "node type %q has no license mapping", nodeType)
		require.NotEmpty(t, licenseTypes, "node type %q maps to no license types", nodeType)
	}
}

func TestLicenseTypesForNodeType(t *testing.T) {
	trust, ok := types.LicenseTypesForNodeType(types.NodeTypeTrust)
	require.True(t, ok)
	require.Equal(t, []string{types.LicenseTypeNodeTrust}, trust)

	nano, ok := types.LicenseTypesForNodeType(types.NodeTypeNano)
	require.True(t, ok)
	require.Equal(t, []string{types.LicenseTypeNodeNano}, nano)

	// Unknown types report absence rather than an empty slice, so callers can
	// distinguish "not configured" from "configured as unlicensable".
	for _, unknown := range []string{"", "quantum", types.LicenseTypeNodeTrust, "TRUST"} {
		ids, ok := types.LicenseTypesForNodeType(unknown)
		require.False(t, ok, "node type %q unexpectedly mapped", unknown)
		require.Nil(t, ids)
	}
}

// TestNodeLicenseTypesIsStableAndDeduplicated: the result seeds both the
// x/license registry and x/network's license_types param in the v3 handler, so
// it must not vary between calls — map iteration order must not leak — and must
// not repeat an id when several node types share one license type.
func TestNodeLicenseTypesIsStableAndDeduplicated(t *testing.T) {
	first := types.NodeLicenseTypes()
	require.Equal(t, []string{types.LicenseTypeNodeTrust, types.LicenseTypeNodeNano}, first)

	for i := 0; i < 20; i++ {
		require.Equal(t, first, types.NodeLicenseTypes(), "ordering is not stable across calls")
	}

	seen := make(map[string]struct{}, len(first))
	for _, id := range first {
		_, dup := seen[id]
		require.False(t, dup, "duplicate license type %q", id)
		seen[id] = struct{}{}
	}

	// Every mapped license type is reachable from the exported set, so seeding
	// from it cannot miss one that ensureNodeTypeLicensed would then demand.
	for _, nodeType := range types.NodeTypes {
		licenseTypes, _ := types.LicenseTypesForNodeType(nodeType)
		for _, id := range licenseTypes {
			require.Contains(t, first, id)
		}
	}
}
