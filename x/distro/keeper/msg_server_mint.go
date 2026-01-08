package keeper

import (
	"context"
	"time"

	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	"github.com/TrustedSmartChain/tsc/x/distro/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

func (ms msgServer) Mint(goCtx context.Context, msg *types.MsgMint) (*types.MsgMintResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	signers := msg.GetSigners()

	if len(signers) == 0 {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "signer is required")
	}

	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	if !ms.IsAuthorized(ctx, params, signers[0].String()) {
		return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "unauthorized sender")
	}

	amount, ok := math.NewIntFromString(msg.Amount)
	if !ok {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "invalid amount")
	}

	// Get current supply with proper error handling
	supply := ms.k.bankKeeper.GetSupply(ctx, params.Denom)
	if supply.Amount.IsNegative() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "current supply is negative")
	}

	currentSupply := supply.Amount
	maxSupply, ok := math.NewIntFromString(params.MaxSupply)
	if !ok {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "invalid max supply")
	}
	if currentSupply.Add(amount).GT(maxSupply) {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "max supply exceeded")
	}

	if err := validateMintingLimits(ctx, currentSupply, amount, params); err != nil {
		return nil, err
	}

	// Ensure the module account exists
	moduleAddr := ms.k.accountKeeper.GetModuleAddress(types.ModuleName)
	if moduleAddr == nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "module account not found")
	}

	coins := sdk.NewCoins(sdk.NewCoin(params.Denom, amount))
	err = ms.k.bankKeeper.MintCoins(ctx, types.ModuleName, coins)
	if err != nil {
		return nil, err
	}

	if err := ms.depositCoins(ctx, params.ReceivingAddress, amount, params.Denom); err != nil {
		return nil, err
	}

	return &types.MsgMintResponse{}, nil
}

func parseDate(dateStr string) (time.Time, error) {
	return time.Parse("2006-01-02", dateStr)
}

func (ms msgServer) IsAuthorized(ctx context.Context, params types.Params, mintingAddress string) bool {
	return params.MintingAddress == mintingAddress
}

func (ms msgServer) depositCoins(ctx context.Context, toAddress string, amount math.Int, denom string) error {
	acct, err := sdk.AccAddressFromBech32(toAddress)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidAddress, "invalid address '%s'", toAddress)
	}

	// Ensure the module account exists
	moduleAddr := ms.k.accountKeeper.GetModuleAddress(types.ModuleName)
	if moduleAddr == nil {
		return errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "module account not found")
	}

	coins := sdk.NewCoins(sdk.NewCoin(denom, amount))
	if err := ms.k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, acct, coins); err != nil {
		return err
	}
	return nil
}

// validateMintingLimits checks if the requested mint amount is within distributable limits
func validateMintingLimits(ctx sdk.Context, currentSupply math.Int, amount math.Int, params types.Params) error {
	schedule, err := newHalvingSchedule(params)
	if err != nil {
		return err
	}

	targetDate := ctx.BlockTime().Truncate(24 * time.Hour) // Normalize to day

	totalDistributable, err := schedule.TotalDistributableAt(targetDate)
	if err != nil {
		return err
	}

	newSupply := currentSupply.Add(amount)
	if newSupply.GT(totalDistributable) {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "mint would exceed distributable limit: requested total %s, limit %s", newSupply.String(), totalDistributable.String())
	}

	return nil
}

// HalvingSchedule encapsulates the token distribution logic with halving periods
type HalvingSchedule struct {
	StartDate       time.Time
	MonthsPerPeriod uint64
	MaxSupply       math.Int
}

func newHalvingSchedule(params types.Params) (*HalvingSchedule, error) {
	startDate, err := parseDate(params.DistributionStartDate)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid distribution start date: %v", err)
	}

	maxSupply, ok := math.NewIntFromString(params.MaxSupply)
	if !ok {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "invalid max supply")
	}

	return &HalvingSchedule{
		StartDate:       startDate,
		MonthsPerPeriod: params.MonthsInHalvingPeriod,
		MaxSupply:       maxSupply,
	}, nil
}

// PeriodAllocation returns the total tokens allocated for a given period
// Period 1: MaxSupply/2, Period 2: MaxSupply/4, Period 3: MaxSupply/8, etc.
func (h *HalvingSchedule) PeriodAllocation(period uint64) math.Int {
	if period == 0 {
		return math.ZeroInt()
	}

	// Period n allocation = MaxSupply / 2^n
	divisor := math.NewIntFromUint64(1 << period) // 2^period using bit shift
	return h.MaxSupply.Quo(divisor)
}

// PeriodBounds returns the start (inclusive) and end (inclusive) dates for a period
func (h *HalvingSchedule) PeriodBounds(period uint64) (start, end time.Time) {
	monthsOffset := int((period - 1) * h.MonthsPerPeriod)
	start = h.StartDate.AddDate(0, monthsOffset, 0)
	end = h.StartDate.AddDate(0, monthsOffset+int(h.MonthsPerPeriod), -1)
	return start, end
}

// CurrentPeriod returns the halving period for a given date
// Uses day-based comparison to correctly handle leap years and variable month lengths
func (h *HalvingSchedule) CurrentPeriod(targetDate time.Time) (uint64, error) {
	if targetDate.Before(h.StartDate) {
		return 0, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "target date is before distribution start date")
	}

	// Find which period contains the target date by checking period bounds
	period := uint64(1)
	for {
		periodStart, periodEnd := h.PeriodBounds(period)

		// Check if targetDate falls within this period (inclusive on both ends)
		if !targetDate.Before(periodStart) && !targetDate.After(periodEnd) {
			return period, nil
		}

		period++
		if period > 1000 { // prevent infinite loop
			return period, nil
		}
	}
}

// daysInRange counts days between two dates (inclusive), properly handling leap years
func daysInRange(start, end time.Time) int {
	return int(end.Sub(start).Hours()/24) + 1 // +1 because both start and end are inclusive
}

// daysSinceStart counts days from start to target
func daysSinceStart(start, target time.Time) int {
	days := int(target.Sub(start).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// TotalDistributableAt calculates the maximum tokens that can be distributed by a given date
func (h *HalvingSchedule) TotalDistributableAt(targetDate time.Time) (math.Int, error) {
	currentPeriod, err := h.CurrentPeriod(targetDate)
	if err != nil {
		return math.ZeroInt(), err
	}

	total := math.ZeroInt()

	// Add full allocations from all completed periods
	for period := uint64(1); period < currentPeriod; period++ {
		total = total.Add(h.PeriodAllocation(period))
	}

	// Add pro-rata allocation from current period
	periodStart, periodEnd := h.PeriodBounds(currentPeriod)
	daysInPeriod := daysInRange(periodStart, periodEnd)
	daysElapsed := daysSinceStart(periodStart, targetDate) + 1 // +1 because current day counts

	if daysInPeriod > 0 {
		periodAllocation := h.PeriodAllocation(currentPeriod)
		// Pro-rata: (allocation * daysElapsed) / daysInPeriod
		currentPeriodAmount := periodAllocation.Mul(math.NewInt(int64(daysElapsed))).Quo(math.NewInt(int64(daysInPeriod)))
		total = total.Add(currentPeriodAmount)
	}

	return total, nil
}
