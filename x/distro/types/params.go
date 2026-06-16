package types

import (
	"fmt"
	"time"

	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
)

const DefaultMintingAddress string = "tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"
const DefaultReceivingAddress string = "tsc1grl8wuaj0yg6wvzvyxdtnnajp9em49m5fjz07v"
const DefaultDenom string = "aTSC"
const DefaultMaxSupply string = "21000000000000000000000000"
const DefaultDistributionStartDate string = "2025-07-22"
const DefaultMonthsInHalvingPeriod uint64 = 48

// Decentralized distribution defaults.
const DefaultDistributionLicenseTypeID string = "tsc.node"
const DefaultLicenseTallyThreshold string = "0.667"
const DefaultStakeTallyThreshold string = "0.667"
const DefaultEpochIdentifier string = "day"

// DefaultDistributionTypeID is the id of the single distribution type configured
// by default (claiming 100% of the day's budget).
const DefaultDistributionTypeID string = "default"

// DefaultReviewDelayDays keeps a distribution PENDING (challengeable) for three
// days after consensus before it auto-promotes to LIVE.
const DefaultReviewDelayDays uint64 = 3

// DefaultChallengeBond is the bond escrowed to challenge a pending distribution
// (10 tokens at 18 decimals). Tune per economic policy.
const DefaultChallengeBond string = "10000000000000000000"

// DefaultVoteWindowDays is how many days a VOTING distribution may remain open
// (without reaching consensus) before it expires and stops being tallied.
const DefaultVoteWindowDays uint64 = 7

// NewParams creates a new Params instance.
func NewParams(
	minting_address string,
	receiving_address string,
	denom string,
	max_supply string,
	distribution_start_date string,
	months_in_halving_period uint64,
	distribution_license_type_id string,
	epoch_identifier string,
	review_delay_days uint64,
	challenge_bond string,
	vote_window_days uint64,
	validator_addresses []string,
	distribution_types []DistributionType) Params {
	return Params{
		MintingAddress:            minting_address,
		ReceivingAddress:          receiving_address,
		Denom:                     denom,
		MaxSupply:                 max_supply,
		DistributionStartDate:     distribution_start_date,
		MonthsInHalvingPeriod:     months_in_halving_period,
		DistributionLicenseTypeId: distribution_license_type_id,
		EpochIdentifier:           epoch_identifier,
		ReviewDelayDays:           review_delay_days,
		ChallengeBond:             challenge_bond,
		VoteWindowDays:            vote_window_days,
		ValidatorAddresses:        validator_addresses,
		DistributionTypes:         distribution_types,
	}
}

// DefaultDistributionTypes returns the single default distribution type, which
// claims 100% of the day's budget under two uncapped categories ("type1",
// "type2") and requires both the license and stake tallies (matching the pre-v2
// single-distribution behaviour). Operators override this set via gov.
func DefaultDistributionTypes() []DistributionType {
	return []DistributionType{
		{
			Id:         DefaultDistributionTypeID,
			Percentage: "1",
			Categories: []DistributionCategory{
				{Name: "type1", Percentage: ""},
				{Name: "type2", Percentage: ""},
			},
			LicenseTallyThreshold:   DefaultLicenseTallyThreshold,
			StakeTallyThreshold:     DefaultStakeTallyThreshold,
			ValidatorTallyThreshold: "",
		},
	}
}

func DefaultParams() Params {
	return NewParams(
		DefaultMintingAddress,
		DefaultReceivingAddress,
		DefaultDenom,
		DefaultMaxSupply,
		DefaultDistributionStartDate,
		DefaultMonthsInHalvingPeriod,
		DefaultDistributionLicenseTypeID,
		DefaultEpochIdentifier,
		DefaultReviewDelayDays,
		DefaultChallengeBond,
		DefaultVoteWindowDays,
		nil,
		DefaultDistributionTypes(),
	)
}

// Validate validates the set of params.
func (p Params) Validate() error {
	if err := validateMintingAddress(p.MintingAddress); err != nil {
		return err
	}
	if err := validateReceivingAddress(p.ReceivingAddress); err != nil {
		return err
	}
	if err := validateDenom(p.Denom); err != nil {
		return err
	}
	if err := validateMaxSupply(p.MaxSupply); err != nil {
		return err
	}
	if err := validateDistributionStartDate(p.DistributionStartDate); err != nil {
		return err
	}
	if err := validateMonthsInHalvingPeriod(p.MonthsInHalvingPeriod); err != nil {
		return err
	}
	if err := validateLicenseTypeID(p.DistributionLicenseTypeId); err != nil {
		return err
	}
	if err := validateEpochIdentifier(p.EpochIdentifier); err != nil {
		return err
	}
	if err := validateChallengeBond(p.ChallengeBond); err != nil {
		return err
	}
	if err := validateValidatorAddresses(p.ValidatorAddresses); err != nil {
		return err
	}
	if err := validateDistributionTypes(p.DistributionTypes); err != nil {
		return err
	}
	if p.VoteWindowDays == 0 {
		return fmt.Errorf("vote window days must be greater than zero")
	}
	// A zero review delay promotes a consensus root to LIVE at the very next epoch,
	// leaving no window in which a faulty root can be challenged. The challenge /
	// bond / review machinery is a core safety mechanism, so require a non-zero
	// window.
	if p.ReviewDelayDays == 0 {
		return fmt.Errorf("review delay days must be greater than zero (a zero delay removes the challenge window)")
	}

	return nil
}

