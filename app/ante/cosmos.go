package ante

import (
	cosmosante "github.com/cosmos/evm/ante/cosmos"
	evmante "github.com/cosmos/evm/ante/evm"
	evmtypes "github.com/cosmos/evm/x/vm/types"
	ibcante "github.com/cosmos/ibc-go/v10/modules/core/ante"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	sdkvesting "github.com/cosmos/cosmos-sdk/x/auth/vesting/types"

	networkante "github.com/nodelabs-sdk/nodelabs/x/network/ante"
)

// newCosmosAnteHandler creates the default ante handler for Cosmos
// transactions, composed with the x/network gasless machinery:
//
//   - hard resource caps reject oversized gasless txs before
//     SetUpContextDecorator hands out a gas meter;
//   - MinGasPriceDecorator is skipped for gasless txs;
//   - the TxFeeChecker clears gasless txs at zero fee and delegates paid
//     txs to the dynamic fee checker;
//   - the admission decorator, after signature verification, checks signer
//     standing and burns the per-signer daily quota — committed even when
//     the msg handler later fails.
func newCosmosAnteHandler(ctx sdk.Context, options HandlerOptions) sdk.AnteHandler {
	feemarketParams := options.FeeMarketKeeper.GetParams(ctx)
	var txFeeChecker ante.TxFeeChecker
	if options.DynamicFeeChecker {
		txFeeChecker = evmante.NewDynamicFeeChecker(&feemarketParams)
	}
	txFeeChecker = networkante.NewGaslessTxFeeChecker(options.GaslessAllowlist, txFeeChecker)

	return sdk.ChainAnteDecorators(
		NewRedelegationMarkerDecorator(),
		NewMinSelfDelegationDecorator(),
		cosmosante.NewRejectMessagesDecorator(), // reject MsgEthereumTxs
		cosmosante.NewAuthzLimiterDecorator( // disable the Msg types that cannot be included on an authz.MsgExec msgs field
			sdk.MsgTypeURL(&evmtypes.MsgEthereumTx{}),
			sdk.MsgTypeURL(&sdkvesting.MsgCreateVestingAccount{}),
		),
		networkante.NewGaslessCapsDecorator(options.NetworkKeeper, options.GaslessAllowlist),
		ante.NewSetUpContextDecorator(),
		ante.NewExtensionOptionsDecorator(options.ExtensionOptionChecker),
		ante.NewValidateBasicDecorator(),
		ante.NewTxTimeoutHeightDecorator(),
		ante.NewValidateMemoDecorator(options.AccountKeeper),
		networkante.NewSkipForGaslessDecorator(cosmosante.NewMinGasPriceDecorator(&feemarketParams), options.GaslessAllowlist),
		ante.NewConsumeGasForTxSizeDecorator(options.AccountKeeper),
		ante.NewDeductFeeDecorator(options.AccountKeeper, options.BankKeeper, options.FeegrantKeeper, txFeeChecker),
		// SetPubKeyDecorator must be called before all signature verification decorators
		ante.NewSetPubKeyDecorator(options.AccountKeeper),
		ante.NewValidateSigCountDecorator(options.AccountKeeper),
		ante.NewSigGasConsumeDecorator(options.AccountKeeper, options.SigGasConsumer),
		ante.NewSigVerificationDecorator(options.AccountKeeper, options.SignModeHandler),
		ante.NewIncrementSequenceDecorator(options.AccountKeeper),
		ibcante.NewRedundantRelayDecorator(options.IBCKeeper),
		networkante.NewGaslessAdmissionDecorator(options.GaslessAllowlist, options.AdmissionRouter),
	)
}
