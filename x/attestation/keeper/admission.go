package keeper

import (
	"context"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

// CheckAndConsumeGaslessQuota is the ante-side admission check for the
// attestation module's gasless msgs: the signing node must be active, on
// the counter's first roll of each UTC day the operator must still hold at
// least one active counted license, and the per-node daily counter is
// incremented against the msg's limit. It runs in the ante chain, so its
// writes are committed even when the msg handler later fails (failing txs
// still burn quota) and over-limit spam is rejected at CheckTx. Handlers
// re-read these counters and never re-increment.
func (k Keeper) CheckAndConsumeGaslessQuota(ctx context.Context, m sdk.Msg) error {
	params, err := k.Params.Get(ctx)
	if err != nil {
		return err
	}

	switch msg := m.(type) {
	case *types.MsgAttestRwa:
		return k.consumeAttestQuota(ctx, msg.NodeAddress, k.RwaCounters, params.RwaDailyLimit)
	case *types.MsgAttestRwu:
		return k.consumeAttestQuota(ctx, msg.NodeAddress, k.RwuCounters, params.RwuDailyLimit)
	default:
		return errorsmod.Wrapf(types.ErrNotGaslessMsg, "%T", m)
	}
}

func (k Keeper) consumeAttestQuota(ctx context.Context, nodeAddr string, counters collections.Map[string, types.ActivityCounter], limit uint64) error {
	node, active, err := k.networkKeeper.IsActiveNode(ctx, nodeAddr)
	if err != nil {
		return err
	}
	if node.Address == "" {
		return errorsmod.Wrapf(types.ErrNodeNotFound, "node %s", nodeAddr)
	}
	if !active {
		return errorsmod.Wrapf(types.ErrNodeNotActive, "node %s", nodeAddr)
	}

	sdkCtx := sdk.UnwrapSDKContext(ctx)
	blockTime := sdkCtx.BlockTime()

	counter, err := counters.Get(ctx, nodeAddr)
	if err != nil {
		counter = types.ActivityCounter{}
	}
	if !types.SameDay(counter.LatestTime, blockTime) {
		counter.DailyCount = 0
	}
	counter.LatestTime = blockTime
	counter.LatestHeight = sdkCtx.BlockHeight()

	// License gating of ongoing service, checked on the first counted tx of
	// each UTC day: a revoked operator's fleet must not keep its free tx
	// channel.
	if counter.DailyCount == 0 {
		if err := k.networkKeeper.EnsureOperatorLicensed(ctx, node.Operator); err != nil {
			return err
		}
	}

	counter.DailyCount++
	if counter.DailyCount > limit {
		return errorsmod.Wrapf(types.ErrQuotaExceeded, "node %s exceeded %d attestations today", nodeAddr, limit)
	}
	return counters.Set(ctx, nodeAddr, counter)
}
