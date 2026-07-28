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

// Node type identifiers, matching the node licenses they are activated
// under. The RWA trust-only rule keys off these.
const (
	NodeTypeTrust = "tsc.node.trust"
	NodeTypeNano  = "tsc.node.nano"
)

// MaxAttestationsPerMsg bounds the attestations array in a single
// MsgAttestRwa/MsgAttestRwu.
const MaxAttestationsPerMsg = 100

// MaxContractAddressLen bounds the contract_address field; the chain treats
// the value as opaque.
const MaxContractAddressLen = 128