// validateChallengeBond ensures the bond is a non-negative integer amount.
func validateChallengeBond(v string) error {
	if v == "" {
		return fmt.Errorf("challenge bond cannot be empty (use \"0\" to disable)")
	}
	bond, ok := math.NewIntFromString(v)
	if !ok {
		return fmt.Errorf("challenge bond must be a valid integer")
	}
	if bond.IsNegative() {
		return fmt.Errorf("challenge bond cannot be negative")
	}
	return nil
}

func validateLicenseTypeID(v string) error {
	if v == "" {
		return fmt.Errorf("distribution license type id cannot be empty")
	}
	return nil
}

// validateThreshold ensures the value is a decimal in the range (0, 1].
func validateThreshold(name, v string) error {
	if v == "" {
		return fmt.Errorf("%s cannot be empty", name)
	}
	dec, err := math.LegacyNewDecFromStr(v)
	if err != nil {
		return fmt.Errorf("%s must be a valid decimal: %w", name, err)
	}
	if !dec.IsPositive() {
		return fmt.Errorf("%s must be greater than zero", name)
	}
	if dec.GT(math.LegacyOneDec()) {
		return fmt.Errorf("%s must not exceed 1", name)
	}
	return nil
}

func validateEpochIdentifier(v string) error {
	if v == "" {
		return fmt.Errorf("epoch identifier cannot be empty")
	}
	return nil
}
func validateMintingAddress(v string) error {
	if v == "" {
		return fmt.Errorf("minting address cannot be empty")
	}
	_, err := sdk.AccAddressFromBech32(v)
	if err != nil {
		return fmt.Errorf("invalid minting address: %w", err)
	}
	return nil
}
func validateReceivingAddress(v string) error {
	if v == "" {
		return fmt.Errorf("receiving address cannot be empty")
	}
	_, err := sdk.AccAddressFromBech32(v)
	if err != nil {
		return fmt.Errorf("invalid receiving address: %w", err)
	}
	return nil
}
func validateDenom(v string) error {
	if v == "" {
		return fmt.Errorf("denom cannot be empty")
	}

	if DefaultDenom != v {
		return fmt.Errorf("denom must be '%s'", DefaultDenom)
	}
	return nil
}
func validateMaxSupply(v string) error {
	if v == "" {
		return fmt.Errorf("max supply cannot be empty")
	}

	maxSupply, ok := math.NewIntFromString(v)
	if !ok {
		return fmt.Errorf("max supply must be a valid integer")
	}

	if !maxSupply.IsPositive() {
		return fmt.Errorf("max supply must be positive")
	}
	return nil
}
func validateDistributionStartDate(v string) error {
	if v == "" {
		return fmt.Errorf("distribution start date cannot be empty")
	}

	_, err := time.Parse("2006-01-02", v)
	if err != nil {
		return fmt.Errorf("distribution start date must be in YYYY-MM-DD format: %w", err)
	}
	return nil
}
func validateMonthsInHalvingPeriod(v uint64) error {
	if v == 0 {
		return fmt.Errorf("months in halving period must be greater than zero")
	}
	return nil
}

// validateValidatorAddresses checks each address parses as a bech32 account
// address and that there are no duplicates. An empty list is allowed.
func validateValidatorAddresses(addrs []string) error {
	seen := make(map[string]struct{}, len(addrs))
	for _, a := range addrs {
		if _, err := sdk.AccAddressFromBech32(a); err != nil {
			return fmt.Errorf("invalid validator address %q: %w", a, err)
		}
		if _, dup := seen[a]; dup {
			return fmt.Errorf("duplicate validator address %q", a)
		}
		seen[a] = struct{}{}
	}
	return nil
}

