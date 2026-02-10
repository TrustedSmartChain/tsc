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
