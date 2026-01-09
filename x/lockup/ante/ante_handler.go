package ante

import (
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	"github.com/TrustedSmartChain/tsc/x/lockup/keeper"
	"github.com/TrustedSmartChain/tsc/x/lockup/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"
	authztypes "github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	distributiontypes "github.com/cosmos/cosmos-sdk/x/distribution/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types/v1"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"cosmossdk.io/x/feegrant"

	ibctransfertypes "github.com/cosmos/ibc-go/v10/modules/apps/transfer/types"
)

type LockedDelegationsDecorator struct {
	accountKeeper types.AccountKeeper
	bankKeeper    types.BankKeeper
	lockupKeeper  keeper.Keeper
	stakingKeeper types.StakingKeeper
}

func NewLockedDelegationsDecorator(accountKeeper types.AccountKeeper, bankKeeper types.BankKeeper, lockupKeeper keeper.Keeper, stakingKeeper types.StakingKeeper) LockedDelegationsDecorator {
	return LockedDelegationsDecorator{
		accountKeeper: accountKeeper,
		bankKeeper:    bankKeeper,
		lockupKeeper:  lockupKeeper,
		stakingKeeper: stakingKeeper,
	}
}

func (d LockedDelegationsDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (newCtx sdk.Context, err error) {
	// Skip during genesis (block height 0) as staking params aren't initialized yet
	// and there won't be any locks at genesis anyway
	if ctx.BlockHeight() == 0 {
		return next(ctx, tx, simulate)
	}

	msgs := tx.GetMsgs()

	bondDenom, err := d.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return ctx, err
	}

	err = d.handleMsgs(ctx, msgs, bondDenom)
	if err != nil {
		return ctx, err
	}

	return next(ctx, tx, simulate)
}

