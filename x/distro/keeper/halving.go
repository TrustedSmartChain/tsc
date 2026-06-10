package keeper

import (
	"time"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// The halving emission math lives in x/distro/types (pure, store-free) so the
// off-chain distribution worker can import it and reproduce the chain's budget
// exactly. These wrappers only translate module Params into the explicit
// inputs the types functions take — the math itself must not be duplicated
// here.

// dateBudget returns the maximum total amount distributable for a single day
// (the per-day cap that bounds on-demand claim minting). See types.DateBudget
// for the schedule semantics.
func dateBudget(params types.Params, date string) (math.Int, error) {
	maxSupply, err := maxSupplyFromParams(params)
	if err != nil {
		return math.ZeroInt(), err
	}
	return types.DateBudget(date, maxSupply, params.DistributionStartDate, params.MonthsInHalvingPeriod)
}

// newHalvingSchedule builds the types.HalvingSchedule for the module's params.
func newHalvingSchedule(params types.Params) (*types.HalvingSchedule, error) {
	maxSupply, err := maxSupplyFromParams(params)
	if err != nil {
		return nil, err
	}
	return types.NewHalvingSchedule(params.DistributionStartDate, maxSupply, params.MonthsInHalvingPeriod)
}

func maxSupplyFromParams(params types.Params) (math.Int, error) {
	maxSupply, ok := math.NewIntFromString(params.MaxSupply)
	if !ok {
		return math.ZeroInt(), errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "invalid max supply")
	}
	return maxSupply, nil
}

func parseDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}

// dateForEpoch maps an x/epochs epoch number to its distribution day
// (YYYY-MM-DD): epoch 1 is the start date, epoch N is startDate+(N-1) days. This
// is how the daily epoch hook is translated into the date the module keys state
// by.
func dateForEpoch(startDateStr string, epoch int64) (string, error) {
	start, err := parseDate(startDateStr)
	if err != nil {
		return "", errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid distribution start date: %v", err)
	}
	if epoch <= 0 {
		return "", errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "epoch must be positive")
	}
	return start.AddDate(0, 0, int(epoch-1)).Format("2006-01-02"), nil
}

// daysBetween returns the number of whole days from `from` to `to` (positive
// when `to` is later). Both dates must be YYYY-MM-DD.
func daysBetween(from, to string) (int64, error) {
	f, err := parseDate(from)
	if err != nil {
		return 0, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid date %q: must be YYYY-MM-DD", from)
	}
	t, err := parseDate(to)
	if err != nil {
		return 0, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid date %q: must be YYYY-MM-DD", to)
	}
	return int64(t.Sub(f).Hours() / 24), nil
}
