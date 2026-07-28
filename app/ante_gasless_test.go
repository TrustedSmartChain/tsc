package app

// Ante-level tests for the gasless tx machinery: the x/network primitives
// composed into the chain's cosmos ante chain (app/ante/cosmos.go). The
// ante handler is built from the app's real keepers exactly as
// setAnteHandler builds it, and txs are properly signed so the full
// decorator chain — including signature verification and fee deduction —
// runs. Fee-market floors are set non-zero throughout: the dev genesis's
// base_fee=0 masks fee-path bugs.
//
// Everything runs under one app instance in a single test function: the
// cosmos/evm chain config is a process-global that cannot be set twice.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	abci "github.com/cometbft/cometbft/abci/types"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
	"github.com/ethereum/go-ethereum/common"

	"cosmossdk.io/collections"
	"cosmossdk.io/math"
	upgradetypes "cosmossdk.io/x/upgrade/types"

	clienttx "github.com/cosmos/cosmos-sdk/client/tx"
	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	"github.com/cosmos/cosmos-sdk/crypto/keys/secp256k1"
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"
	authsigning "github.com/cosmos/cosmos-sdk/x/auth/signing"
	authtx "github.com/cosmos/cosmos-sdk/x/auth/tx"
	banktypes "github.com/cosmos/cosmos-sdk/x/bank/types"
	minttypes "github.com/cosmos/cosmos-sdk/x/mint/types"

	evmante "github.com/cosmos/evm/ante"
	antetypes "github.com/cosmos/evm/ante/types"

	licensetypes "github.com/webstack-sdk/webstack/x/license/types"
	networkante "github.com/webstack-sdk/webstack/x/network/ante"
	networktypes "github.com/webstack-sdk/webstack/x/network/types"

	chainante "github.com/TrustedSmartChain/tsc/v3/app/ante"
	attestationtypes "github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

const gaslessTestChainID = "chain-test"

var gaslessTestBlockTime = time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

type gaslessEnv struct {
	t   *testing.T
	app *ChainApp
	ctx sdk.Context

	ante sdk.AnteHandler

	baseFee math.LegacyDec

	operatorPriv, keyPriv, nodePriv, node2Priv cryptotypes.PrivKey
	operator, key, node, node2                 sdk.AccAddress
}

func (env *gaslessEnv) newSigner() (cryptotypes.PrivKey, sdk.AccAddress) {
	env.t.Helper()
	priv := secp256k1.GenPrivKey()
	addr := sdk.AccAddress(priv.PubKey().Address())
	acc := env.app.AccountKeeper.NewAccountWithAddress(env.ctx, addr)
	env.app.AccountKeeper.SetAccount(env.ctx, acc)
	return priv, addr
}

// seedLicense writes one active license of typeID for holder straight into
// license state.
func (env *gaslessEnv) seedLicense(holder, typeID string) {
	env.t.Helper()
	lk := env.app.LicenseKeeper
	require.NoError(env.t, lk.LicenseTypes.Set(env.ctx, typeID, licensetypes.LicenseType{
		Id:           typeID,
		MaxSupply:    math.ZeroInt(),
		IssuedCount:  math.OneInt(),
		ActiveCount:  math.OneInt(),
		RevokedCount: math.ZeroInt(),
	}))
	require.NoError(env.t, lk.LicenseCounts.Set(env.ctx, typeID, 1))
	require.NoError(env.t, lk.Licenses.Set(env.ctx, collections.Join(typeID, uint64(1)), licensetypes.License{
		Id: 1, Type: typeID, Holder: holder, StartDate: "2025-01-01", Status: licensetypes.StatusActive,
	}))
	require.NoError(env.t, lk.ActiveLicensesByHolder.Set(env.ctx, collections.Join3(holder, typeID, uint64(1))))
}

// seedNode writes an active trust node for the operator straight into
// network state.
func (env *gaslessEnv) seedNode(node sdk.AccAddress) {
	env.t.Helper()
	nk := env.app.NetworkKeeper
	now := env.ctx.BlockTime()
	require.NoError(env.t, nk.Nodes.Set(env.ctx, node.String(), networktypes.Node{
		Address: node.String(), Operator: env.operator.String(), ActivatedBy: env.key.String(),
		Type: attestationtypes.NodeTypeTrust, Status: networktypes.NodeActive, LastActiveTime: now,
	}))
	require.NoError(env.t, nk.OperatorNodes.Set(env.ctx, collections.Join(env.operator.String(), node.String())))
	require.NoError(env.t, nk.RecentNodeActivity.Set(env.ctx, collections.Join3(env.operator.String(), networktypes.DayEpoch(now), node.String())))
}

