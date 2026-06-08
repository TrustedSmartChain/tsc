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

	// Validate distributions and index them for cross-checks.
	dateStatus := make(map[string]DistributionStatus, len(gs.Distributions))
	for _, d := range gs.Distributions {
		if _, dup := dateStatus[d.Date]; dup {
			return fmt.Errorf("duplicate distribution for date %q", d.Date)
		}
		dateStatus[d.Date] = d.Status

		// PENDING/UNDER_REVIEW/LIVE must carry a candidate/finalized root.
		// VOTING and EXPIRED days legitimately have no root.
		switch d.Status {
		case DISTRIBUTION_STATUS_VOTING, DISTRIBUTION_STATUS_EXPIRED:
			// no root required
		default:
			if len(d.MerkleRoot) == 0 {
				return fmt.Errorf("distribution %q in status %s has empty merkle root", d.Date, d.Status)
			}
		}
		// An UNDER_REVIEW day must record who challenged it (for bond resolution).
		if d.Status == DISTRIBUTION_STATUS_UNDER_REVIEW && d.Challenger == "" {
			return fmt.Errorf("distribution %q is under review but has no challenger", d.Date)
		}
		if d.ChallengeBond != "" {
			if bond, ok := math.NewIntFromString(d.ChallengeBond); !ok || bond.IsNegative() {
				return fmt.Errorf("distribution %q has invalid challenge bond %q", d.Date, d.ChallengeBond)
			}
		}
		if d.ClaimedAmount != "" {
			if claimed, ok := math.NewIntFromString(d.ClaimedAmount); !ok || claimed.IsNegative() {
				return fmt.Errorf("distribution %q has invalid claimed amount %q", d.Date, d.ClaimedAmount)
			}
		}
	}

	// Votes must carry a non-empty root.
	for _, v := range gs.Votes {
		if len(v.MerkleRoot) == 0 {
			return fmt.Errorf("vote for date %q signer %s has empty merkle root", v.Date, v.Signer)
		}
	}

	// Claimed rewards may only reference a finalized (live) distribution.
	for _, cr := range gs.ClaimedRewards {
		status, ok := dateStatus[cr.Date]
		if !ok {
			return fmt.Errorf("claimed rewards reference unknown date %q", cr.Date)
		}
		if status != DISTRIBUTION_STATUS_LIVE {
			return fmt.Errorf("claimed rewards reference non-live date %q (status %s)", cr.Date, status)
		}
	}

	return nil
}
