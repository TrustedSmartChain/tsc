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

// NodeTypeTrust is the node type currently allowed to submit RWA
// attestations.
//
// This is the last hardcoded node type in the module, and it is temporary. The
// node type -> license type mapping that used to live here is gone: x/network
// owns the registry and resolves the binding itself. What remains is the RWA
// entitlement, which is TSC-specific and so cannot move to webstack — it
// becomes a stored per-node-type record set by the node type's creator, at
// which point this constant and the string comparison in AttestRwa both go.
//
// Until then a node type registered in x/network cannot be granted RWA rights
// without a binary release, which is exactly the coupling being removed.
const NodeTypeTrust = "trust"

// MaxAttestationsPerMsg bounds the attestations array in a single
// MsgAttestRwa/MsgAttestRwu.
const MaxAttestationsPerMsg = 100

// MaxContractAddressLen bounds the contract_address field; the chain treats
// the value as opaque.
const MaxContractAddressLen = 128
