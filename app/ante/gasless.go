package ante

import (
	storetypes "cosmossdk.io/store/types"
	txsigning "cosmossdk.io/x/tx/signing"

	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/x/auth/ante"
	authkeeper "github.com/cosmos/cosmos-sdk/x/auth/keeper"
	ibcante "github.com/cosmos/ibc-go/v10/modules/core/ante"
	ibckeeper "github.com/cosmos/ibc-go/v10/modules/core/keeper"

	nodetypes "github.com/TrustedSmartChain/tsc/v3/x/node/types"
)

// GaslessMsgTypeURLs lists message type URLs that are exempt from fees and gas limits.
// Add new gasless message types here — no governance vote required.
var GaslessMsgTypeURLs = []string{
	sdk.MsgTypeURL(&nodetypes.MsgCheckIn{}),
}

// GaslessDecorator short-circuits the fee/gas ante chain for whitelisted message types.
// It must be positioned after NewValidateMemoDecorator and before NewMinGasPriceDecorator.
//
// For gasless txs: sets an infinite gas meter and runs only the sig/sequence decorators.
// For all other txs: passes through to the normal ante chain unchanged.
type GaslessDecorator struct {
	inner sdk.AnteHandler
}

func NewGaslessDecorator(
	ak authkeeper.AccountKeeper,
	sigGasConsumer ante.SignatureVerificationGasConsumer,
	signModeHandler *txsigning.HandlerMap,
	ibcKeeper *ibckeeper.Keeper,
) GaslessDecorator {
	return GaslessDecorator{
		inner: sdk.ChainAnteDecorators(
			ante.NewSetPubKeyDecorator(ak),
			ante.NewValidateSigCountDecorator(ak),
			ante.NewSigGasConsumeDecorator(ak, sigGasConsumer),
			ante.NewSigVerificationDecorator(ak, signModeHandler),
			ante.NewIncrementSequenceDecorator(ak),
			ibcante.NewRedundantRelayDecorator(ibcKeeper),
		),
	}
}

func (d GaslessDecorator) AnteHandle(ctx sdk.Context, tx sdk.Tx, simulate bool, next sdk.AnteHandler) (sdk.Context, error) {
	if !isGaslessTx(tx) {
		return next(ctx, tx, simulate)
	}
	newCtx := ctx.WithGasMeter(storetypes.NewInfiniteGasMeter())
	return d.inner(newCtx, tx, simulate)
}

// isGaslessTx returns true only when every message in the tx is on the whitelist.
// A tx mixing gasless and fee-paying messages is NOT gasless.
func isGaslessTx(tx sdk.Tx) bool {
	msgs := tx.GetMsgs()
	if len(msgs) == 0 {
		return false
	}
	for _, msg := range msgs {
		typeURL := sdk.MsgTypeURL(msg)
		found := false
		for _, allowed := range GaslessMsgTypeURLs {
			if typeURL == allowed {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
