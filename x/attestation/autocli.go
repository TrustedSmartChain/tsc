package module

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"

	modulev1 "github.com/TrustedSmartChain/tsc/v4/api/attestation/v1"
)

// AutoCLIOptions implements the autocli.HasAutoCLIConfig interface.
func (am AppModule) AutoCLIOptions() *autocliv1.ModuleOptions {
	return &autocliv1.ModuleOptions{
		Query: &autocliv1.ServiceCommandDescriptor{
			Service: modulev1.Query_ServiceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "Params",
					Use:       "params",
					Short:     "Query the attestation module parameters",
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: modulev1.Msg_ServiceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					// The repeated attestations field doesn't map to
					// positional args; node software submits these msgs
					// programmatically.
					RpcMethod: "AttestRwa",
					Skip:      true,
				},
				{
					RpcMethod: "AttestRwu",
					Skip:      true,
				},
				{
					// Gov-gated; submitted via governance proposal.
					RpcMethod: "UpdateParams",
					Skip:      true,
				},
			},
		},
	}
}