func (d LockedDelegationsDecorator) handleMsgs(ctx sdk.Context, msgs []sdk.Msg, bondDenom string) error {
	for _, msg := range msgs {
		switch m := msg.(type) {
		case *banktypes.MsgSend:

			fromAddr, err := sdk.AccAddressFromBech32(m.FromAddress)
			if err != nil {
				return err
			}

			ok, lockedAboveDelegated, err := checkDelegationsAgainstLocked(ctx, fromAddr, d.lockupKeeper)
			if err != nil {
				return err
			}

			if ok {
				return nil
			}

			for _, coin := range m.Amount {
				if coin.Denom != bondDenom {
					continue
				}

				totalBalance := d.bankKeeper.GetBalance(ctx, fromAddr, coin.Denom).Amount
				available := totalBalance.Sub(*lockedAboveDelegated)
				if available.LT(coin.Amount) {
					if available.IsNegative() {
						available = math.ZeroInt()
					}
					return errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "insufficient unlocked balance: available %s, required %s", available, coin.Amount)
				}
			}

		case *banktypes.MsgMultiSend:

			// Check each input
			for _, input := range m.Inputs {
				fromAddr, err := sdk.AccAddressFromBech32(input.Address)
				if err != nil {
					return err
				}
				ok, lockedAboveDelegated, err := checkDelegationsAgainstLocked(ctx, fromAddr, d.lockupKeeper)
				if err != nil {
					return err
				}

				if ok {
					continue
				}

				for _, coin := range input.Coins {
					if coin.Denom != bondDenom {
						continue
					}

					totalBalance := d.bankKeeper.GetBalance(ctx, fromAddr, coin.Denom).Amount
					available := totalBalance.Sub(*lockedAboveDelegated)
					if available.LT(coin.Amount) {
						if available.IsNegative() {
							available = math.ZeroInt()
						}
						return errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "insufficient unlocked balance: available %s, required %s", available, coin.Amount)
					}
				}
			}

		case *authztypes.MsgGrant: // Is this needed? This doesn't actually move funds, just grants permission.

			fromAddr, err := sdk.AccAddressFromBech32(m.Granter)
			if err != nil {
				return err
			}

			ok, lockedAboveDelegated, err := checkDelegationsAgainstLocked(ctx, fromAddr, d.lockupKeeper)
			if err != nil {
				return err
			}

			if ok {
				return nil
			}

			if m.Grant.Authorization.GetTypeUrl() == sdk.MsgTypeURL(&banktypes.SendAuthorization{}) {
				authorization, err := m.Grant.GetAuthorization()
				if err != nil {
					return err
				}
				sendAuth, ok := authorization.(*banktypes.SendAuthorization)
				if ok {
					for _, coin := range sendAuth.SpendLimit {
						if coin.Denom != bondDenom {
							continue
						}

						totalBalance := d.bankKeeper.GetBalance(ctx, fromAddr, coin.Denom).Amount
						available := totalBalance.Sub(*lockedAboveDelegated)
						if available.LT(coin.Amount) {
							if available.IsNegative() {
								available = math.ZeroInt()
							}
							return errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "insufficient unlocked balance: available %s, required %s", available, coin.Amount)
						}
					}
				}
			}

		case *stakingtypes.MsgUndelegate:

			if m.Amount.Denom != bondDenom {
				return nil
			}

			fromAddr, err := sdk.AccAddressFromBech32(m.DelegatorAddress)
			if err != nil {
				return err
			}

			totalLocked, err := d.lockupKeeper.GetLockedAmountByAddress(ctx, fromAddr)
			if err != nil {
				return err
			}

			totalDelegated, err := d.lockupKeeper.GetTotalDelegatedAmount(ctx, fromAddr)
			if err != nil {
				return err
			}
			delegatedAfterUndelegate := totalDelegated.Sub(m.Amount.Amount)

			if delegatedAfterUndelegate.LT(*totalLocked) {
				return errorsmod.Wrapf(
					types.ErrInsufficientDelegations,
					"unbond would cause new delegated amount to be less than the locked amount: %s < %s",
					delegatedAfterUndelegate.String(),
					totalLocked.String(),
				)
			}

		case *authztypes.MsgExec:

			msgs, err := m.GetMessages()
			if err != nil {
				return err
			}

			err = d.handleMsgs(ctx, msgs, bondDenom)
			if err != nil {
				return err
			}

		case *govtypes.MsgDeposit:

			fromAddr, err := sdk.AccAddressFromBech32(m.Depositor)
			if err != nil {
				return err
			}

			ok, lockedAboveDelegated, err := checkDelegationsAgainstLocked(ctx, fromAddr, d.lockupKeeper)
			if err != nil {
				return err
			}

			if ok {
				return nil
			}

			for _, coin := range m.Amount {
				if coin.Denom != bondDenom {
					continue
				}

				totalBalance := d.bankKeeper.GetBalance(ctx, fromAddr, coin.Denom).Amount
				available := totalBalance.Sub(*lockedAboveDelegated)
				if available.LT(coin.Amount) {
					if available.IsNegative() {
						available = math.ZeroInt()
					}
					return errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "insufficient unlocked balance: available %s, required %s", available, coin.Amount)
				}
			}

		case *distributiontypes.MsgFundCommunityPool:

			fromAddr, err := sdk.AccAddressFromBech32(m.Depositor)
			if err != nil {
				return err
			}

			ok, lockedAboveDelegated, err := checkDelegationsAgainstLocked(ctx, fromAddr, d.lockupKeeper)
			if err != nil {
				return err
			}

			if ok {
				return nil
			}

			for _, coin := range m.Amount {
				if coin.Denom != bondDenom {
					continue
				}

				totalBalance := d.bankKeeper.GetBalance(ctx, fromAddr, coin.Denom).Amount
				available := totalBalance.Sub(*lockedAboveDelegated)
				if available.LT(coin.Amount) {
					if available.IsNegative() {
						available = math.ZeroInt()
					}
					return errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "insufficient unlocked balance: available %s, required %s", available, coin.Amount)
				}
			}

		case *distributiontypes.MsgDepositValidatorRewardsPool:

			fromAddr, err := sdk.AccAddressFromBech32(m.Depositor)
			if err != nil {
				return err
			}

			ok, lockedAboveDelegated, err := checkDelegationsAgainstLocked(ctx, fromAddr, d.lockupKeeper)
			if err != nil {
				return err
			}

			if ok {
				return nil
			}

			for _, coin := range m.Amount {
				if coin.Denom != bondDenom {
					continue
				}

				totalBalance := d.bankKeeper.GetBalance(ctx, fromAddr, coin.Denom).Amount
				available := totalBalance.Sub(*lockedAboveDelegated)
				if available.LT(coin.Amount) {
					if available.IsNegative() {
						available = math.ZeroInt()
					}
					return errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "insufficient unlocked balance: available %s, required %s", available, coin.Amount)
				}
			}

		case *feegrant.MsgGrantAllowance:

			fromAddr, err := sdk.AccAddressFromBech32(m.Granter)
			if err != nil {
				return err
			}

			ok, lockedAboveDelegated, err := checkDelegationsAgainstLocked(ctx, fromAddr, d.lockupKeeper)
			if err != nil {
				return err
			}

			if ok {
				return nil
			}

			var coins []sdk.Coin
			allowance, ok := m.Allowance.GetCachedValue().(feegrant.FeeAllowanceI)
			if !ok {
				return nil
			}

			switch a := allowance.(type) {
			case *feegrant.BasicAllowance:
				if a.SpendLimit == nil {
					return nil
				}
				coins = a.SpendLimit
			case *feegrant.PeriodicAllowance:
				if a.Basic.SpendLimit == nil {
					return nil
				}
				coins = a.Basic.SpendLimit
			case *feegrant.AllowedMsgAllowance:
				allowanceInner, ok := a.Allowance.GetCachedValue().(feegrant.FeeAllowanceI)
				if !ok {
					return nil
				}

				switch ai := allowanceInner.(type) {
				case *feegrant.BasicAllowance:
					if ai.SpendLimit == nil {
						return nil
					}
					coins = ai.SpendLimit
				case *feegrant.PeriodicAllowance:
					if ai.Basic.SpendLimit == nil {
						return nil
					}
					coins = ai.Basic.SpendLimit
				default:
					return nil
				}
			default:
				return nil
			}

			for _, coin := range coins {
				if coin.Denom != bondDenom {
					continue
				}

				totalBalance := d.bankKeeper.GetBalance(ctx, fromAddr, coin.Denom).Amount
				available := totalBalance.Sub(*lockedAboveDelegated)
				if available.LT(coin.Amount) {
					if available.IsNegative() {
						available = math.ZeroInt()
					}
					return errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "insufficient unlocked balance: available %s, required %s", available, coin.Amount)
				}
			}

		case *ibctransfertypes.MsgTransfer:

			fromAddr, err := sdk.AccAddressFromBech32(m.Sender)
			if err != nil {
				return err
			}

			ok, lockedAboveDelegated, err := checkDelegationsAgainstLocked(ctx, fromAddr, d.lockupKeeper)
			if err != nil {
				return err
			}

			if ok {
				return nil
			}

			if m.Token.Denom != bondDenom {
				return nil
			}

			totalBalance := d.bankKeeper.GetBalance(ctx, fromAddr, m.Token.Denom).Amount
			available := totalBalance.Sub(*lockedAboveDelegated)
			if available.LT(m.Token.Amount) {
				if available.IsNegative() {
					available = math.ZeroInt()
				}
				return errorsmod.Wrapf(errortypes.ErrInsufficientFunds, "insufficient unlocked balance: available %s, required %s", available, m.Token.Amount)
			}

		default:
			continue
		}
	}

	return nil
}

// checkDelegationsAgainstLocked checks if the total delegated amount is greater than the total locked amount.
// Returns true if total delegated amount is greater than total locked amount.
func checkDelegationsAgainstLocked(ctx sdk.Context, addr sdk.AccAddress, lockupKeeper keeper.Keeper) (bool, *math.Int, error) {

	delegationsTotal, err := lockupKeeper.GetTotalDelegatedAmount(ctx, addr)
	if err != nil {
		return false, nil, err
	}

	totalLocked, err := lockupKeeper.GetLockedAmountByAddress(ctx, addr)
	if err != nil {
		return false, nil, err
	}

	lockedAboveDelegated := totalLocked.Sub(*delegationsTotal)
	if lockedAboveDelegated.IsNegative() {
		lockedAboveDelegated = math.ZeroInt()
	}

	return delegationsTotal.GTE(*totalLocked), &lockedAboveDelegated, nil

}
