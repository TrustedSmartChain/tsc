package types

import "cosmossdk.io/errors"

var (
	ErrInvalidSigner      = errors.Register(ModuleName, 1100, "invalid signer address")
	ErrNodeNotFound       = errors.Register(ModuleName, 1101, "node not found")
	ErrNodeNotActive      = errors.Register(ModuleName, 1102, "node is not active")
	ErrNodeTypeNotAllowed = errors.Register(ModuleName, 1103, "node's type may not send this attestation")
	ErrInvalidAttestation = errors.Register(ModuleName, 1104, "invalid attestation")
	ErrQuotaExceeded      = errors.Register(ModuleName, 1105, "daily attestation quota exceeded")
	ErrNotGaslessMsg      = errors.Register(ModuleName, 1106, "not a gasless msg of the attestation module")

	// 1107 was ErrNodeTypeUnlicensed, raised by this module's own node
	// type -> license type mapping. x/network owns that binding now and
	// reports it as ErrInvalidNodeType / ErrNoActiveLicenses, which this
	// module propagates. The code is retired rather than reused.
)
