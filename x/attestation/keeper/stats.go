package keeper

import (
	"context"
	"errors"
	"time"

	"cosmossdk.io/collections"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/v4/x/attestation/types"
)

// statsKind selects which last-attested time in AttestationStats a msg sets.
type statsKind func(*types.AttestationStats) **time.Time

var (
	rwaStats statsKind = func(s *types.AttestationStats) **time.Time { return &s.LastRwaTime }
	rwuStats statsKind = func(s *types.AttestationStats) **time.Time { return &s.LastRwuTime }
)

// GetNodeStats returns when a node last attested each kind, with unset times
// if the node has never attested.
func (k Keeper) GetNodeStats(ctx context.Context, nodeAddr string) (types.AttestationStats, error) {
	stats, err := k.NodeStats.Get(ctx, nodeAddr)
	if errors.Is(err, collections.ErrNotFound) {
		return types.AttestationStats{NodeAddress: nodeAddr}, nil
	}
	return stats, err
}

// recordNodeStats sets the node's last-attested time for kind to the current
// block time. It runs in the msg handler rather than ante admission, so it
// covers gasless and fee-paying txs alike, and only msgs that execute
// successfully.
func (k Keeper) recordNodeStats(ctx context.Context, nodeAddr string, kind statsKind) error {
	stats, err := k.GetNodeStats(ctx, nodeAddr)
	if err != nil {
		return err
	}

	blockTime := sdk.UnwrapSDKContext(ctx).BlockTime()
	*kind(&stats) = &blockTime

	return k.NodeStats.Set(ctx, nodeAddr, stats)
}
