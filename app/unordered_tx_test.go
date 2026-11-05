package app

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/math"

	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
)

// signUnorderedTx signs msgs as an unordered tx: unordered=true, the given
// timeout_timestamp, and the given sequence (0 for a valid unordered tx).
func (env *gaslessEnv) signUnorderedTx(priv cryptotypes.PrivKey, fee sdk.Coins, gas uint64, timeout time.Time, sequence uint64, msgs ...sdk.Msg) sdk.Tx {
	env.t.Helper()

	txConfig := env.app.TxConfig()
	b := txConfig.NewTxBuilder()
	require.NoError(env.t, b.SetMsgs(msgs...))
	b.SetFeeAmount(fee)
	b.SetGasLimit(gas)
	b.SetUnordered(true)
	b.SetTimeoutTimestamp(timeout)

	addr := sdk.AccAddress(priv.PubKey().Address())
	acc := env.app.AccountKeeper.GetAccount(env.ctx, addr)
	require.NotNil(env.t, acc)

	require.NoError(env.t, b.SetSignatures(signingtypes.SignatureV2{
		PubKey:   priv.PubKey(),
		Data:     &signingtypes.SingleSignatureData{SignMode: signingtypes.SignMode_SIGN_MODE_DIRECT},
		Sequence: sequence,
	}))
	sigV2, err := clienttx.SignWithPrivKey(
		env.ctx,
		signingtypes.SignMode_SIGN_MODE_DIRECT,
		authsigning.SignerData{
			ChainID:       gaslessTestChainID,
			AccountNumber: acc.GetAccountNumber(),
			Sequence:      sequence,
			PubKey:        priv.PubKey(),
			Address:       addr.String(),
		},
		b, priv, txConfig, sequence,
	)
	require.NoError(env.t, err)
	require.NoError(env.t, b.SetSignatures(sigV2))
	return b.GetTx()
}

// paidSend is a funded 1 aTSC bank send from the operator, paying the fee
// floor for gas.
func (env *gaslessEnv) paidSend(to sdk.AccAddress) (sdk.Msg, sdk.Coins, uint64) {
	const gas = 200_000
	fee := sdk.NewCoins(sdk.NewCoin(BaseDenom, env.baseFee.MulInt64(gas).TruncateInt()))
	msg := &banktypes.MsgSend{
		FromAddress: env.operator.String(),
		ToAddress:   to.String(),
		Amount:      sdk.NewCoins(sdk.NewCoin(BaseDenom, math.OneInt())),
	}
	return msg, fee, gas
}

func TestUnorderedTransactionsEnabled(t *testing.T) {
	env := setupGaslessEnv(t)
	require.True(t, env.app.AccountKeeper.UnorderedTransactionsEnabled())
}

// An unordered tx is admitted without touching the account sequence, a
// replay of it is rejected, and another one with a different timeout from
// the same account is admitted.
func TestUnorderedTxAdmittedOnceWithoutSequence(t *testing.T) {
	env := setupGaslessEnv(t)
	msg, fee, gas := env.paidSend(env.key)
	seqBefore := env.app.AccountKeeper.GetAccount(env.ctx, env.operator).GetSequence()
	timeout := env.ctx.BlockTime().Add(5 * time.Minute)

	tx := env.signUnorderedTx(env.operatorPriv, fee, gas, timeout, 0, msg)
	require.NoError(t, env.runAnte(tx))
	require.Equal(t, seqBefore, env.app.AccountKeeper.GetAccount(env.ctx, env.operator).GetSequence(),
		"an unordered tx must not advance the sequence")

	has, err := env.app.AccountKeeper.ContainsUnorderedNonce(env.ctx, env.operator, timeout)
	require.NoError(t, err)
	require.True(t, has)

	require.ErrorContains(t, env.runAnte(tx), "already",
		"a replayed unordered tx must be rejected")

	// The timeout is the nonce: a nanosecond apart is a different tx.
	other := env.signUnorderedTx(env.operatorPriv, fee, gas, timeout.Add(time.Nanosecond), 0, msg)
	require.NoError(t, env.runAnte(other))

	// Ordered txs from the same account keep working alongside.
	require.NoError(t, env.runAnte(env.signTx(env.operatorPriv, fee, gas, nil, msg)))
	require.Equal(t, seqBefore+1, env.app.AccountKeeper.GetAccount(env.ctx, env.operator).GetSequence())
}

func TestUnorderedTxRejectsInvalidTimeoutOrSequence(t *testing.T) {
	env := setupGaslessEnv(t)
	msg, fee, gas := env.paidSend(env.key)
	now := env.ctx.BlockTime()

	cases := []struct {
		name     string
		timeout  time.Time
		sequence uint64
		wantErr  string
	}{
		{name: "no timeout", timeout: time.Time{}, wantErr: "must have timeout_timestamp set"},
		{name: "timeout in the past", timeout: now.Add(-time.Second), wantErr: "tx timeout"},
		{name: "timeout beyond the 10 minute max", timeout: now.Add(10*time.Minute + time.Second), wantErr: "ttl exceeds 10m0s"},
		{name: "sequence set", timeout: now.Add(time.Minute), sequence: 1, wantErr: "sequence is not allowed for unordered transactions"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := env.runAnte(env.signUnorderedTx(env.operatorPriv, fee, gas, tc.timeout, tc.sequence, msg))
			require.ErrorContains(t, err, tc.wantErr)
		})
	}
}

// x/auth's PreBlocker prunes nonces whose timeout has passed.
func TestUnorderedNoncesPrunedAfterTimeout(t *testing.T) {
	env := setupGaslessEnv(t)
	msg, fee, gas := env.paidSend(env.key)
	timeout := env.ctx.BlockTime().Add(time.Minute)
	require.NoError(t, env.runAnte(env.signUnorderedTx(env.operatorPriv, fee, gas, timeout, 0, msg)))

	// Still within the TTL: kept.
	_, err := env.app.ModuleManager.PreBlock(env.ctx.WithBlockTime(timeout.Add(-time.Second)))
	require.NoError(t, err)
	has, err := env.app.AccountKeeper.ContainsUnorderedNonce(env.ctx, env.operator, timeout)
	require.NoError(t, err)
	require.True(t, has)

	// Past it: pruned.
	_, err = env.app.ModuleManager.PreBlock(env.ctx.WithBlockTime(timeout.Add(time.Second)))
	require.NoError(t, err)
	has, err = env.app.AccountKeeper.ContainsUnorderedNonce(env.ctx, env.operator, timeout)
	require.NoError(t, err)
	require.False(t, has)
}

// Gasless txs can be unordered too: the zero-fee path and its admission
// quota still apply, and a replay is still rejected.
func TestUnorderedGaslessTx(t *testing.T) {
	env := setupGaslessEnv(t)
	before := env.statusCounter(env.node)
	timeout := env.ctx.BlockTime().Add(5 * time.Minute)

	tx := env.signUnorderedTx(env.nodePriv, nil, 200_000, timeout, 0, env.statusMsg(env.node))
	require.NoError(t, env.runAnte(tx))
	require.Equal(t, before+1, env.statusCounter(env.node))

	require.Error(t, env.runAnte(tx))
	require.Equal(t, before+1, env.statusCounter(env.node), "a rejected replay must not burn quota")
}
