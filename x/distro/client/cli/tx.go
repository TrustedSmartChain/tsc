package cli

import (
	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"

	"github.com/TrustedSmartChain/tsc/x/distro/types"
)

// !NOTE: Must enable in module.go (disabled in favor of autocli.go)

// NewTxCmd returns a root CLI command handler for certain modules
// transaction commands.
func NewTxCmd() *cobra.Command {
	txCmd := &cobra.Command{
		Use:                        types.ModuleName,
		Short:                      types.ModuleName + " subcommands.",
		DisableFlagParsing:         true,
		SuggestionsMinimumDistance: 2,
		RunE:                       client.ValidateCmd,
	}

	txCmd.AddCommand(
		MsgUpdateParams(),
	)
	return txCmd
}

// Returns a CLI command handler for registering a
// contract for the module.
func MsgUpdateParams() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [minting-address] [receiving-address]",
		Short: "Update the params (must be submitted from the authority)",
		Args:  cobra.ExactArgs(2),
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
