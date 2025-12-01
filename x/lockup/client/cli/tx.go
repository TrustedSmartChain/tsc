package cli

import (
	"strconv"
	"time"

	"github.com/spf13/cobra"

	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/x/lockup/types"
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
		MsgLock(),
	)
	return txCmd
}

// Returns a CLI command handler for registering a
// contract for the module.
func MsgUpdateParams() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update-params [some-value]",
		Short: "Update the params (must be submitted from the authority)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			senderAddress := cliCtx.GetFromAddress()

			someValue, err := strconv.ParseBool(args[0])
			if err != nil {
				return err
			}

			msg := &types.MsgUpdateParams{
				Authority: senderAddress.String(),
				Params: types.Params{
					SomeValue: someValue,
				},
			}

			if err := msg.Validate(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

// MsgLock returns a CLI command handler for locking tokens
func MsgLock() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lock [lockup_amount] [lock_months]",
		Short: "Lock tokens for long-term staking",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			cliCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			lockupAmount, err := sdk.ParseCoinNormalized(args[0])
			if err != nil {
				return err
			}

			lockMonths, err := strconv.ParseInt(args[1], 10, 64)
			if err != nil {
				return err
			}

			// lets convert the lockTime to the calendar day, then set the unlockTime to the same time + lockMonths in seconds
			unlockTime := time.Now().AddDate(0, int(lockMonths), 0)

			lockup := &types.Lock{
				Amount:     lockupAmount,
				UnlockDate: unlockTime.Format(time.DateOnly),
			}

			msg := &types.MsgLock{
				Lockups:       []*types.Lock{lockup},
				LockupAddress: cliCtx.GetFromAddress().String(),
			}

			return tx.GenerateOrBroadcastTxCLI(cliCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
