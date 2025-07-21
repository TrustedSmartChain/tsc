package types

import (
	"encoding/json"
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

// Stringer method for Params.
func (p Params) String() string {
	bz, err := json.Marshal(p)
	if err != nil {
		panic(err)
	}

	return string(bz)
}

// Validate does the sanity check on the params.
func (p Params) Validate() error {
	// TODO:
	return nil
}
