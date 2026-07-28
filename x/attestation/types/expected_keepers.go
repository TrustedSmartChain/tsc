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

// LicenseKeeper is the x/license keeper surface the attestation module
// consumes, satisfied by webstack's licensekeeper.Keeper. It is used to bind
// a node's declared type to a license of that same type: x/network stores
// Node.Type as the opaque string its operator supplied and counts licenses
// in aggregate, so without this check an operator holding only a cheaper
// license tier could activate a node declaring a privileged type and claim
// that tier's rights.
type LicenseKeeper interface {
	// CountActiveLicenses returns the number of active licenses holder holds
	// across the given license types, stopping the walk once the count
	// reaches stopAt (zero counts everything).
	CountActiveLicenses(ctx context.Context, holder string, licenseTypes []string, stopAt uint64) (uint64, error)
}
