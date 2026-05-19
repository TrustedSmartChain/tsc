package ante

import (
	"strings"
	"testing"

	"cosmossdk.io/math"
	protov2 "google.golang.org/protobuf/proto"

	"github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/authz"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	stakingtypes "github.com/cosmos/cosmos-sdk/x/staking/types"

	"github.com/TrustedSmartChain/tsc/v2/app/hooks"
)

// mockTx is a minimal sdk.Tx that only carries a message slice — the decorator
// never touches GetMsgsV2, so we leave it as a stub.
type mockTx struct {
	msgs []sdk.Msg
}

func (m mockTx) GetMsgs() []sdk.Msg                { return m.msgs }
func (m mockTx) GetMsgsV2() ([]protov2.Message, error) { return nil, nil }

func noopNext(ctx sdk.Context, _ sdk.Tx, _ bool) (sdk.Context, error) { return ctx, nil }

func mustExec(t *testing.T, inner ...sdk.Msg) *authz.MsgExec {
	t.Helper()
	anys := make([]*types.Any, len(inner))
	for i, msg := range inner {
		any, err := types.NewAnyWithValue(msg)
		if err != nil {
			t.Fatalf("pack any: %v", err)
		}
		anys[i] = any
	}
	return &authz.MsgExec{Grantee: "cosmos1grantee", Msgs: anys}
}

func intPtr(i math.Int) *math.Int { return &i }

func TestMinSelfDelegationDecorator(t *testing.T) {
	floor := hooks.MinSelfDelegation
	below := floor.Sub(math.OneInt())
	above := floor.Add(math.OneInt())

	tests := []struct {
		name    string
		msgs    []sdk.Msg
		wantErr string // substring; empty means expect success
	}{
		{
			name: "empty tx passes",
			msgs: nil,
		},
		{
			name: "unrelated msg passes",
			msgs: []sdk.Msg{&banktypes.MsgSend{}},
		},
		{
			name: "create at floor passes",
			msgs: []sdk.Msg{&stakingtypes.MsgCreateValidator{MinSelfDelegation: floor}},
		},
		{
			name: "create above floor passes",
			msgs: []sdk.Msg{&stakingtypes.MsgCreateValidator{MinSelfDelegation: above}},
		},
		{
			name:    "create below floor rejected",
			msgs:    []sdk.Msg{&stakingtypes.MsgCreateValidator{MinSelfDelegation: below}},
			wantErr: "below the required minimum",
		},
		{
			name: "edit with nil MinSelfDelegation passes",
			msgs: []sdk.Msg{&stakingtypes.MsgEditValidator{MinSelfDelegation: nil}},
		},
		{
			name: "edit at floor passes",
			msgs: []sdk.Msg{&stakingtypes.MsgEditValidator{MinSelfDelegation: intPtr(floor)}},
		},
		{
			name:    "edit below floor rejected",
			msgs:    []sdk.Msg{&stakingtypes.MsgEditValidator{MinSelfDelegation: intPtr(below)}},
			wantErr: "below the required minimum",
		},
		{
			name: "authz wrapping good create passes",
			msgs: []sdk.Msg{mustExec(t, &stakingtypes.MsgCreateValidator{MinSelfDelegation: floor})},
		},
		{
			name: "authz wrapping bad create rejected",
			msgs: []sdk.Msg{mustExec(t, &stakingtypes.MsgCreateValidator{MinSelfDelegation: below})},
			wantErr: "below the required minimum",
		},
		{
			name: "authz wrapping bad edit rejected",
			msgs: []sdk.Msg{mustExec(t, &stakingtypes.MsgEditValidator{MinSelfDelegation: intPtr(below)})},
			wantErr: "below the required minimum",
		},
		{
			name: "nested authz wrapping bad create rejected",
			msgs: []sdk.Msg{
				mustExec(t,
					mustExec(t,
						&stakingtypes.MsgCreateValidator{MinSelfDelegation: below},
					),
				),
			},
			wantErr: "below the required minimum",
		},
		{
			name: "deep nesting beyond cap rejected",
			msgs: []sdk.Msg{deeplyNestedExec(t, maxNestedMsgs+1, &stakingtypes.MsgCreateValidator{MinSelfDelegation: floor})},
			wantErr: "more nested msgs than permitted",
		},
	}

	d := NewMinSelfDelegationDecorator()
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := d.AnteHandle(sdk.Context{}, mockTx{msgs: tc.msgs}, false, noopNext)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// deeplyNestedExec wraps innerMsg in `depth` layers of authz.MsgExec.
func deeplyNestedExec(t *testing.T, depth int, innerMsg sdk.Msg) *authz.MsgExec {
	t.Helper()
	current := mustExec(t, innerMsg)
	for i := 1; i < depth; i++ {
		current = mustExec(t, current)
	}
	return current
}
