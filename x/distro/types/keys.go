package types

import (
	"cosmossdk.io/collections"
)

var (
	// ParamsKey saves the current module params.
	ParamsKey = collections.NewPrefix(0)

	// VotesKeyPrefix indexes raw per-signer submitted roots by (date, signer).
	VotesKeyPrefix = collections.NewPrefix(1)

	// DistributionsKeyPrefix indexes the canonical distribution per day (date).
	DistributionsKeyPrefix = collections.NewPrefix(2)

	// ClaimedKeyPrefix is the set of claimed reward nonces by (date, nonce).
	ClaimedKeyPrefix = collections.NewPrefix(3)

	// ClaimTotalsKeyPrefix accumulates the claimed amount per (date, category).
	ClaimTotalsKeyPrefix = collections.NewPrefix(4)
)

const (
	ModuleName = "distro"

	StoreKey = ModuleName

	QuerierRoute = ModuleName
)
