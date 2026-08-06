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

// Node type ids this module authorizes attestations for, as registered in
// x/network. They are compiled in rather than read from the registry: which
// tiers may attest is a protocol rule here, not chain configuration, so a node
// type registered on chain that is not named in an AttestationMsg's
// AllowedNodeTypes cannot attest at all.
const (
	NodeTypeTrust = "tsc.trust"
	NodeTypeNano  = "tsc.nano"
)

// MaxAttestationsPerMsg bounds the attestations array in a single
// MsgAttestRwa/MsgAttestRwu.
const MaxAttestationsPerMsg = 100

// MaxContractAddressLen bounds the contract_address field; the chain treats
// the value as opaque.
const MaxContractAddressLen = 128
