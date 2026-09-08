package ante

import (
	"fmt"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	"github.com/cosmos/cosmos-sdk/x/authz"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/TrustedSmartChain/tsc/v4/app/hooks"
)

// maxNestedMsgs caps recursion through authz.MsgExec wrappers, matching the
// limit used by cosmos-evm's AuthzLimiterDecorator.
const maxNestedMsgs = 7

// MinSelfDelegationDecorator rejects any MsgCreateValidator or MsgEditValidator
// whose MinSelfDelegation is below the chain-wide floor. For MsgEditValidator,
// the check is skipped when the field is not being updated (nil). Messages
// wrapped in authz.MsgExec are unpacked and checked recursively.
type MinSelfDelegationDecorator struct{}

func NewMinSelfDelegationDecorator() MinSelfDelegationDecorator {
	return MinSelfDelegationDecorator{}
}

func (d MinSelfDelegationDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	if err := d.checkMsgs(tx.GetMsgs(), 1); err != nil {
		return ctx, err
	}
	return next(ctx, tx, simulate)
}

func (d MinSelfDelegationDecorator) checkMsgs(msgs []sdk.Msg, nestedLvl int) error {
	if nestedLvl >= maxNestedMsgs {
		return errorsmod.Wrapf(
			errortypes.ErrInvalidRequest,
			"found more nested msgs than permitted; got: %d, expected: <%d",
			nestedLvl, maxNestedMsgs,
		)
	}
	for _, msg := range msgs {
		switch m := msg.(type) {
		case *authz.MsgExec:
			innerMsgs, err := m.GetMessages()
			if err != nil {
				return err
			}
			if err := d.checkMsgs(innerMsgs, nestedLvl+1); err != nil {
				return err
			}
		case *stakingtypes.MsgCreateValidator:
			if err := assertFloor(m.MinSelfDelegation); err != nil {
				return err
			}
		case *stakingtypes.MsgEditValidator:
			if m.MinSelfDelegation == nil {
				continue
			}
			if err := assertFloor(*m.MinSelfDelegation); err != nil {
				return err
			}
		}
	}
	return nil
}

func assertFloor(v math.Int) error {
	if v.LT(hooks.MinSelfDelegation) {
		return errorsmod.Wrap(
			errortypes.ErrInvalidRequest,
			fmt.Sprintf("min_self_delegation %s is below the required minimum of %s aTSC", v, hooks.MinSelfDelegation),
		)
	}
	return nil
}
