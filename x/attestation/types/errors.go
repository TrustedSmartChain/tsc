package types

import "cosmossdk.io/errors"

var (
	ErrInvalidSigner      = errors.Register(ModuleName, 1100, "invalid signer address")
	ErrNodeNotFound       = errors.Register(ModuleName, 1101, "node not found")
	ErrNodeNotActive      = errors.Register(ModuleName, 1102, "node is not active")
	ErrRwaTrustOnly       = errors.Register(ModuleName, 1103, "only trust nodes may submit RWA attestations")
	ErrInvalidAttestation = errors.Register(ModuleName, 1104, "invalid attestation")
	ErrQuotaExceeded      = errors.Register(ModuleName, 1105, "daily attestation quota exceeded")
	ErrNotGaslessMsg      = errors.Register(ModuleName, 1106, "not a gasless msg of the attestation module")
	ErrNodeTypeUnlicensed = errors.Register(ModuleName, 1107, "node's operator holds no active license of the node's declared type")
)