func setupGaslessEnv(t *testing.T) *gaslessEnv {
	t.Helper()

	chainApp := Setup(t, gaslessTestChainID, 9001)

	// Commit the genesis state so a context over the committed multistore
	// sees it (SetupWithGenesisValSet deliberately does not commit).
	_, err := chainApp.FinalizeBlock(&abci.RequestFinalizeBlock{
		Height: chainApp.LastBlockHeight() + 1,
		Time:   gaslessTestBlockTime.Add(-time.Minute),
	})
	require.NoError(t, err)
	_, err = chainApp.Commit()
	require.NoError(t, err)

	header := cmtproto.Header{
		Height:  chainApp.LastBlockHeight() + 1,
		ChainID: gaslessTestChainID,
		Time:    gaslessTestBlockTime,
	}
	env := &gaslessEnv{
		t:       t,
		app:     chainApp,
		ctx:     chainApp.NewUncachedContext(false, header),
		baseFee: math.LegacyNewDec(1_000_000_000),
	}

	// Non-zero fee floors so the paid path actually enforces fees.
	fmParams := chainApp.FeeMarketKeeper.GetParams(env.ctx)
	fmParams.NoBaseFee = false
	fmParams.BaseFee = env.baseFee
	fmParams.MinGasPrice = env.baseFee
	require.NoError(t, chainApp.FeeMarketKeeper.SetParams(env.ctx, fmParams))

	// Run the real v3 upgrade handler for the network/attestation param
	// seeding — not hand-seeded values. RunMigrations no-ops (the version
	// map is current), then the handler seeds namespace owners and params.
	// The handler enables the license precompile, which a fresh test genesis
	// already activates — deactivate it first to mimic the pre-v3 state.
	evmParams := chainApp.EVMKeeper.GetParams(env.ctx)
	withoutLicensePrecompile := make([]string, 0, len(evmParams.ActiveStaticPrecompiles))
	for _, addr := range evmParams.ActiveStaticPrecompiles {
		if addr != licensetypes.PrecompileAddress {
			withoutLicensePrecompile = append(withoutLicensePrecompile, addr)
		}
	}
	evmParams.ActiveStaticPrecompiles = withoutLicensePrecompile
	require.NoError(t, chainApp.EVMKeeper.SetParams(env.ctx, evmParams))
	require.NoError(t, chainApp.UpgradeKeeper.ApplyUpgrade(env.ctx, upgradetypes.Plan{
		Name:   UpgradeNameV3,
		Height: env.ctx.BlockHeight(),
	}))

	// One operator with one trust license, an active activation key, and two
	// active trust nodes (the second is quota fodder).
	env.operatorPriv, env.operator = env.newSigner()
	env.keyPriv, env.key = env.newSigner()
	env.nodePriv, env.node = env.newSigner()
	env.node2Priv, env.node2 = env.newSigner()

	env.seedLicense(env.operator.String(), attestationtypes.NodeTypeTrust)

	nk := chainApp.NetworkKeeper
	require.NoError(t, nk.ActivationKeys.Set(env.ctx, env.key.String(), networktypes.ActivationKey{
		Address: env.key.String(), Operator: env.operator.String(), CreatedAt: env.ctx.BlockTime(), Status: networktypes.KeyActive,
	}))
	require.NoError(t, nk.OperatorActivationKeys.Set(env.ctx, collections.Join(env.operator.String(), env.key.String())))
	env.seedNode(env.node)
	env.seedNode(env.node2)
	require.NoError(t, nk.OperatorNodeCounts.Set(env.ctx, env.operator.String(), networktypes.OperatorNodeCounts{Total: 2, Active: 2}))
	require.NoError(t, nk.Operators.Set(env.ctx, env.operator.String()))

	// Fund the operator for the paid-path cases.
	fund := sdk.NewCoins(sdk.NewCoin(BaseDenom, math.NewIntWithDecimal(1000, 18)))
	require.NoError(t, chainApp.BankKeeper.MintCoins(env.ctx, minttypes.ModuleName, fund))
	require.NoError(t, chainApp.BankKeeper.SendCoinsFromModuleToAccount(env.ctx, minttypes.ModuleName, env.operator, fund))

	// Build the ante handler exactly as setAnteHandler does.
	gaslessAllowlist := networkante.NewAllowlist(networktypes.GaslessMessages(), attestationtypes.GaslessMessages())
	admissionRouter := networkante.NewAdmissionRouter(chainApp.NetworkKeeper, networktypes.GaslessMessages()).
		Merge(networkante.NewAdmissionRouter(chainApp.AttestationKeeper, attestationtypes.GaslessMessages()))

	options := chainante.HandlerOptions{
		Cdc:                    chainApp.AppCodec(),
		AccountKeeper:          chainApp.AccountKeeper,
		BankKeeper:             chainApp.BankKeeper,
		ExtensionOptionChecker: antetypes.HasDynamicFeeExtensionOption,
		EvmKeeper:              chainApp.EVMKeeper,
		FeegrantKeeper:         chainApp.FeeGrantKeeper,
		IBCKeeper:              chainApp.IBCKeeper,
		FeeMarketKeeper:        chainApp.FeeMarketKeeper,
		SignModeHandler:        chainApp.TxConfig().SignModeHandler(),
		SigGasConsumer:         evmante.SigVerificationGasConsumer,
		MaxTxGasWanted:         0,
		DynamicFeeChecker:      true,
		PendingTxListener:      func(common.Hash) {},

		NetworkKeeper:    chainApp.NetworkKeeper,
		GaslessAllowlist: gaslessAllowlist,
		AdmissionRouter:  admissionRouter,
	}
	require.NoError(t, options.Validate())
	env.ante = chainante.NewAnteHandler(options)

	return env
}

