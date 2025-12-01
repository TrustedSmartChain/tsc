package ante

import (
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	bankkeeper "github.com/cosmos/cosmos-sdk/x/bank/keeper"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1"

	"cosmossdk.io/x/feegrant"

	"github.com/TrustedSmartChain/tsc/x/lockup/helpers"
	"github.com/TrustedSmartChain/tsc/x/lockup/types"
	ibctransfertypes "github.com/cosmos/ibc-go/v8/modules/apps/transfer/types"
)

// NewLockedBalanceDecorator checks that the sender has sufficient unlocked balance for msgs that spend funds.
type LockedBalanceDecorator struct {
	accountKeeper ante.AccountKeeper
	bankKeeper    bankkeeper.Keeper
}

func NewLockedBalanceDecorator(accountKeeper ante.AccountKeeper, bankKeeper bankkeeper.Keeper) LockedBalanceDecorator {
	return LockedBalanceDecorator{
		accountKeeper: accountKeeper,
		bankKeeper:    bankKeeper,
	}
}

func (lbd LockedBalanceDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (newCtx sdk.Context, err error) {
	for _, msg := range tx.GetMsgs() {
		switch msg.(type) {
		case *banktypes.MsgSend:

			sendMsg := msg.(*banktypes.MsgSend)
			fromAddr, err := sdk.AccAddressFromBech32(sendMsg.FromAddress)
			if err != nil {
				return ctx, err
			}
			acc := lbd.accountKeeper.GetAccount(ctx, fromAddr)
			if ltsAcc, ok := acc.(*types.Account); ok {
				// Calculate locked balance
				locked := math.NewInt(0)
				blockTime := ctx.BlockTime()
				for unlockDate, lockup := range ltsAcc.Lockups {
					if helpers.IsLocked(blockTime, unlockDate) {
						locked = locked.Add(lockup.Amount.Amount)
					}
				}
				// Get total balance
				totalBalance := lbd.bankKeeper.GetBalance(ctx, fromAddr, sendMsg.Amount[0].Denom).Amount
				available := totalBalance.Sub(locked)
				if available.LT(sendMsg.Amount[0].Amount) {
					return ctx, errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "insufficient unlocked balance: available %s, required %s", available, sendMsg.Amount[0].Amount)
				}
			}

		case *banktypes.MsgMultiSend:

			multiSendMsg := msg.(*banktypes.MsgMultiSend)
			// Check each input
			for _, input := range multiSendMsg.Inputs {
				fromAddr, err := sdk.AccAddressFromBech32(input.Address)
				if err != nil {
					return ctx, err
				}
				acc := lbd.accountKeeper.GetAccount(ctx, fromAddr)
				if ltsAcc, ok := acc.(*types.Account); ok {
					// Calculate locked balance
					locked := math.NewInt(0)
					blockTime := ctx.BlockTime()
					for unlockDate, lockup := range ltsAcc.Lockups {
						if helpers.IsLocked(blockTime, unlockDate) {
							locked = locked.Add(lockup.Amount.Amount)
						}
					}
					// Get total balance for each coin in input
					for _, coin := range input.Coins {
						totalBalance := lbd.bankKeeper.GetBalance(ctx, fromAddr, coin.Denom).Amount
						available := totalBalance.Sub(locked)
						if available.LT(coin.Amount) {
							return ctx, errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "insufficient unlocked balance: available %s, required %s", available, coin.Amount)
						}
					}
				}
			}

		case *authztypes.MsgGrant: // Is this needed? This doesn't actually move funds, just grants permission.

			msgGrant := msg.(*authztypes.MsgGrant)
			// Check each input
			fromAddr, err := sdk.AccAddressFromBech32(msgGrant.Granter)
			if err != nil {
				return ctx, err
			}
			acc := lbd.accountKeeper.GetAccount(ctx, fromAddr)
			if ltsAcc, ok := acc.(*types.Account); ok {
				// Calculate locked balance
				locked := math.NewInt(0)
				blockTime := ctx.BlockTime()
				for unlockDate, lockup := range ltsAcc.Lockups {
					if helpers.IsLocked(blockTime, unlockDate) {
						locked = locked.Add(lockup.Amount.Amount)
					}
				}
				// Get total balance for each coin in input
				if msgGrant.Grant.Authorization.GetTypeUrl() != sdk.MsgTypeURL(&banktypes.SendAuthorization{}) {
					authorization, err := msgGrant.Grant.GetAuthorization()
					if err != nil {
						return ctx, err
					}
					sendAuth, ok := authorization.(*banktypes.SendAuthorization)
					if ok {
						for _, coin := range sendAuth.SpendLimit {
							totalBalance := lbd.bankKeeper.GetBalance(ctx, fromAddr, coin.Denom).Amount
							available := totalBalance.Sub(locked)
							if available.LT(coin.Amount) {
								return ctx, errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "insufficient unlocked balance: available %s, required %s", available, coin.Amount)
							}
						}
					}
				}
			}

		// TODO: flesh out these msg types to enforce locked balance checks
		case *authztypes.MsgExec:
		case *govtypes.MsgDeposit:
		case *distributiontypes.MsgFundCommunityPool:
		case *feegrant.MsgGrantAllowance:
		case *ibctransfertypes.MsgTransfer:
		case *types.MsgSendAndLock:
		case *types.MsgMultiSendAndLock:

		default:
			// skip other messages
		}
	}

	return next(ctx, tx, simulate)
}