// validateDistributionTypes enforces the per-type rules: at least one type;
// unique non-empty ids; each type percentage in (0,1] with all type percentages
// summing to exactly 1; at least one consensus threshold set per type (each in
// (0,1] when set); non-empty unique category names; and per type either all
// category percentages zero/unset (uncapped) or set and summing to exactly 1.
func validateDistributionTypes(types []DistributionType) error {
	if len(types) == 0 {
		return fmt.Errorf("at least one distribution type must be configured")
	}
	seen := make(map[string]struct{}, len(types))
	typeSum := math.LegacyZeroDec()
	for _, dt := range types {
		if dt.Id == "" {
			return fmt.Errorf("distribution type id cannot be empty")
		}
		if _, dup := seen[dt.Id]; dup {
			return fmt.Errorf("duplicate distribution type %q", dt.Id)
		}
		seen[dt.Id] = struct{}{}

		pct, err := parsePercentage(fmt.Sprintf("distribution type %q percentage", dt.Id), dt.Percentage)
		if err != nil {
			return err
		}
		if !pct.IsPositive() {
			return fmt.Errorf("distribution type %q percentage must be greater than zero", dt.Id)
		}
		typeSum = typeSum.Add(pct)

		hasMechanism := false
		for _, m := range []struct{ name, v string }{
			{"license_tally_threshold", dt.LicenseTallyThreshold},
			{"stake_tally_threshold", dt.StakeTallyThreshold},
			{"validator_tally_threshold", dt.ValidatorTallyThreshold},
		} {
			if m.v == "" || m.v == "0" {
				continue
			}
			if err := validateThreshold(fmt.Sprintf("distribution type %q %s", dt.Id, m.name), m.v); err != nil {
				return err
			}
			hasMechanism = true
		}
		if !hasMechanism {
			return fmt.Errorf("distribution type %q must configure at least one consensus threshold", dt.Id)
		}

		if len(dt.Categories) == 0 {
			return fmt.Errorf("distribution type %q must have at least one category", dt.Id)
		}
		catSeen := make(map[string]struct{}, len(dt.Categories))
		catSum := math.LegacyZeroDec()
		anyCatPct := false
		for _, c := range dt.Categories {
			if c.Name == "" {
				return fmt.Errorf("distribution type %q has an empty category name", dt.Id)
			}
			if _, dup := catSeen[c.Name]; dup {
				return fmt.Errorf("distribution type %q has duplicate category %q", dt.Id, c.Name)
			}
			catSeen[c.Name] = struct{}{}
			cp, err := parsePercentage(fmt.Sprintf("distribution type %q category %q percentage", dt.Id, c.Name), c.Percentage)
			if err != nil {
				return err
			}
			if cp.IsPositive() {
				anyCatPct = true
			}
			catSum = catSum.Add(cp)
		}
		if anyCatPct && !catSum.Equal(math.LegacyOneDec()) {
			return fmt.Errorf("distribution type %q category percentages must sum to exactly 1 (got %s)", dt.Id, catSum)
		}
	}
	if !typeSum.Equal(math.LegacyOneDec()) {
		return fmt.Errorf("distribution type percentages must sum to exactly 1 (got %s)", typeSum)
	}
	return nil
}

// parsePercentage parses a fraction in [0,1]; an empty string is treated as zero.
func parsePercentage(name, v string) (math.LegacyDec, error) {
	if v == "" {
		return math.LegacyZeroDec(), nil
	}
	dec, err := math.LegacyNewDecFromStr(v)
	if err != nil {
		return math.LegacyZeroDec(), fmt.Errorf("%s must be a valid decimal: %w", name, err)
	}
	if dec.IsNegative() {
		return math.LegacyZeroDec(), fmt.Errorf("%s must not be negative", name)
	}
	if dec.GT(math.LegacyOneDec()) {
		return math.LegacyZeroDec(), fmt.Errorf("%s must not exceed 1", name)
	}
	return dec, nil
}

// FindDistributionType returns the configured distribution type with the given
// id, if any.
func (p Params) FindDistributionType(id string) (DistributionType, bool) {
	for _, dt := range p.DistributionTypes {
		if dt.Id == id {
			return dt, true
		}
	}
	return DistributionType{}, false
}

// FindCategory returns the category with the given name within the type, if any.
func (dt DistributionType) FindCategory(name string) (DistributionCategory, bool) {
	for _, c := range dt.Categories {
		if c.Name == name {
			return c, true
		}
	}
	return DistributionCategory{}, false
}

// HasCategoryPercentages reports whether any category in the type carries a
// positive percentage. When false, categories are uncapped within the type
// budget; when true, the type's category percentages sum to exactly 1.
func (dt DistributionType) HasCategoryPercentages() bool {
	for _, c := range dt.Categories {
		if d, err := math.LegacyNewDecFromStr(c.Percentage); err == nil && d.IsPositive() {
			return true
		}
	}
	return false
}
