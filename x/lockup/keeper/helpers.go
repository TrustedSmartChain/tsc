package keeper

import (
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

func (k Keeper) GetTotalDelegatedAmount(ctx sdk.Context, addr sdk.AccAddress) (*math.Int, error) {
	totalDelegatedAmount := math.ZeroInt()

	more := true
	for more {

		delegations, err := k.stakingKeeper.GetDelegatorDelegations(ctx, addr, 100)
		if err != nil {
			return nil, err
		}

		for _, delegation := range delegations {
			valAddr, err := sdk.ValAddressFromBech32(delegation.GetValidatorAddr())
			if err != nil {
				return nil, sdkerrors.ErrInvalidAddress.Wrapf("invalid validator address: %s", err)
			}
			validator, err := k.stakingKeeper.GetValidator(ctx, valAddr)
			if err != nil {
				return nil, err
			}
			tokens := validator.TokensFromShares(delegation.GetShares())
			totalDelegatedAmount = totalDelegatedAmount.Add(tokens.Ceil().TruncateInt())
		}
		more = len(delegations) == 100

	}

	return &totalDelegatedAmount, nil
}

// GetDelegationAmount returns the token amount of a single delegation.
// Used by BeforeDelegationRemoved to subtract the delegation being removed
// (which is still in the KV store with original shares at that point).
func (k Keeper) GetDelegationAmount(ctx sdk.Context, delAddr sdk.AccAddress, valAddr sdk.ValAddress) (math.Int, error) {
	delegation, err := k.stakingKeeper.GetDelegation(ctx, delAddr, valAddr)
	if err != nil {
		// Delegation not found — nothing to subtract.
		return math.ZeroInt(), nil
	}

	validator, err := k.stakingKeeper.GetValidator(ctx, valAddr)
	if err != nil {
		return math.ZeroInt(), err
	}

	tokens := validator.TokensFromShares(delegation.GetShares())
	return tokens.Ceil().TruncateInt(), nil
}
