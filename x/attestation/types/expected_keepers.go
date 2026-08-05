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

	// EnsureOperatorLicensed rejects operators holding no active license of any
	// license type bound to a registered node type. Used by the gasless
	// admission's daily license gate, which runs per node rather than per node
	// type and so has nothing narrower to check.
	EnsureOperatorLicensed(ctx context.Context, operator string) error

	// EnsureOperatorLicensedForNodeType rejects operators holding no active
	// license of the type bound to nodeType, and unregistered node types
	// outright. It is what binds a node's declared type to a license of that
	// type: x/network stores Node.Type as supplied at activation, so without
	// re-checking here a node whose operator has since been revoked would keep
	// exercising its tier's rights.
	//
	// Checking per attestation rather than at activation is the point — the
	// binding is re-evaluated continuously, so a revocation stops the operator's
	// nodes immediately instead of leaving them permanently labelled.
	//
	// Distinct failures arrive through one return: networktypes.ErrInvalidNodeType
	// for an unregistered type, networktypes.ErrNoActiveLicenses for a registered
	// one the operator holds nothing for. This module propagates rather than
	// discriminating, so it needs no import for the errors.Is targets.
	EnsureOperatorLicensedForNodeType(ctx context.Context, operator, nodeType string) error
}
