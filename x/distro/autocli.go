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
					RpcMethod:      "Distribution",
					Use:            "distribution [date]",
					Short:          "Query the canonical distribution for a day (YYYY-MM-DD)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "date"}},
				},
				{
					RpcMethod:      "DistributionVotes",
					Use:            "distribution-votes [date]",
					Short:          "Query all submitted votes for a day (YYYY-MM-DD)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "date"}},
				},
				{
					RpcMethod:      "Claimed",
					Use:            "claimed [date] [nonce]",
					Short:          "Report whether a reward nonce has been claimed",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "date"}, {ProtoField: "nonce"}},
				},
				{
					RpcMethod:      "ClaimsByDate",
					Use:            "claims-by-date [date]",
					Short:          "List the reward nonces claimed for a day (YYYY-MM-DD)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "date"}},
				},
				{
					RpcMethod:      "ClaimTotalByCategory",
					Use:            "claim-total-by-category [date]",
					Short:          "Query cumulative claimed totals by category for a day (YYYY-MM-DD)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "date"}},
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: modulev1.Msg_ServiceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "SubmitDistributionRoot",
					Use:       "submit-distribution-root [date] [merkle-root]",
					Short:     "Submit a node's merkle root for a day (YYYY-MM-DD)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "date"},
						{ProtoField: "merkle_root"},
					},
				},
				{
					RpcMethod: "Claim",
					Use:       "claim [date] [nonce] [address] [total] [proof]",
					Short:     "Claim a distribution reward via a merkle proof (pass the category breakdown with --categories key=amount)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "date"},
						{ProtoField: "nonce"},
						{ProtoField: "address"},
						{ProtoField: "total"},
						{ProtoField: "proof"},
					},
				},
				{
					RpcMethod: "ChallengeDistribution",
					Use:       "challenge-distribution [date]",
					Short:     "Challenge a pending day's distribution (escrows the challenge bond)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "date"},
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
