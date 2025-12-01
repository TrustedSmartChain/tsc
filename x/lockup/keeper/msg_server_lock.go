package keeper

import (
	"context"
	"time"

	cosmossdk_io_math "cosmossdk.io/math"

	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	// errorsmod "cosmossdk.io/errors"
	// "cosmossdk.io/math"
	"github.com/TrustedSmartChain/tsc/x/lockup/helpers"
	"github.com/TrustedSmartChain/tsc/x/lockup/types"
	sdk "github.com/cosmos/cosmos-sdk/types"

	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

func (ms msgServer) Lock(goCtx context.Context, msg *types.MsgLock) (*types.MsgLockResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	lockupAddr, err := sdk.AccAddressFromBech32(msg.LockupAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid lockup address: %s", err)
	}
	acc := ms.k.accountKeeper.GetAccount(ctx, lockupAddr)

	if acc == nil {
		// create a new lockup account if it doesn't exist
		baseAcc := authtypes.NewBaseAccountWithAddress(lockupAddr)
		longTermStakeAcc := types.NewLongTermStakeAccount(baseAcc)
		ms.k.accountKeeper.SetAccount(ctx, longTermStakeAcc)
		acc = longTermStakeAcc
	} else {
		acc = ms.k.accountKeeper.GetAccount(ctx, lockupAddr)
		_, ok := acc.(*types.Account)
		if !ok {
			longTermStakeAcc := types.NewLongTermStakeAccount(acc.(*authtypes.BaseAccount))
			ms.k.accountKeeper.SetAccount(ctx, longTermStakeAcc)
			acc = longTermStakeAcc
		}
	}

	ltsAcc := acc.(*types.Account)
	var totalLockedAmount cosmossdk_io_math.Int
	for unlockDate, lock := range ltsAcc.Lockups {
		if helpers.IsLocked(ctx.BlockTime(), unlockDate) {
			totalLockedAmount = totalLockedAmount.Add(lock.Amount.Amount)
		}
	}

	bondDenom := stakingtypes.DefaultParams().BondDenom
	for _, lock := range msg.Lockups {
		if !lock.Amount.IsValid() || lock.Amount.IsZero() {
			return nil, sdkerrors.ErrInvalidCoins.Wrapf("invalid lock amount: %s", lock.Amount.String())
		}

		if lock.Amount.Denom != bondDenom {
			return nil, sdkerrors.ErrInvalidCoins.Wrapf("invalid denom: %s, expected: %s", lock.Amount.Denom, bondDenom)
		}

		// TODO: Do we want to only allow lockups for 6,12,18,24 months? Or should it be anything in between?
		// TODO: Remove any time skew by rounding to the nearest day?
		// QUESTION: Should we change the lockup request to take in months instead of a unix timestamp?

		unlockTime, err := time.Parse(time.DateOnly, lock.UnlockDate)
		if err != nil {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid unlock date format: %s", lock.UnlockDate)
		}

		if ctx.BlockTime().AddDate(0, 6, 0).Before(unlockTime) {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("unlock time must be at least 6 months from now")
		}
		if ctx.BlockTime().AddDate(2, 0, 0).After(unlockTime) {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("unlock time cannot be more than 2 years from now")
		}
		totalLockedAmount = totalLockedAmount.Add(lock.Amount.Amount)
	}

	totalBalance := ms.k.bankKeeper.GetBalance(ctx, lockupAddr, bondDenom)
	if totalBalance.Amount.LT(totalLockedAmount) {
		return nil, sdkerrors.ErrInsufficientFunds.Wrapf("trying to lock more %s than available: %s < %s", bondDenom, totalLockedAmount.String(), totalBalance.Amount.String())
	}

	for _, msgLockup := range msg.Lockups {
		existingLockup, exists := ltsAcc.Lockups[msgLockup.UnlockDate]
		if exists {
			existingLockup.Amount.Amount = existingLockup.Amount.Amount.Add(msgLockup.Amount.Amount)
			ltsAcc.Lockups[msgLockup.UnlockDate] = existingLockup
		} else {
			ltsAcc.Lockups[msgLockup.UnlockDate] = &types.Lockup{
				Amount: msgLockup.Amount,
			}
		}
	}

	ms.k.accountKeeper.SetAccount(ctx, ltsAcc)

	return &types.MsgLockResponse{}, nil
}

// ExtendLocks implements types.MsgServer.
func (ms msgServer) ExtendLocks(goCtx context.Context, msg *types.MsgExtend) (*types.MsgExtendResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	newLockups := map[string]*types.Lock{}
	for _, extension := range msg.Extensions {
		from, err := time.Parse(time.DateOnly, extension.From)
		if err != nil {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid from format: %s", extension.From)
		}

		to, err := time.Parse(time.DateOnly, extension.Lock.UnlockDate)
		if err != nil {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("invalid unlock date format: %s", extension.Lock.UnlockDate)
		}

		if !to.After(from) {
			return nil, sdkerrors.ErrInvalidRequest.Wrapf("unlock date must be after from date")
		}

		existingLock, exists := newLockups[extension.Lock.UnlockDate]
		if exists {
			existingLock.Amount.Amount = existingLock.Amount.Amount.Add(extension.Lock.Amount.Amount)
			newLockups[extension.Lock.UnlockDate] = existingLock
			continue
		}
		newLockups[extension.Lock.UnlockDate] = extension.Lock
	}

	addr, err := sdk.AccAddressFromBech32(msg.ExtendingAddress)
	if err != nil {
		return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid lockup address: %s", err)
	}

	acc := ms.k.accountKeeper.GetAccount(ctx, addr)
	if acc == nil {
		return nil, sdkerrors.ErrNotFound.Wrapf("no account found for address: %s", msg.ExtendingAddress)
	}

	ltsAcc, ok := acc.(*types.Account)
	if !ok {
		return nil, types.ErrInvalidAccount.Wrapf("account is not a long-term stake account: %s", msg.ExtendingAddress)
	}

	for _, lockups := range newLockups {
		existingLockup, exists := ltsAcc.Lockups[lockups.UnlockDate]
		if exists {
			existingLockup.Amount.Amount = existingLockup.Amount.Amount.Add(lockups.Amount.Amount)
			ltsAcc.Lockups[lockups.UnlockDate] = existingLockup
		} else {
			ltsAcc.Lockups[lockups.UnlockDate] = &types.Lockup{
				Amount: lockups.Amount,
			}
		}
	}
	ms.k.accountKeeper.SetAccount(ctx, ltsAcc)

	return &types.MsgExtendResponse{}, nil
}
