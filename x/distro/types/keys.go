package types

import (
	"cosmossdk.io/collections"
)

var (
	// ParamsKey saves the current module params.
	ParamsKey = collections.NewPrefix(0)

	// VotesKeyPrefix indexes raw per-signer submitted roots by (epoch, signer).
	VotesKeyPrefix = collections.NewPrefix(1)

	// EpochDistributionsKeyPrefix indexes the canonical distribution per epoch.
	EpochDistributionsKeyPrefix = collections.NewPrefix(2)

	// ClaimedKeyPrefix is the set of claimed reward nonces by (epoch, nonce).
	ClaimedKeyPrefix = collections.NewPrefix(3)
)

const (
	ModuleName = "distro"

	StoreKey = ModuleName

	QuerierRoute = ModuleName
)
