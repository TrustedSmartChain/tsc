package types

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
)

var (
	_ sdk.Msg = &MsgAttestRwa{}
	_ sdk.Msg = &MsgAttestRwu{}
	_ sdk.Msg = &MsgUpdateParams{}
)

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
	if _, err := sdk.AccAddressFromBech32(nodeAddress); err != nil {
		return ErrInvalidSigner.Wrapf("invalid node address: %s", err)
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
