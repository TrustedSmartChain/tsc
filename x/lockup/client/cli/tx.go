package cli

import (
	"fmt"
	"strings"

	"cosmossdk.io/math"
	"github.com/cosmos/cosmos-sdk/client"
	"github.com/cosmos/cosmos-sdk/client/flags"
	"github.com/cosmos/cosmos-sdk/client/tx"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/spf13/cobra"

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
		CmdLock(),
		CmdExtend(),
		CmdSendDelegateAndLock(),
		CmdMultiSendDelegateAndLock(),
	)
	return txCmd
}

func CmdLock() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lock unlock-date amount",
		Short: "Lock tokens until a specific date",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			unlockDate := args[0]
			amount, ok := math.NewIntFromString(args[1])
			if !ok {
				return fmt.Errorf("invalid amount: %s", args[1])
			}

			msg := &types.MsgLock{
				Address:    clientCtx.GetFromAddress().String(),
				UnlockDate: unlockDate,
				Amount:     sdk.NewCoin("aTSC", amount),
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func CmdExtend() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "extend [from-date:to-date:amount] [from-date:to-date:amount]...",
		Short: "Extend lock unlock dates",
		Long: `Extend the unlock date of existing locks. You can specify multiple extensions.
		Example: '2026-12-01:2027-12-01:1000000000' extends a lock that unlocks on 2026-12-01 by locking an additional 1000000000 until 2027-12-01.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			extensions := make([]*types.Extension, 0, len(args))
			for i, arg := range args {
				parts := strings.Split(arg, ":")
				if len(parts) != 3 {
					return fmt.Errorf("invalid extension format at position %d: expected 'from-date:to-date:amount', got '%s'", i, arg)
				}

				fromDate := parts[0]
				toDate := parts[1]
				amount, ok := math.NewIntFromString(parts[2])
				if !ok {
					return fmt.Errorf("invalid amount at position %d: %s", i, parts[2])
				}

				extensions = append(extensions, &types.Extension{
					FromDate: fromDate,
					ToDate:   toDate,
					Amount:   sdk.NewCoin("aTSC", amount),
				})
			}

			msg := &types.MsgExtend{
				Address:    clientCtx.GetFromAddress().String(),
				Extensions: extensions,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func CmdSendDelegateAndLock() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "send-delegate-and-lock [to-address] [validator-address] [unlock-date] [amount]",
		Short: "Send tokens to an address, delegate them to a validator, and lock them",
		Long: `Send tokens from your address to another address, delegate them to a validator, and lock them until a specific unlock date.
Example: 
  send-delegate-and-lock optio1abc... optiovaloper1xyz... 2026-12-01 1000`,
		Args: cobra.ExactArgs(4),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			toAddress := args[0]
			validatorAddress := args[1]
			unlockDate := args[2]
			amount, ok := math.NewIntFromString(args[3])
			if !ok {
				return fmt.Errorf("invalid amount: %s", args[3])
			}

			msg := &types.MsgSendDelegateAndLock{
				FromAddress:      clientCtx.GetFromAddress().String(),
				ToAddress:        toAddress,
				ValidatorAddress: validatorAddress,
				UnlockDate:       unlockDate,
				Amount:           sdk.NewCoin("aTSC", amount),
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}

func CmdMultiSendDelegateAndLock() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "multi-send-delegate-and-lock [to-address:validator-address:unlock-date:amount] [to-address:validator-address:unlock-date:amount]...",
		Short: "Send tokens to multiple addresses, delegate them, and lock them",
		Long: `Send tokens to multiple addresses, delegate them to validators, and lock them until specific unlock dates.
Example: 
  multi-send-delegate-and-lock optio1abc...:optiovaloper1xyz...:2026-12-01:1000 optio1def...:optiovaloper1uvw...:2027-01-01:2000`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			clientCtx, err := client.GetClientTxContext(cmd)
			if err != nil {
				return err
			}

			outputs := make([]*types.MultiSendDelegateAndLockOutput, 0, len(args))
			totalAmount := math.ZeroInt()

			for i, arg := range args {
				parts := strings.Split(arg, ":")
				if len(parts) != 4 {
					return fmt.Errorf("invalid output format at position %d: expected 'to-address:validator-address:unlock-date:amount', got '%s'", i, arg)
				}

				toAddress := parts[0]
				validatorAddress := parts[1]
				unlockDate := parts[2]
				amount, ok := math.NewIntFromString(parts[3])
				if !ok {
					return fmt.Errorf("invalid amount at position %d: %s", i, parts[3])
				}

				totalAmount = totalAmount.Add(amount)

				outputs = append(outputs, &types.MultiSendDelegateAndLockOutput{
					ToAddress:        toAddress,
					ValidatorAddress: validatorAddress,
					UnlockDate:       unlockDate,
					Amount:           sdk.NewCoin("aTSC", amount),
				})
			}

			msg := &types.MsgMultiSendDelegateAndLock{
				FromAddress: clientCtx.GetFromAddress().String(),
				TotalAmount: sdk.NewCoin("aTSC", totalAmount),
				Outputs:     outputs,
			}

			if err := msg.ValidateBasic(); err != nil {
				return err
			}

			return tx.GenerateOrBroadcastTxCLI(clientCtx, cmd.Flags(), msg)
		},
	}

	flags.AddTxFlagsToCmd(cmd)
	return cmd
}
