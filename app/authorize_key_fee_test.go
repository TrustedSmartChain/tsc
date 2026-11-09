package app

import (
	"testing"

	"github.com/stretchr/testify/require"

	"cosmossdk.io/x/feegrant"

	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"

	networktypes "github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// signFeeGrantedTx signs msgs as an ordered tx whose fee is paid by granter.
func (env *gaslessEnv) signFeeGrantedTx(priv cryptotypes.PrivKey, granter sdk.AccAddress, fee sdk.Coins, gas uint64, msgs ...sdk.Msg) sdk.Tx {
	env.t.Helper()

	txConfig := env.app.TxConfig()
	b := txConfig.NewTxBuilder()
	require.NoError(env.t, b.SetMsgs(msgs...))
	b.SetFeeAmount(fee)
	b.SetGasLimit(gas)
	b.SetFeeGranter(granter)

	addr := sdk.AccAddress(priv.PubKey().Address())
	acc := env.app.AccountKeeper.GetAccount(env.ctx, addr)
	require.NotNil(env.t, acc)

	require.NoError(env.t, b.SetSignatures(signingtypes.SignatureV2{
		PubKey:   priv.PubKey(),
		Data:     &signingtypes.SingleSignatureData{SignMode: signingtypes.SignMode_SIGN_MODE_DIRECT},
		Sequence: acc.GetSequence(),
	}))
	sigV2, err := clienttx.SignWithPrivKey(
		env.ctx,
		signingtypes.SignMode_SIGN_MODE_DIRECT,
		authsigning.SignerData{
			ChainID:       gaslessTestChainID,
			AccountNumber: acc.GetAccountNumber(),
			Sequence:      acc.GetSequence(),
			PubKey:        priv.PubKey(),
			Address:       addr.String(),
		},
		b, priv, txConfig, acc.GetSequence(),
	)
	require.NoError(env.t, err)
	require.NoError(env.t, b.SetSignatures(sigV2))
	return b.GetTx()
}

func (env *gaslessEnv) authorizeKeyMsg(operator sdk.AccAddress) *networktypes.MsgAuthorizeActivationKey {
	_, key := env.newSigner()
	return &networktypes.MsgAuthorizeActivationKey{Operator: operator.String(), ActivationAddress: key.String()}
}

// MsgAuthorizeActivationKey always pays a fee (nodelabs v0.3.0): it
// is off the gasless allowlist, so even a licensed operator's zero-fee
// authorization is refused, and a paid one is admitted.
func TestAuthorizeActivationKeyRequiresFee(t *testing.T) {
	env := setupGaslessEnv(t)
	const gas = 200_000
	fee := sdk.NewCoins(sdk.NewCoin(BaseDenom, env.baseFee.MulInt64(gas).TruncateInt()))

	require.NotContains(t, networktypes.GaslessMessages(), sdk.MsgTypeURL(&networktypes.MsgAuthorizeActivationKey{}))

	// The env's operator holds an active trust license: under v0.3.0-alpha.1
	// this was the free path.
	err := env.runAnte(env.signTx(env.operatorPriv, nil, gas, nil, env.authorizeKeyMsg(env.operator)))
	require.ErrorContains(t, err, "insufficient fee")

	balanceBefore := env.balance(env.operator)
	require.NoError(t, env.runAnte(env.signTx(env.operatorPriv, fee, gas, nil, env.authorizeKeyMsg(env.operator))))
	require.Equal(t, balanceBefore.Sub(fee.AmountOf(BaseDenom)), env.balance(env.operator))
}

// The onboarding flow: a new, unlicensed wallet with no funds authorizes a key
// with the fee paid through a grant restricted to MsgAuthorizeActivationKey.
func TestAuthorizeActivationKeyFeeGranted(t *testing.T) {
	env := setupGaslessEnv(t)
	const gas = 200_000
	fee := sdk.NewCoins(sdk.NewCoin(BaseDenom, env.baseFee.MulInt64(gas).TruncateInt()))

	customerPriv, customer := env.newSigner()
	allowance, err := feegrant.NewAllowedMsgAllowance(
		&feegrant.BasicAllowance{SpendLimit: sdk.NewCoins(sdk.NewCoin(BaseDenom, fee.AmountOf(BaseDenom).MulRaw(10)))},
		[]string{sdk.MsgTypeURL(&networktypes.MsgAuthorizeActivationKey{})},
	)
	require.NoError(t, err)
	require.NoError(t, env.app.FeeGrantKeeper.GrantAllowance(env.ctx, env.operator, customer, allowance))

	granterBefore := env.balance(env.operator)
	require.NoError(t, env.runAnte(env.signFeeGrantedTx(customerPriv, env.operator, fee, gas, env.authorizeKeyMsg(customer))))
	require.True(t, env.balance(customer).IsZero(), "the customer pays nothing")
	require.Equal(t, granterBefore.Sub(fee.AmountOf(BaseDenom)), env.balance(env.operator))

	// Zero fee without the grant is still refused.
	err = env.runAnte(env.signTx(customerPriv, nil, gas, nil, env.authorizeKeyMsg(customer)))
	require.ErrorContains(t, err, "insufficient fee")
}
