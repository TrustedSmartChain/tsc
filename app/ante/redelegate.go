package ante

import (
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	lockupkeeper "github.com/TrustedSmartChain/tsc/v2/x/lockup/keeper"
)

// RedelegationMarkerDecorator annotates the context for every MsgBeginRedelegate
// in the transaction so that the lockup BeforeDelegationRemoved hook can
// distinguish a move-delegation from a plain undelegation and skip the invariant
// check for the source delegation (which is being re-delegated, not withdrawn).
type RedelegationMarkerDecorator struct{}

func NewRedelegationMarkerDecorator() RedelegationMarkerDecorator {
	return RedelegationMarkerDecorator{}
}

func (d RedelegationMarkerDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	for _, msg := range tx.GetMsgs() {
		redelegate, ok := msg.(*stakingtypes.MsgBeginRedelegate)
		if !ok {
			continue
		}
		delAddr, err := sdk.AccAddressFromBech32(redelegate.DelegatorAddress)
		if err != nil {
			return ctx, err
		}
		valSrcAddr, err := sdk.ValAddressFromBech32(redelegate.ValidatorSrcAddress)
		if err != nil {
			return ctx, err
		}
		ctx = ctx.WithContext(lockupkeeper.WithRedelegating(ctx, delAddr, valSrcAddr))
	}
	return next(ctx, tx, simulate)
}
