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

// DefaultDistributionReviewDelay keeps a distribution PENDING (challengeable) for
// three epochs after consensus before it auto-promotes to LIVE.
const DefaultDistributionReviewDelay uint64 = 3

// DefaultChallengeBond is the bond escrowed to challenge a pending distribution
// (10 tokens at 18 decimals). Tune per economic policy.
const DefaultChallengeBond string = "10000000000000000000"

// NewParams creates a new Params instance.
func NewParams(
	minting_address string,
	receiving_address string,
	denom string,
	max_supply string,
	distribution_start_date string,
	months_in_halving_period uint64,
	distribution_license_type_id string,
	license_tally_threshold string,
	stake_tally_threshold string,
	epoch_identifier string,
	distribution_review_delay uint64,
	challenge_bond string) Params {
	return Params{
		MintingAddress:            minting_address,
		ReceivingAddress:          receiving_address,
		Denom:                     denom,
		MaxSupply:                 max_supply,
		DistributionStartDate:     distribution_start_date,
		MonthsInHalvingPeriod:     months_in_halving_period,
		DistributionLicenseTypeId: distribution_license_type_id,
		LicenseTallyThreshold:     license_tally_threshold,
		StakeTallyThreshold:       stake_tally_threshold,
		EpochIdentifier:           epoch_identifier,
		DistributionReviewDelay:   distribution_review_delay,
		ChallengeBond:             challenge_bond,
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
		DefaultLicenseTallyThreshold,
		DefaultStakeTallyThreshold,
		DefaultEpochIdentifier,
		DefaultDistributionReviewDelay,
		DefaultChallengeBond,
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
	if err := validateThreshold("license_tally_threshold", p.LicenseTallyThreshold); err != nil {
		return err
	}
	if err := validateThreshold("stake_tally_threshold", p.StakeTallyThreshold); err != nil {
		return err
	}
	if err := validateEpochIdentifier(p.EpochIdentifier); err != nil {
		return err
	}
	if err := validateChallengeBond(p.ChallengeBond); err != nil {
		return err
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
