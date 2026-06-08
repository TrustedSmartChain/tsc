package types

import (
	"fmt"

	"cosmossdk.io/math"
)

// DefaultIndex is the default global index
const DefaultIndex uint64 = 1

// DefaultGenesis returns the default genesis state
func DefaultGenesis() *GenesisState {
	return &GenesisState{

		Params: DefaultParams(),
	}
}

// Validate performs basic genesis state validation returning an error upon any
// failure.
func (gs GenesisState) Validate() error {
	if err := gs.Params.Validate(); err != nil {
		return err
	}

	// Validate epoch distributions and index them for cross-checks.
	epochStatus := make(map[int64]DistributionStatus, len(gs.EpochDistributions))
	for _, ed := range gs.EpochDistributions {
		if _, dup := epochStatus[ed.Epoch]; dup {
			return fmt.Errorf("duplicate epoch distribution for epoch %d", ed.Epoch)
		}
		epochStatus[ed.Epoch] = ed.Status

		// PENDING/UNDER_REVIEW/LIVE must carry a candidate/finalized root.
		// VOTING and EXPIRED epochs legitimately have no root.
		switch ed.Status {
		case DISTRIBUTION_STATUS_VOTING, DISTRIBUTION_STATUS_EXPIRED:
			// no root required
		default:
			if len(ed.MerkleRoot) == 0 {
				return fmt.Errorf("epoch distribution %d in status %s has empty merkle root", ed.Epoch, ed.Status)
			}
		}
		// An UNDER_REVIEW epoch must record who challenged it (for bond resolution).
		if ed.Status == DISTRIBUTION_STATUS_UNDER_REVIEW && ed.Challenger == "" {
			return fmt.Errorf("epoch distribution %d is under review but has no challenger", ed.Epoch)
		}
		if ed.ChallengeBond != "" {
			if bond, ok := math.NewIntFromString(ed.ChallengeBond); !ok || bond.IsNegative() {
				return fmt.Errorf("epoch distribution %d has invalid challenge bond %q", ed.Epoch, ed.ChallengeBond)
			}
		}
		if ed.ClaimedAmount != "" {
			if claimed, ok := math.NewIntFromString(ed.ClaimedAmount); !ok || claimed.IsNegative() {
				return fmt.Errorf("epoch distribution %d has invalid claimed amount %q", ed.Epoch, ed.ClaimedAmount)
			}
		}
	}

	// Votes must carry a non-empty root.
	for _, v := range gs.Votes {
		if len(v.MerkleRoot) == 0 {
			return fmt.Errorf("vote for epoch %d signer %s has empty merkle root", v.Epoch, v.Signer)
		}
	}

	// Claimed rewards may only reference a finalized (live) epoch distribution.
	for _, cr := range gs.ClaimedRewards {
		status, ok := epochStatus[cr.Epoch]
		if !ok {
			return fmt.Errorf("claimed rewards reference unknown epoch %d", cr.Epoch)
		}
		if status != DISTRIBUTION_STATUS_LIVE {
			return fmt.Errorf("claimed rewards reference non-live epoch %d (status %s)", cr.Epoch, status)
		}
	}

	return nil
}
