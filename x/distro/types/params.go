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

// NewParams creates a new Params instance.
func NewParams(
	minting_address string,
	receiving_address string,
	denom string,
	max_supply string,
	distribution_start_date string,
	months_in_halving_period uint64) Params {
	return Params{
		MintingAddress:        minting_address,
		ReceivingAddress:      receiving_address,
		Denom:                 denom,
		MaxSupply:             max_supply,
		DistributionStartDate: distribution_start_date,
		MonthsInHalvingPeriod: months_in_halving_period,
	}
}

func DefaultParams() Params {
	return NewParams(DefaultMintingAddress, DefaultReceivingAddress, DefaultDenom, DefaultMaxSupply, DefaultDistributionStartDate, DefaultMonthsInHalvingPeriod)
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
