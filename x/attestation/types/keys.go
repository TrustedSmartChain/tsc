package types

import (
	"cosmossdk.io/collections"
)

var (
	// ParamsKey saves the current module params.
	ParamsKey = collections.NewPrefix(0)
	// RwaCountersKey saves the per-node daily RWA attestation counters.
	RwaCountersKey = collections.NewPrefix(1)
	// RwuCountersKey saves the per-node daily RWU attestation counters.
	RwuCountersKey = collections.NewPrefix(2)
)

const (
	ModuleName = "attestation"

	StoreKey = ModuleName

	QuerierRoute = ModuleName
)

// Node type identifiers: the protocol-level vocabulary a node declares at
// activation, which x/network validates against its allowed_node_types param
// and stores verbatim on the Node. The RWA trust-only rule keys off these.
//
// These are deliberately not license type ids; see LicenseTypesForNodeType.
const (
	NodeTypeTrust = "trust"
	NodeTypeNano  = "nano"
)

// NodeTypes lists every node type this module understands, in a stable order.
// It is what app wiring seeds into x/network's allowed_node_types.
var NodeTypes = []string{NodeTypeTrust, NodeTypeNano}

// Node license type ids, as registered in x/license and counted by x/network's
// license_types param. Namespaced so later non-node license types cannot
// collide with them.
const (
	LicenseTypeNodeTrust = "node.trust"
	LicenseTypeNodeNano  = "node.nano"
)

// licenseTypesByNodeType maps a declared node type to the license type ids that
// authorize it.
//
// The two vocabularies are kept separate on purpose. License type ids are
// namespaced SKUs whose lifecycle belongs to the license namespace owner; node
// types are protocol vocabulary carried in network params and in every emitted
// attestation event. Binding them by string identity made renaming a SKU
// silently unlicense running nodes, and forced exactly one SKU per node type —
// this mapping instead lets several license types back one node type, which is
// what a grandfathered tier or a mid-migration id needs.
var licenseTypesByNodeType = map[string][]string{
	NodeTypeTrust: {LicenseTypeNodeTrust},
	NodeTypeNano:  {LicenseTypeNodeNano},
}

// LicenseTypesForNodeType returns the license type ids that back nodeType, and
// whether the node type is known here.
//
// Unknown node types are unlicensable rather than unrestricted: allowed_node_types
// is an x/network param, so a type can become activatable before this module has
// been taught what license backs it, and that must not confer attestation rights.
func LicenseTypesForNodeType(nodeType string) ([]string, bool) {
	ids, ok := licenseTypesByNodeType[nodeType]
	return ids, ok
}

// NodeLicenseTypes returns every license type id backing some node type, in a
// stable order and without duplicates.
//
// This is the set that must exist in the x/license registry and in x/network's
// license_types param for activation and attestation to work at all. App wiring
// seeds both from here, so the registry, the param, and the mapping this module
// enforces cannot drift apart.
func NodeLicenseTypes() []string {
	ids := make([]string, 0, len(NodeTypes))
	seen := make(map[string]struct{}, len(NodeTypes))
	for _, nodeType := range NodeTypes {
		for _, id := range licenseTypesByNodeType[nodeType] {
			if _, dup := seen[id]; dup {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}
	return ids
}

// MaxAttestationsPerMsg bounds the attestations array in a single
// MsgAttestRwa/MsgAttestRwu.
const MaxAttestationsPerMsg = 100

// MaxContractAddressLen bounds the contract_address field; the chain treats
// the value as opaque.
const MaxContractAddressLen = 128
