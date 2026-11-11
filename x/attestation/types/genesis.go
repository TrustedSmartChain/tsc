package types

import (
	"fmt"

	networktypes "github.com/nodelabs-sdk/nodelabs/x/network/types"
)

// DefaultGenesis returns the default genesis state.
func DefaultGenesis() *GenesisState {
	return &GenesisState{
		Params:      DefaultParams(),
		RwaCounters: []AttestationCounter{},
		RwuCounters: []AttestationCounter{},
		NodeStats:   []AttestationStats{},
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
	if err := validateCounters("rwu_counters", gs.RwuCounters); err != nil {
		return err
	}
	return validateNodeStats(gs.NodeStats)
}

func validateNodeStats(stats []AttestationStats) error {
	seen := make(map[string]struct{}, len(stats))
	for _, s := range stats {
		if err := networktypes.ValidateCanonicalAddress("node", s.NodeAddress); err != nil {
			return fmt.Errorf("node_stats: %w", err)
		}
		if _, dup := seen[s.NodeAddress]; dup {
			return fmt.Errorf("node_stats: duplicate stats for %s", s.NodeAddress)
		}
		// A node only gets an entry by attesting, so an entry with neither
		// time set is not one the chain could have written.
		if s.LastRwaTime == nil && s.LastRwuTime == nil {
			return fmt.Errorf("node_stats: %s: at least one of last_rwa_time and last_rwu_time must be set", s.NodeAddress)
		}
		seen[s.NodeAddress] = struct{}{}
	}
	return nil
}

func validateCounters(name string, counters []AttestationCounter) error {
	seen := make(map[string]struct{}, len(counters))
	for _, c := range counters {
		if err := networktypes.ValidateCanonicalAddress("node", c.NodeAddress); err != nil {
			return fmt.Errorf("%s: %w", name, err)
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
