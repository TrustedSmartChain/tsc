package types

import (
	"context"

	networktypes "github.com/webstack-sdk/webstack/x/network/types"
)

// NetworkKeeper is the x/network keeper surface the attestation module
// consumes, satisfied by webstack's networkkeeper.Keeper. TouchNodeActivity
// carries the recent-activity index invariant (exactly one entry per node,
// moved between day buckets) enforced by webstack-side keeper tests.
type NetworkKeeper interface {
	// IsActiveNode returns the node record and whether it exists with active
	// status.
	IsActiveNode(ctx context.Context, nodeAddr string) (networktypes.Node, bool, error)

	// TouchNodeActivity records counted activity for the node at the current
	// block time.
	TouchNodeActivity(ctx context.Context, nodeAddr string) error

	// EnsureOperatorLicensed rejects operators without at least one active
	// counted license; used by the gasless admission's daily license gate.
	EnsureOperatorLicensed(ctx context.Context, operator string) error
}
