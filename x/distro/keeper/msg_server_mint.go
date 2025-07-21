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

func validateMintingLimits(ctx sdk.Context, currentSupply math.Int, amount math.Int, params types.Params) error {
	startDate, err := parseDate(params.DistributionStartDate)
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid distribution start date: %v", err)
	}
	targetDate, err := parseDate(ctx.BlockTime().Format("2006-01-02"))
	if err != nil {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "invalid target date: %v", err)
	}

	months := monthsBetween(startDate, targetDate)
	if months < 0 {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "target date is before start date")
	}

	currentHalvingPeriod := 1 + uint64(months)/params.MonthsInHalvingPeriod
	var totalDistributable math.Int = math.ZeroInt()

	for period := uint64(1); period < currentHalvingPeriod; period++ {
		periodYearlyLimit, _ := math.NewIntFromString(params.MaxSupply)
		periodYearlyLimit = periodYearlyLimit.Quo(math.NewIntFromUint64(1 << (period - 1))).Quo(math.NewIntFromUint64(2))
		totalDistributable = totalDistributable.Add(periodYearlyLimit)
	}

	if currentHalvingPeriod > 0 {
		periodStart := startDate.AddDate(0, int((currentHalvingPeriod-1)*params.MonthsInHalvingPeriod), 0)
		periodEnd := startDate.AddDate(0, int(currentHalvingPeriod*params.MonthsInHalvingPeriod), -1)

		daysInPeriod := math.NewIntFromUint64(uint64(periodEnd.Sub(periodStart).Hours()/24) + 1)
		periodYearlyLimit, _ := math.NewIntFromString(params.MaxSupply)
		periodYearlyLimit = periodYearlyLimit.Quo(math.NewIntFromUint64(1 << (currentHalvingPeriod - 1))).Quo(math.NewIntFromUint64(2))

		daysElapsed := math.NewIntFromUint64(uint64(targetDate.Sub(periodStart).Hours() / 24))
		if daysElapsed.GT(daysInPeriod) {
			daysElapsed = daysInPeriod
		}

		if !daysInPeriod.IsZero() {
			currentPeriodAmount := periodYearlyLimit.Mul(daysElapsed).Quo(daysInPeriod)
			totalDistributable = totalDistributable.Add(currentPeriodAmount)
		}
	}

	if amount.Add(currentSupply).GT(totalDistributable) {
		return errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "amount exceeds total distributable limit")
	}

	return nil
}

func monthsBetween(start, end time.Time) int {
	if end.Before(start) {
		return -1
	}

	years := end.Year() - start.Year()
	months := years*12 + int(end.Month()) - int(start.Month())

	// Adjust for day of month
	if end.Day() < start.Day() {
		months--
	}

	// Handle edge case where end is exactly on start's day but in a prior month
	if months < 0 {
		return 0
	}
	return months
}
