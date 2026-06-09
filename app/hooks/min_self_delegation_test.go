package hooks_test

import (
	"context"
	"strings"
	"testing"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/TrustedSmartChain/tsc/v3/app/hooks"
)

// stubKeeper returns the configured validator for any address. The hook only
// needs GetValidator.
type stubKeeper struct {
	validator stakingtypes.Validator
	err       error
}

func (s stubKeeper) GetValidator(_ context.Context, _ sdk.ValAddress) (stakingtypes.Validator, error) {
	return s.validator, s.err
}

func TestMinSelfDelegationHookAfterValidatorCreated(t *testing.T) {
	floor := hooks.MinSelfDelegation
	below := floor.Sub(math.OneInt())
	above := floor.Add(math.OneInt())

	tests := []struct {
		name    string
		msd     math.Int
		wantErr string
	}{
		{name: "at floor passes", msd: floor},
		{name: "above floor passes", msd: above},
		{name: "below floor rejected", msd: below, wantErr: "below the required minimum"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sk := stubKeeper{validator: stakingtypes.Validator{MinSelfDelegation: tc.msd}}
			h := hooks.NewMinSelfDelegationHook(sk)

			err := h.AfterValidatorCreated(context.Background(), sdk.ValAddress{0x01})
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestValidateStakingGenesis(t *testing.T) {
	floor := hooks.MinSelfDelegation
	below := floor.Sub(math.OneInt())

	tests := []struct {
		name    string
		gs      *stakingtypes.GenesisState
		wantErr string
	}{
		{
			name: "no validators passes",
			gs:   &stakingtypes.GenesisState{},
		},
		{
			name: "all validators at or above floor passes",
			gs: &stakingtypes.GenesisState{Validators: []stakingtypes.Validator{
				{OperatorAddress: "valoper1a", MinSelfDelegation: floor},
				{OperatorAddress: "valoper1b", MinSelfDelegation: floor.MulRaw(2)},
			}},
		},
		{
			name: "one validator below floor rejected",
			gs: &stakingtypes.GenesisState{Validators: []stakingtypes.Validator{
				{OperatorAddress: "valoper1a", MinSelfDelegation: floor},
				{OperatorAddress: "valoper1b", MinSelfDelegation: below},
			}},
			wantErr: "valoper1b",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := hooks.ValidateStakingGenesis(tc.gs)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}
