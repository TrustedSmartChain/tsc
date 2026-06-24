package cli

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"

	"github.com/TrustedSmartChain/tsc/v3/x/node/types"
)

func NewQueryCmd() *cobra.Command {
	queryCmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      "Node query subcommands",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}
	queryCmd.AddCommand(QueryCheckInCmd())
	return queryCmd
}

func QueryCheckInCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "check-in [address] [date]",
		Short: "Query a node check-in by address and date (YYYY-MM-DD)",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientQueryContext(cmd)
			if err != nil {
				return err
			}

			queryClient := types.NewQueryClient(clientCtx)
			res, err := queryClient.CheckIn(cmd.Context(), &types.QueryCheckInRequest{
				Address: args[0],
				Date:    args[1],
			})
			if err != nil {
				return err
			}

			return clientCtx.PrintProto(res)
		},
	}
	flags.AddQueryFlagsToCmd(cmd)
	return cmd
}
