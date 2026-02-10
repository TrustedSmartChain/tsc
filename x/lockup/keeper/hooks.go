package keeper

import (
	"context"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/TrustedSmartChain/tsc/x/lockup/types"
)

// Hooks wraps the lockup keeper to implement the staking StakingHooks interface.
// This allows the lockup module to reject undelegations that would cause the
// total delegated amount to drop below the total locked amount — regardless of
// whether the undelegation originates from a Cosmos SDK message or an EVM
// staking precompile call.
type Hooks struct {
	k Keeper
}

var _ stakingtypes.StakingHooks = Hooks{}

// Hooks returns the lockup module's staking hooks.
func (k Keeper) Hooks() Hooks {
	return Hooks{k}
}

// --------------------------------------------------------------------------
// Delegation hooks — enforce the lockup invariant
// --------------------------------------------------------------------------

// BeforeDelegationCreated is called when a new delegation is about to be created.
func (h Hooks) BeforeDelegationCreated(_ context.Context, _ sdk.AccAddress, _ sdk.ValAddress) error {
	return nil
}

// BeforeDelegationSharesModified is called when an existing delegation's shares
// are about to be modified (e.g. during Unbond). At this point the delegation
// still holds its original shares, so we cannot yet verify the post-modification
// invariant. The check is deferred to AfterDelegationModified /
// BeforeDelegationRemoved.
func (h Hooks) BeforeDelegationSharesModified(_ context.Context, _ sdk.AccAddress, _ sdk.ValAddress) error {
	return nil
}

// AfterDelegationModified fires after a delegation's shares have been changed
// but the delegation record still exists (partial undelegation or re-delegation).
// We re-check the lockup invariant: totalDelegated >= totalLocked.
func (h Hooks) AfterDelegationModified(ctx context.Context, delAddr sdk.AccAddress, _ sdk.ValAddress) error {
	return h.checkLockupInvariant(ctx, delAddr)
}

// BeforeDelegationRemoved fires when a delegation is about to be fully removed
// (shares reached zero). The delegation record still exists in the KV store at
// this point with its ORIGINAL shares (the SDK does not call SetDelegation
// before RemoveDelegation). We must subtract this delegation's value from the
// total before checking the invariant.
func (h Hooks) BeforeDelegationRemoved(ctx context.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) error {
	return h.checkLockupInvariantExcluding(ctx, delAddr, valAddr)
}

// checkLockupInvariant verifies that the delegator's total delegated amount is
// still >= their total locked amount. Returns an error if the invariant is
// violated, which causes the staking keeper to abort the operation.
func (h Hooks) checkLockupInvariant(ctx context.Context, delAddr sdk.AccAddress) error {
	return h.checkLockupInvariantExcluding(ctx, delAddr, nil)
}

// checkLockupInvariantExcluding is the core invariant check. If excludeVal is
// non-nil, the delegation to that validator is excluded from the total (used by
// BeforeDelegationRemoved where the KV store still has the old record).
func (h Hooks) checkLockupInvariantExcluding(ctx context.Context, delAddr sdk.AccAddress, excludeVal sdk.ValAddress) error {
	sdkCtx := sdk.UnwrapSDKContext(ctx)

	totalLocked, err := h.k.GetLockedAmountByAddress(sdkCtx, delAddr)
	if err != nil {
		return err
	}

	// No locks — nothing to enforce.
	if totalLocked.IsZero() {
		return nil
	}

	totalDelegated, err := h.k.GetTotalDelegatedAmount(sdkCtx, delAddr)
	if err != nil {
		return err
	}

	// If we're inside BeforeDelegationRemoved, the delegation being removed
	// is still in the KV store with its original shares. Subtract it.
	if excludeVal != nil {
		excludeAmount, err := h.k.GetDelegationAmount(sdkCtx, delAddr, excludeVal)
		if err != nil {
			return err
		}
		adjusted := totalDelegated.Sub(excludeAmount)
		totalDelegated = &adjusted
	}

	if totalDelegated.LT(*totalLocked) {
		return types.ErrInsufficientDelegations.Wrapf(
			"undelegation would cause delegated amount to drop below locked amount: delegated %s < locked %s",
			totalDelegated.String(),
			totalLocked.String(),
		)
	}

	return nil
}

// --------------------------------------------------------------------------
// Validator hooks — no-ops for the lockup module
// --------------------------------------------------------------------------

func (h Hooks) AfterValidatorCreated(_ context.Context, _ sdk.ValAddress) error {
	return nil
}

func (h Hooks) BeforeValidatorModified(_ context.Context, _ sdk.ValAddress) error {
	return nil
}

func (h Hooks) AfterValidatorRemoved(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

func (h Hooks) AfterValidatorBonded(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

func (h Hooks) AfterValidatorBeginUnbonding(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}

func (h Hooks) BeforeValidatorSlashed(_ context.Context, _ sdk.ValAddress, _ math.LegacyDec) error {
	return nil
}

func (h Hooks) AfterUnbondingInitiated(_ context.Context, _ uint64) error {
	return nil
}
