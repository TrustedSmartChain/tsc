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

	// Validate distributions and index them (by date+type) for cross-checks.
	dateStatus := make(map[string]DistributionStatus, len(gs.Distributions))
	for _, d := range gs.Distributions {
		if err := validateDateString(d.Date); err != nil {
			return fmt.Errorf("distribution: %w", err)
		}
		if d.DistroType == "" {
			return fmt.Errorf("distribution for date %q has empty distro_type", d.Date)
		}
		dtKey := d.Date + "\x00" + d.DistroType
		if _, dup := dateStatus[dtKey]; dup {
			return fmt.Errorf("duplicate distribution for date %q type %q", d.Date, d.DistroType)
		}
		dateStatus[dtKey] = d.Status

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
		// The lifecycle timestamps, when set, must be valid days.
		if d.VotingSinceDate != "" {
			if err := validateDateString(d.VotingSinceDate); err != nil {
				return fmt.Errorf("distribution %q has invalid voting_since_date: %w", d.Date, err)
			}
		}
		if d.PendingSinceDate != "" {
			if err := validateDateString(d.PendingSinceDate); err != nil {
				return fmt.Errorf("distribution %q has invalid pending_since_date: %w", d.Date, err)
			}
		}
	}

	// Votes must carry a valid date, a distro type, and a non-empty root.
	for _, v := range gs.Votes {
		if err := validateDateString(v.Date); err != nil {
			return fmt.Errorf("vote: %w", err)
		}
		if v.DistroType == "" {
			return fmt.Errorf("vote for date %q signer %s has empty distro_type", v.Date, v.Signer)
		}
		if len(v.MerkleRoot) == 0 {
			return fmt.Errorf("vote for date %q signer %s has empty merkle root", v.Date, v.Signer)
		}
	}

	// Claimed rewards may only reference a finalized (live) (date, type).
	for _, cr := range gs.ClaimedRewards {
		status, ok := dateStatus[cr.Date+"\x00"+cr.DistroType]
		if !ok {
			return fmt.Errorf("claimed rewards reference unknown date %q type %q", cr.Date, cr.DistroType)
		}
		if status != DISTRIBUTION_STATUS_LIVE {
			return fmt.Errorf("claimed rewards reference non-live date %q type %q (status %s)", cr.Date, cr.DistroType, status)
		}
	}

	// Per-category claim totals must reference a known LIVE (date, type), name a
	// category, and carry a non-negative integer total.
	for _, ct := range gs.CategoryClaimTotals {
		if err := validateDateString(ct.Date); err != nil {
			return fmt.Errorf("category claim total: %w", err)
		}
		status, ok := dateStatus[ct.Date+"\x00"+ct.DistroType]
		if !ok {
			return fmt.Errorf("category claim total references unknown date %q type %q", ct.Date, ct.DistroType)
		}
		if status != DISTRIBUTION_STATUS_LIVE {
			return fmt.Errorf("category claim total references non-live date %q type %q (status %s)", ct.Date, ct.DistroType, status)
		}
		if ct.Category == "" {
			return fmt.Errorf("category claim total for date %q type %q has empty category", ct.Date, ct.DistroType)
		}
		if total, ok := math.NewIntFromString(ct.Total); !ok || total.IsNegative() {
			return fmt.Errorf("category claim total for date %q type %q category %q has invalid total %q", ct.Date, ct.DistroType, ct.Category, ct.Total)
		}
	}

	return nil
}
