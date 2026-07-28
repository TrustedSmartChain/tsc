package types

import "fmt"

// DefaultParams returns the default attestation module parameters.
func DefaultParams() Params {
	return Params{
		RwaDailyLimit: 5,
		RwuDailyLimit: 5,
	}
}

// Validate checks the parameter set is well-formed.
func (p Params) Validate() error {
	if p.RwaDailyLimit == 0 {
		return fmt.Errorf("rwa_daily_limit must be positive")
	}
	if p.RwuDailyLimit == 0 {
		return fmt.Errorf("rwu_daily_limit must be positive")
	}
	return nil
}
