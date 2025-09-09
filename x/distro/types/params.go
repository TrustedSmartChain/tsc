package types

import (
	"fmt"
	"time"

	sdk "github.com/cosmos/cosmos-sdk/types"
)

// DefaultParams returns default module parameters.
func DefaultParams() Params {
	// TODO:
	return Params{
		MintingAddress:        "tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4",
		ReceivingAddress:      "tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4",
		Denom:                 "aTSC",
		MaxSupply:             "21000000000000000000000000",
		DistributionStartDate: "2025-01-01",
		MonthsInHalvingPeriod: 48,
	}
}

// Validate validates the set of params.
func (p Params) Validate() error {
	if err := validateMinitingAddress(p.MintingAddress); err != nil {
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
func validateMinitingAddress(v string) error {
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
	return nil
}
func validateMaxSupply(v string) error {
	if v == "" {
		return fmt.Errorf("max supply cannot be empty")
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
