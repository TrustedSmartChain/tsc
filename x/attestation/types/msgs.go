package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"

	networktypes "github.com/webstack-sdk/webstack/x/network/types"
)

var (
	_ sdk.Msg = &MsgAttestRwa{}
	_ sdk.Msg = &MsgAttestRwu{}
	_ sdk.Msg = &MsgUpdateParams{}
)

// AttestationMsg is an attestation message that names the node types allowed
// to send it.
//
// The rule lives on the message, one line each, rather than in the handlers: a
// new attestation kind that forgets to declare its node types will not compile
// as an AttestationMsg, so it cannot reach checkNode and be admitted by
// omission.
//
// These lists are compiled in. A node type registered in x/network that is not
// named here cannot attest, and adding one is a binary release — the node type
// registry is runtime configuration but this authorization is not.
type AttestationMsg interface {
	sdk.Msg

	// GetNodeAddress is the signing node, satisfied by the generated getter.
	GetNodeAddress() string

	// AllowedNodeTypes are the x/network node type ids permitted to send this
	// message. Empty would admit nothing, so every message names at least one.
	AllowedNodeTypes() []string
}

var (
	_ AttestationMsg = &MsgAttestRwa{}
	_ AttestationMsg = &MsgAttestRwu{}
)

// AllowedNodeTypes implements AttestationMsg. RWA attestations make claims
// about real-world assets, so only the trust tier may send them.
func (msg *MsgAttestRwa) AllowedNodeTypes() []string { return []string{NodeTypeTrust} }

// AllowedNodeTypes implements AttestationMsg. Usage reporting is open to both
// tiers, but named rather than assumed: a node type absent from this list is
// denied, so a tier added later reports nothing until it is added here too.
func (msg *MsgAttestRwu) AllowedNodeTypes() []string { return []string{NodeTypeTrust, NodeTypeNano} }

// GaslessMessages returns the msg type URLs this module admits with a zero
// fee, appended to the network module's list when composing the app's
// gasless allowlist.
func GaslessMessages() []string {
	return []string{
		sdk.MsgTypeURL(&MsgAttestRwa{}),
		sdk.MsgTypeURL(&MsgAttestRwu{}),
	}
}

func (msg *MsgAttestRwa) ValidateBasic() error {
	return validateAttestMsg(msg.NodeAddress, msg.Attestations)
}

func (msg *MsgAttestRwu) ValidateBasic() error {
	return validateAttestMsg(msg.NodeAddress, msg.Attestations)
}

func validateAttestMsg(nodeAddress string, attestations []ContractAttestation) error {
	// Canonical form, not merely decodable: the node address is a store key
	// for this module's daily counters, and a non-canonical bech32 alias
	// would be a second identity for the same account.
	if err := networktypes.ValidateCanonicalAddress("node", nodeAddress); err != nil {
		return err
	}
	if len(attestations) == 0 {
		return ErrInvalidAttestation.Wrap("attestations must not be empty")
	}
	if len(attestations) > MaxAttestationsPerMsg {
		return ErrInvalidAttestation.Wrapf("attestations length %d exceeds max %d", len(attestations), MaxAttestationsPerMsg)
	}
	for i, a := range attestations {
		if a.ContractAddress == "" {
			return ErrInvalidAttestation.Wrapf("attestation %d: contract_address must not be empty", i)
		}
		if len(a.ContractAddress) > MaxContractAddressLen {
			return ErrInvalidAttestation.Wrapf("attestation %d: contract_address exceeds %d bytes", i, MaxContractAddressLen)
		}
		if a.CurrentSupply.IsNil() {
			return ErrInvalidAttestation.Wrapf("attestation %d: current_supply must be set", i)
		}
		if a.CurrentSupply.IsNegative() {
			return ErrInvalidAttestation.Wrapf("attestation %d: current_supply must not be negative", i)
		}
	}
	return nil
}

func (msg *MsgUpdateParams) ValidateBasic() error {
	if _, err := sdk.AccAddressFromBech32(msg.Authority); err != nil {
		return ErrInvalidSigner.Wrapf("invalid authority address: %s", err)
	}
	return msg.Params.Validate()
}
