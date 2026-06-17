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
					Use:            "distribution [date] [distro-type]",
					Short:          "Query the canonical distribution for a (day, type)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "date"}, {ProtoField: "distro_type"}},
				},
				{
					RpcMethod:      "DistributionVotes",
					Use:            "distribution-votes [date] [distro-type]",
					Short:          "Query all submitted votes for a (day, type)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "date"}, {ProtoField: "distro_type"}},
				},
				{
					RpcMethod:      "Claimed",
					Use:            "claimed [date] [distro-type] [nonce]",
					Short:          "Report whether a reward nonce has been claimed",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "date"}, {ProtoField: "distro_type"}, {ProtoField: "nonce"}},
				},
				{
					RpcMethod:      "ClaimsByDate",
					Use:            "claims-by-date [date] [distro-type]",
					Short:          "List the reward nonces claimed for a (day, type)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "date"}, {ProtoField: "distro_type"}},
				},
				{
					RpcMethod:      "ClaimTotalByCategory",
					Use:            "claim-total-by-category [date] [distro-type]",
					Short:          "Query cumulative claimed totals by category for a (day, type)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{{ProtoField: "date"}, {ProtoField: "distro_type"}},
				},
				{
					RpcMethod: "Audit",
					Use:       "audit",
					Short:     "Run the module's invariants (claim budget, bond solvency) against current state",
				},
				{
					RpcMethod: "Distributions",
					Use:       "distributions",
					Short:     "List all distributions (date-ordered, paginated)",
				},
				{
					RpcMethod: "ActiveDistributions",
					Use:       "active-distributions",
					Short:     "List the in-flight (VOTING/PENDING/UNDER_REVIEW) distributions",
				},
			},
		},
		Tx: &autocliv1.ServiceCommandDescriptor{
			Service: modulev1.Msg_ServiceDesc.ServiceName,
			RpcCommandOptions: []*autocliv1.RpcCommandOptions{
				{
					RpcMethod: "SubmitDistributionRoot",
					Use:       "submit-distribution-root [date] [distro-type] [merkle-root]",
					Short:     "Submit a node's merkle root for a (day, type). Provide the header leaf with --version and --totals-by-category key=amount and its inclusion proof with --header-proof",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "date"},
						{ProtoField: "distro_type"},
						{ProtoField: "merkle_root"},
					},
				},
				{
					RpcMethod: "Claim",
					Use:       "claim [date] [distro-type] [nonce] [address] [category] [amount] [denom] [proof]",
					Short:     "Claim a single (category, amount) distribution reward via a merkle proof",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "date"},
						{ProtoField: "distro_type"},
						{ProtoField: "nonce"},
						{ProtoField: "address"},
						{ProtoField: "category"},
						{ProtoField: "amount"},
						{ProtoField: "denom"},
						{ProtoField: "proof"},
					},
				},
				{
					RpcMethod: "ChallengeDistribution",
					Use:       "challenge-distribution [date] [distro-type]",
					Short:     "Challenge a pending day's distribution (escrows the challenge bond)",
					PositionalArgs: []*autocliv1.PositionalArgDescriptor{
						{ProtoField: "date"},
						{ProtoField: "distro_type"},
					},
				},
				{
					RpcMethod: "UpdateParams",
					Skip:      true,
				},
				{
					// Authority-gated (governance); submitted via a gov proposal
					// rather than a direct CLI tx, like UpdateParams.
					RpcMethod: "ReviveDistribution",
					Skip:      true,
				},
			},
		},
	}
}
