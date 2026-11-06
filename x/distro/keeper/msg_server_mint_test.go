package keeper_test

import (
	"testing"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v4/x/distro/types"
)

// setupMint configures addrs[0] as minter and addrs[1] as receiver, and sets
// the block time two years into the default schedule (start 2025-07-22).
func setupMint(t *testing.T) *testFixture {
	t.Helper()
	f := SetupTest(t)
	params := types.DefaultParams()
	params.MintingAddress = f.addrs[0].String()
	params.ReceivingAddress = f.addrs[1].String()
	require.NoError(t, f.k.Params.Set(f.ctx, params))
	f.ctx = f.ctx.WithBlockTime(time.Date(2027, 7, 22, 12, 0, 0, 0, time.UTC))
	return f
}

func (f *testFixture) supply() math.Int {
	return f.bankkeeper.GetSupply(f.ctx, types.DefaultDenom).Amount
}

func tsc(n int64) math.Int { return math.NewIntWithDecimal(n, 18) }

func TestMintPaysReceiver(t *testing.T) {
	f := setupMint(t)
	_, err := f.msgServer.Mint(f.ctx, &types.MsgMint{Minter: f.addrs[0].String(), Amount: tsc(1_000).String()})
	require.NoError(t, err)
	require.True(t, tsc(1_000).Equal(f.bankkeeper.GetBalance(f.ctx, f.addrs[1], types.DefaultDenom).Amount))
	require.True(t, f.bankkeeper.GetBalance(f.ctx, f.addrs[0], types.DefaultDenom).Amount.IsZero(), "the minter is not paid")
}

func TestMintRejectsUnauthorized(t *testing.T) {
	f := setupMint(t)
	_, err := f.msgServer.Mint(f.ctx, &types.MsgMint{Minter: f.addrs[2].String(), Amount: tsc(1).String()})
	require.ErrorContains(t, err, "unauthorized")
}

// Two years into the first 48-month period, at most half of its 10.5M TSC
// allocation is distributable.
func TestMintRespectsSchedule(t *testing.T) {
	f := setupMint(t)
	_, err := f.msgServer.Mint(f.ctx, &types.MsgMint{Minter: f.addrs[0].String(), Amount: tsc(5_300_000).String()})
	require.ErrorContains(t, err, "distributable limit")
	_, err = f.msgServer.Mint(f.ctx, &types.MsgMint{Minter: f.addrs[0].String(), Amount: tsc(5_000_000).String()})
	require.NoError(t, err)
}

// Total supply of the denom, from any source, can never be pushed past the
// fixed 21,000,000 TSC cap by MsgMint.
func TestMintRespectsFixedMaxSupply(t *testing.T) {
	f := setupMint(t)
	// Supply that did not come from x/distro (e.g. genesis), up to 1 aTSC
	// below the cap.
	preexisting := sdk.NewCoins(sdk.NewCoin(types.DefaultDenom, types.MaxSupply().SubRaw(1)))
	require.NoError(t, f.bankkeeper.MintCoins(f.ctx, types.ModuleName, preexisting))
	require.True(t, types.MaxSupply().SubRaw(1).Equal(f.supply()))

	_, err := f.msgServer.Mint(f.ctx, &types.MsgMint{Minter: f.addrs[0].String(), Amount: "2"})
	require.ErrorContains(t, err, "max supply exceeded")
	require.True(t, types.MaxSupply().SubRaw(1).Equal(f.supply()), "a rejected mint must not change supply")
}