// signTx builds a properly signed SIGN_MODE_DIRECT tx. A non-nil ext is set
// as a tx extension option.
func (env *gaslessEnv) signTx(priv cryptotypes.PrivKey, fee sdk.Coins, gas uint64, ext *codectypes.Any, msgs ...sdk.Msg) sdk.Tx {
	env.t.Helper()

	txConfig := env.app.TxConfig()
	b := txConfig.NewTxBuilder()
	require.NoError(env.t, b.SetMsgs(msgs...))
	b.SetFeeAmount(fee)
	b.SetGasLimit(gas)
	if ext != nil {
		b.(authtx.ExtensionOptionsTxBuilder).SetExtensionOptions(ext)
	}

	addr := sdk.AccAddress(priv.PubKey().Address())
	acc := env.app.AccountKeeper.GetAccount(env.ctx, addr)
	require.NotNil(env.t, acc)

	// SIGN_MODE_DIRECT signs over AuthInfo, so the signer info must be in
	// place (with an empty signature) before the sign bytes are computed.
	require.NoError(env.t, b.SetSignatures(signingtypes.SignatureV2{
		PubKey: priv.PubKey(),
		Data: &signingtypes.SingleSignatureData{
			SignMode: signingtypes.SignMode_SIGN_MODE_DIRECT,
		},
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

func (env *gaslessEnv) runAnte(tx sdk.Tx) error {
	_, err := env.ante(env.ctx, tx, false)
	return err
}

func (env *gaslessEnv) statusMsg(node sdk.AccAddress) *networktypes.MsgUpdateNodeStatus {
	return &networktypes.MsgUpdateNodeStatus{
		NodeAddress: node.String(),
		Payload:     networktypes.NodeStatusPayload{Os: "linux", Hostname: "node-1"},
	}
}

func (env *gaslessEnv) statusCounter(node sdk.AccAddress) uint64 {
	counter, err := env.app.NetworkKeeper.NodeStatusCounters.Get(env.ctx, node.String())
	if err != nil {
		return 0
	}
	return counter.DailyCount
}

func (env *gaslessEnv) balance(addr sdk.AccAddress) math.Int {
	return env.app.BankKeeper.GetBalance(env.ctx, addr, BaseDenom).Amount
}

// dynamicFeeExt returns an ExtensionOptionDynamicFeeTx{MaxPriorityPrice: 0}
// extension option.
func dynamicFeeExt(t *testing.T) *codectypes.Any {
	t.Helper()
	ext, err := codectypes.NewAnyWithValue(&antetypes.ExtensionOptionDynamicFeeTx{
		MaxPriorityPrice: math.LegacyZeroDec(),
	})
	require.NoError(t, err)
	return ext
}

func TestGaslessAnte(t *testing.T) {
	env := setupGaslessEnv(t)

	// The env's params came from executing the registered v3 upgrade
	// handler; pin what it seeded.
	t.Run("v3 upgrade handler seeded params", func(t *testing.T) {
		networkParams, err := env.app.NetworkKeeper.GetParams(env.ctx)
		require.NoError(t, err)
		require.Equal(t, []string{attestationtypes.NodeTypeTrust, attestationtypes.NodeTypeNano}, networkParams.LicenseTypes)
		require.Equal(t, []string{attestationtypes.NodeTypeTrust, attestationtypes.NodeTypeNano}, networkParams.AllowedNodeTypes)
		require.Equal(t, sdk.NewCoins(sdk.NewCoin(BaseDenom, math.NewIntWithDecimal(1, 16))), networkParams.DeauthorizeFee)

		attestationParams, err := env.app.AttestationKeeper.Params.Get(env.ctx)
		require.NoError(t, err)
		require.Equal(t, attestationtypes.DefaultParams(), attestationParams)

		owner, err := env.app.PermissionKeeper.IsOwner(env.ctx, networktypes.ModuleName, NetworkNamespaceOwner)
		require.NoError(t, err)
		require.True(t, owner)
	})

	// A zero-fee tx of allowlisted msgs passes the full ante chain despite
	// non-zero fee floors, and the admission quota is burned by the ante
	// itself — no msg handler ran here, which is exactly the property that
	// makes failing txs still burn quota.
	t.Run("zero-fee allowlisted admitted, quota burned", func(t *testing.T) {
		require.NoError(t, env.runAnte(env.signTx(env.nodePriv, nil, 250_000, nil, env.statusMsg(env.node))))
		require.Equal(t, uint64(1), env.statusCounter(env.node))

		require.NoError(t, env.runAnte(env.signTx(env.nodePriv, nil, 250_000, nil, env.statusMsg(env.node))))
		require.Equal(t, uint64(2), env.statusCounter(env.node))

		// No fees were deducted from the coin-less node.
		require.True(t, env.balance(env.node).IsZero())
	})

	// Smuggling a non-allowlisted msg into a zero-fee tx makes the whole tx
	// a regular paid tx, which the fee floors reject.
	t.Run("mixed tx rejected", func(t *testing.T) {
		before := env.statusCounter(env.node)
		bankSend := &banktypes.MsgSend{
			FromAddress: env.node.String(),
			ToAddress:   env.operator.String(),
			Amount:      sdk.NewCoins(sdk.NewInt64Coin(BaseDenom, 1)),
		}
		err := env.runAnte(env.signTx(env.nodePriv, nil, 250_000, nil, env.statusMsg(env.node), bankSend))
		require.Error(t, err)
		require.Contains(t, err.Error(), "fee")

		// The rejected tx burned no quota: it never reached admission.
		require.Equal(t, before, env.statusCounter(env.node))
	})

	// The hard resource caps reject oversized gasless txs before a gas
	// meter is handed out.
	t.Run("caps enforced", func(t *testing.T) {
		params, err := env.app.NetworkKeeper.GetParams(env.ctx)
		require.NoError(t, err)

		// Declared gas above the cap.
		err = env.runAnte(env.signTx(env.nodePriv, nil, params.MaxGaslessGas+1, nil, env.statusMsg(env.node)))
		require.ErrorContains(t, err, "gas")

		// Msg count above the cap.
		msgs := make([]sdk.Msg, params.MaxGaslessMsgs+1)
		for i := range msgs {
			msgs[i] = env.statusMsg(env.node)
		}
		err = env.runAnte(env.signTx(env.nodePriv, nil, params.MaxGaslessGas, nil, msgs...))
		require.ErrorContains(t, err, "msgs")

		// Encoded size above the cap.
		tx := env.signTx(env.nodePriv, nil, params.MaxGaslessGas, nil, env.statusMsg(env.node))
		bigCtx := env.ctx.WithTxBytes(make([]byte, params.MaxGaslessTxBytes+1))
		_, err = env.ante(bigCtx, tx, false)
		require.ErrorContains(t, err, "bytes")

		// A paid tx sails past every gasless cap.
		fee := sdk.NewCoins(sdk.NewCoin(BaseDenom, env.baseFee.MulInt64(int64(params.MaxGaslessGas*10)).TruncateInt()))
		paid := env.signTx(env.operatorPriv, fee, params.MaxGaslessGas*10, nil, &banktypes.MsgSend{
			FromAddress: env.operator.String(),
			ToAddress:   env.node.String(),
			Amount:      sdk.NewCoins(sdk.NewInt64Coin(BaseDenom, 1)),
		})
		_, err = env.ante(bigCtx, paid, false)
		require.NoError(t, err)
	})

	// Signer standing is enforced at the ante, exhausted quotas reject at
	// admission, and the attestation msgs route to the attestation keeper.
	t.Run("admission standing and quota", func(t *testing.T) {
		// A signer with an account but no node record has no standing.
		strangerPriv, stranger := env.newSigner()
		err := env.runAnte(env.signTx(strangerPriv, nil, 250_000, nil, &networktypes.MsgUpdateNodeStatus{
			NodeAddress: stranger.String(),
		}))
		require.ErrorContains(t, err, "not found")

		// Exhaust node2's daily status quota.
		params, err := env.app.NetworkKeeper.GetParams(env.ctx)
		require.NoError(t, err)
		for i := env.statusCounter(env.node2); i < params.StatusDailyLimit; i++ {
			require.NoError(t, env.runAnte(env.signTx(env.node2Priv, nil, 250_000, nil, env.statusMsg(env.node2))))
		}
		err = env.runAnte(env.signTx(env.node2Priv, nil, 250_000, nil, env.statusMsg(env.node2)))
		require.ErrorContains(t, err, "quota")

		// The attestation route dispatches to the attestation keeper and
		// burns its counter.
		attest := &attestationtypes.MsgAttestRwa{
			NodeAddress: env.node.String(),
			Attestations: []attestationtypes.ContractAttestation{
				{ContractAddress: "0xaa", CurrentSupply: math.NewInt(1), BlockHeight: 1},
			},
		}
		require.NoError(t, env.runAnte(env.signTx(env.nodePriv, nil, 250_000, nil, attest)))
		counter, err := env.app.AttestationKeeper.RwaCounters.Get(env.ctx, env.node.String())
		require.NoError(t, err)
		require.Equal(t, uint64(1), counter.DailyCount)
	})

	// Regular txs pay exactly the fee semantics they always had.
	t.Run("paid path untouched", func(t *testing.T) {
		const gas = 200_000
		declared := sdk.NewCoins(sdk.NewCoin(BaseDenom, env.baseFee.MulInt64(gas).TruncateInt()))
		send := &banktypes.MsgSend{
			FromAddress: env.operator.String(),
			ToAddress:   env.node.String(),
			Amount:      sdk.NewCoins(sdk.NewInt64Coin(BaseDenom, 1)),
		}

		// Zero fee on a paid msg is rejected by the floors.
		err := env.runAnte(env.signTx(env.operatorPriv, nil, gas, nil, send))
		require.Error(t, err)
		require.Contains(t, err.Error(), "fee")

		// An adequate fee passes, and without a dynamic-fee extension the
		// full declared fee is deducted (tip cap defaults to MaxInt64, so
		// the effective price is the fee cap).
		before := env.balance(env.operator)
		require.NoError(t, env.runAnte(env.signTx(env.operatorPriv, declared, gas, nil, send)))
		deducted := before.Sub(env.balance(env.operator))
		require.Equal(t, declared.AmountOf(BaseDenom), deducted)
	})

	// The ExtensionOptionDynamicFeeTx{MaxPriorityPrice: 0} semantics that
	// motivated charging the deauthorize cost in the msg handler: a tx can
	// declare an arbitrarily large fee yet only baseFee x gas is deducted,
	// so an ante check against the declared fee would be bypassable.
	t.Run("dynamic fee extension", func(t *testing.T) {
		const gas = 200_000
		// Declared fee: 1 TSC. Effective fee: baseFee x gas, orders of
		// magnitude smaller.
		declared := sdk.NewCoins(sdk.NewCoin(BaseDenom, math.NewIntWithDecimal(1, 18)))
		effective := env.baseFee.MulInt64(gas).Ceil().TruncateInt()

		deauthorize := &networktypes.MsgDeauthorizeActivationKey{
			Operator:          env.operator.String(),
			ActivationAddress: env.key.String(),
		}

		before := env.balance(env.operator)
		require.NoError(t, env.runAnte(env.signTx(env.operatorPriv, declared, gas, dynamicFeeExt(t), deauthorize)))
		deducted := before.Sub(env.balance(env.operator))

		require.Equal(t, effective, deducted, "only baseFee x gas is deducted under MaxPriorityPrice=0")
		require.True(t, deducted.LT(declared.AmountOf(BaseDenom)))

		// The extension does not open a fee bypass for gasless msgs either:
		// a zero-fee allowlisted tx carrying it still clears as gasless,
		// with no deduction.
		counterBefore := env.statusCounter(env.node)
		nodeBalanceBefore := env.balance(env.node)
		require.NoError(t, env.runAnte(env.signTx(env.nodePriv, nil, 250_000, dynamicFeeExt(t), env.statusMsg(env.node))))
		require.Equal(t, counterBefore+1, env.statusCounter(env.node))
		require.Equal(t, nodeBalanceBefore, env.balance(env.node))
	})
}
