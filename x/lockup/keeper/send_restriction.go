package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// SendRestrictionFn is a bank SendRestrictionFn that prevents users from
// transferring tokens that are locked.  It is registered on the bank keeper
// via AppendSendRestriction so it applies to **every** SendCoins call,
// regardless of whether the transfer originates from a Cosmos SDK message,
// an EVM value‑transfer (MsgEthereumTx), IBC, or any other path.
//
// The check mirrors the logic in the ante handler: the "unavailable" amount
// is max(0, totalLocked − totalDelegated).  Any bond‑denom coins in the
// transfer must fit within the sender's free (non‑locked) bank balance.
func (k Keeper) SendRestrictionFn(ctx context.Context, fromAddr, toAddr sdk.AccAddress, amt sdk.Coins) (sdk.AccAddress, error) {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	bondDenom, err := k.stakingKeeper.BondDenom(sdkCtx)
	if err != nil {
		// If we cannot determine the bond denom we cannot enforce
		// lockup rules.  Fail‑open would be dangerous, so fail‑closed.
		return toAddr, err
	}

	// Only check if the transfer contains the bond denom.
	sendAmount := amt.AmountOf(bondDenom)
	if sendAmount.IsZero() {
		return toAddr, nil
	}

	// Determine how much of the sender's locked amount exceeds their
	// delegated amount (i.e. the portion that must remain in the bank
	// balance).
	totalLocked, err := k.GetLockedAmountByAddress(sdkCtx, fromAddr)
	if err != nil {
		return toAddr, err
	}
	if totalLocked.IsZero() {
		return toAddr, nil
	}

	totalDelegated, err := k.GetTotalDelegatedAmount(sdkCtx, fromAddr)
	if err != nil {
		return toAddr, err
	}

	lockedAboveDelegated := totalLocked.Sub(*totalDelegated)
	if lockedAboveDelegated.IsNegative() {
		lockedAboveDelegated = math.ZeroInt()
	}

	if lockedAboveDelegated.IsZero() {
		return toAddr, nil
	}

	// Calculate available = bankBalance − lockedAboveDelegated.
	bankBalance := k.bankKeeper.GetBalance(sdkCtx, fromAddr, bondDenom).Amount
	available := bankBalance.Sub(lockedAboveDelegated)
	if available.IsNegative() {
		available = math.ZeroInt()
	}

	if available.LT(sendAmount) {
		return toAddr, sdkerrors.ErrInsufficientFunds.Wrapf(
			"insufficient unlocked balance: available %s, required %s (locked: %s, delegated: %s)",
			available, sendAmount, totalLocked, totalDelegated,
		)
	}

	return toAddr, nil
}
