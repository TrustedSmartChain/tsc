package types

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultGenesis returns the default genesis state.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:      DefaultParams(),
		RwaCounters: []AttestationCounter{},
		RwuCounters: []AttestationCounter{},
	}
}

// Validate checks the genesis state's structural invariants. Counters
// reference nodes in the network module's store, which is not visible here;
// only local shape is checked.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return fmt.Errorf("params: %w", err)
	}
	if err := validateCounters("rwa_counters", gs.RwaCounters); err != nil {
		return err
	}
	return validateCounters("rwu_counters", gs.RwuCounters)
}

func validateCounters(name string, counters []AttestationCounter) error {
	seen := make(map[string]struct{}, len(counters))
	for _, c := range counters {
		if _, err := sdk.AccAddressFromBech32(c.NodeAddress); err != nil {
			return fmt.Errorf("%s: invalid node address %q: %w", name, c.NodeAddress, err)
		}
		if _, dup := seen[c.NodeAddress]; dup {
			return fmt.Errorf("%s: duplicate counter for %s", name, c.NodeAddress)
		}
		if c.Counter.LatestTime.IsZero() {
			return fmt.Errorf("%s: counter for %s: latest_time must be set", name, c.NodeAddress)
		}
		seen[c.NodeAddress] = struct{}{}
	}
	return nil
}
