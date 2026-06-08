package module

import (
	autocliv1 "cosmossdk.io/api/cosmos/autocli/v1"
	modulev1 "github.com/TrustedSmartChain/tsc/v3/api/distro/v1"
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
					Short:     "Query the current consensus parameters",
				},
				{
					RpcMethod:      "EpochDistribution",
					Use:            "epoch-distribution [epoch]",
					Short:          "Query the canonical distribution for an epoch",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "epoch"}},
				},
				{
					RpcMethod:      "DistributionVotes",
					Use:            "distribution-votes [epoch]",
					Short:          "Query all submitted votes for an epoch",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "epoch"}},
				},
				{
					RpcMethod:      "Claimed",
					Use:            "claimed [epoch] [nonce]",
					Short:          "Report whether a reward nonce has been claimed",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "epoch"}, {ProtoField: "nonce"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: modulev1.Msg_ServiceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "SubmitDistributionRoot",
					Use:       "submit-distribution-root [epoch] [merkle-root]",
					Short:     "Submit a node's merkle root for an epoch",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "epoch"},
						{ProtoField: "merkle_root"},
					},
				},
				{
					RpcMethod: "Claim",
					Use:       "claim [epoch] [nonce] [address] [amount] [proof]",
					Short:     "Claim a distribution reward via a merkle proof",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "epoch"},
						{ProtoField: "nonce"},
						{ProtoField: "address"},
						{ProtoField: "amount"},
						{ProtoField: "proof"},
					},
				},
				{
					RpcMethod: "ChallengeDistribution",
					Use:       "challenge-distribution [epoch]",
					Short:     "Challenge a pending epoch distribution (escrows the challenge bond)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "epoch"},
					},
				},
				{
					RpcMethod: "UpdateParams",
					Skip:      true,
				},
			},
		},
	}
}
