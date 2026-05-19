// Package hooks contains app-level hook implementations.
//
// MinSelfDelegationHook enforces a chain-wide minimum self-delegation for
// validators. The same floor is applied at three layers so it cannot be
// bypassed by paths that skip the tx ante chain (EVM staking precompile, ICA
// host, gov proposal execution, CosmWasm stargate msgs, genesis import):
//
//   - The ante decorator in app/ante rejects MsgCreateValidator and
//     MsgEditValidator early when delivered as Cosmos SDK txs.
//   - MinSelfDelegationHook implements stakingtypes.StakingHooks and rejects
//     in AfterValidatorCreated, which fires from the staking keeper regardless
//     of caller (msg server, precompile, ICA, wasm, gov).
//   - ValidateStakingGenesis is invoked from InitChainer to reject genesis
//     files that would seed validators below the floor (hooks do not fire at
//     genesis).
package hooks

import (
	"context"
	"fmt"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"
	errortypes "github.com/cosmos/cosmos-sdk/types/errors"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"
)

// MinSelfDelegation is the minimum self-delegation (in aTSC) a validator must
// commit to. 500 TSC = 500 * 10^18 aTSC.
var MinSelfDelegation = math.NewIntFromUint64(500).MulRaw(1_000_000_000_000_000_000)

// StakingKeeper is the subset of the staking keeper that
// MinSelfDelegationHook needs to read the post-create validator state.
type StakingKeeper interface {
	GetValidator(ctx context.Context, addr sdk.ValAddress) (stakingtypes.Validator, error)
}

// MinSelfDelegationHook is a stakingtypes.StakingHooks implementation that
// enforces the min-self-delegation floor at the keeper level. All hooks other
// than AfterValidatorCreated are no-ops.
type MinSelfDelegationHook struct {
	sk StakingKeeper
}

func NewMinSelfDelegationHook(sk StakingKeeper) MinSelfDelegationHook {
	return MinSelfDelegationHook{sk: sk}
}

var _ stakingtypes.StakingHooks = MinSelfDelegationHook{}

// AfterValidatorCreated reads the freshly written validator and rejects if its
// MinSelfDelegation is below the chain floor. Returning an error rolls back
// the CreateValidator msg, so any caller path (EVM precompile, ICA, wasm,
// gov, etc.) gets the same enforcement.
func (h MinSelfDelegationHook) AfterValidatorCreated(ctx context.Context, valAddr sdk.ValAddress) error {
	validator, err := h.sk.GetValidator(ctx, valAddr)
	if err != nil {
		return err
	}
	if validator.MinSelfDelegation.LT(MinSelfDelegation) {
		return errorsmod.Wrap(
			errortypes.ErrInvalidRequest,
			fmt.Sprintf("min_self_delegation %s is below the required minimum of %s aTSC",
				validator.MinSelfDelegation, MinSelfDelegation),
		)
	}
	return nil
}

func (h MinSelfDelegationHook) BeforeValidatorModified(_ context.Context, _ sdk.ValAddress) error {
	return nil
}
func (h MinSelfDelegationHook) AfterValidatorRemoved(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}
func (h MinSelfDelegationHook) AfterValidatorBonded(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}
func (h MinSelfDelegationHook) AfterValidatorBeginUnbonding(_ context.Context, _ sdk.ConsAddress, _ sdk.ValAddress) error {
	return nil
}
func (h MinSelfDelegationHook) BeforeDelegationCreated(_ context.Context, _ sdk.AccAddress, _ sdk.ValAddress) error {
	return nil
}
func (h MinSelfDelegationHook) BeforeDelegationSharesModified(_ context.Context, _ sdk.AccAddress, _ sdk.ValAddress) error {
	return nil
}
func (h MinSelfDelegationHook) BeforeDelegationRemoved(_ context.Context, _ sdk.AccAddress, _ sdk.ValAddress) error {
	return nil
}
func (h MinSelfDelegationHook) AfterDelegationModified(_ context.Context, _ sdk.AccAddress, _ sdk.ValAddress) error {
	return nil
}
func (h MinSelfDelegationHook) BeforeValidatorSlashed(_ context.Context, _ sdk.ValAddress, _ math.LegacyDec) error {
	return nil
}
func (h MinSelfDelegationHook) AfterUnbondingInitiated(_ context.Context, _ uint64) error {
	return nil
}

// ValidateStakingGenesis rejects any staking genesis state that seeds a
// validator below the floor. Hooks do not fire on genesis import (the staking
// module calls SetValidator directly, not the msg server), so this check is
// the only thing standing between a misconfigured genesis.json and a
// non-compliant validator set at block 1.
func ValidateStakingGenesis(gs *stakingtypes.GenesisState) error {
	for _, v := range gs.Validators {
		if v.MinSelfDelegation.LT(MinSelfDelegation) {
			return fmt.Errorf(
				"genesis validator %s has min_self_delegation %s, below the required minimum of %s aTSC",
				v.OperatorAddress, v.MinSelfDelegation, MinSelfDelegation,
			)
		}
	}
	return nil
}
