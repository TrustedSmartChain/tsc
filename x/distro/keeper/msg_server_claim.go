package keeper

import (
	"context"
	"errors"
	"strconv"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// Claim mints and pays out a single (category, amount) reward from a finalized
// (live) (day, type) distribution. The leaf is reconstructed from
// (nonce, address, category, amount, denom) and verified against the canonical
// root. Funds are minted on demand and sent to the reward address (the earner);
// the claimer signs the tx but need not be the earner. A given
// (date, type, nonce) can be claimed at most once.
func (ms msgServer) Claim(goCtx context.Context, msg *types.MsgClaim) (*types.MsgClaimResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if _, err := sdk.AccAddressFromBech32(msg.Claimer); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid claimer address")
	}
	recipient, err := sdk.AccAddressFromBech32(msg.Address)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid reward address")
	}
	if msg.Category == "" {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "category cannot be empty")
	}
	// amount is the value minted and budget-checked for this claim.
	amount, err := types.ValidateClaimAmount(msg.Amount)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	}

	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	// The reward denom must be the module denom; budget and supply are tracked in it.
	if msg.Denom != params.Denom {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "denom %q must be the module denom %q", msg.Denom, params.Denom)
	}

	// The claim must target a configured distribution type and category.
	distroType, ok := params.FindDistributionType(msg.DistroType)
	if !ok {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "unknown distribution type %q", msg.DistroType)
	}
	if _, ok := distroType.FindCategory(msg.Category); !ok {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "category %q is not configured for distribution type %q", msg.Category, msg.DistroType)
	}

	// The (day, type) must have reached consensus.
	distKey := collections.Join(msg.Date, msg.DistroType)
	ed, err := ms.k.Distributions.Get(ctx, distKey)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "no distribution for date %q type %q", msg.Date, msg.DistroType)
	}
	if ed.Status != types.DISTRIBUTION_STATUS_LIVE {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "date %q type %q is not live", msg.Date, msg.DistroType)
	}

	// Reject double claims.
	claimedKey := collections.Join3(msg.Date, msg.DistroType, msg.Nonce)
	if claimed, err := ms.k.Claimed.Has(ctx, claimedKey); err != nil {
		return nil, err
	} else if claimed {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "reward %d for date %q type %q already claimed", msg.Nonce, msg.Date, msg.DistroType)
	}

	// Bound the proof length: a valid proof is at most the tree depth, so an
	// over-long proof is malformed and its (un-gas-metered) hash folding should
	// not be performed.
	if len(msg.Proof) > types.MaxProofDepth {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "merkle proof too long: %d exceeds max depth %d", len(msg.Proof), types.MaxProofDepth)
	}

	// Verify the merkle proof against the canonical root. The leaf commits to
	// exactly (nonce, date, distro_type, address, category, amount, denom).
	leaf := types.LeafHash(msg.Nonce, msg.Date, msg.DistroType, msg.Address, msg.Category, msg.Amount, msg.Denom)
	if !types.VerifyProof(ed.MerkleRoot, leaf, msg.Proof) {
		return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "invalid merkle proof")
	}

	// Cap the claim by the stored header totals. The header (captured at consensus,
	// and validated against the type/day budget at submission) records the
	// authoritative per-category totals this distribution pays out for the day, so
	// claims need only respect it rather than re-deriving the budget. A LIVE day
	// with no header (e.g. one seeded directly via genesis) falls back to the
	// budget-derived caps.
	dayBudget, err := dateBudget(params, msg.Date)
	if err != nil {
		return nil, err
	}
	typeBudget := types.TypeBudget(dayBudget, distroType.Percentage)
	headerTotals, headerSum, hasHeader := storedHeaderTotals(ed, typeBudget)

	// Type-level cap: the cumulative amount claimed across all categories may not
	// exceed the type's distributable total.
	claimed, ok := math.NewIntFromString(ed.ClaimedAmount)
	if !ok || ed.ClaimedAmount == "" {
		claimed = math.ZeroInt()
	}
	newClaimed := claimed.Add(amount)
	if newClaimed.GT(headerSum) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest,
			"claim would exceed date %q type %q distributable total: claimed %s + %s > %s", msg.Date, msg.DistroType, claimed, amount, headerSum)
	}

	// Per-category cap: this category's cumulative claimed total may not exceed the
	// header's declared total for it (or, with no header, the configured category
	// cap, or the type total when the type does not cap categories).
	catKey := collections.Join3(msg.Date, msg.DistroType, msg.Category)
	catRunning := amount
	if existing, err := ms.k.ClaimTotals.Get(ctx, catKey); err == nil {
		prev, ok := math.NewIntFromString(existing.Total)
		if !ok {
			prev = math.ZeroInt()
		}
		catRunning = prev.Add(amount)
	} else if !errors.Is(err, collections.ErrNotFound) {
		return nil, err
	}
	categoryCap := headerSum
	switch {
	case hasHeader:
		c, ok := headerTotals[msg.Category]
		if !ok {
			c = math.ZeroInt()
		}
		categoryCap = c
	case distroType.HasCategoryPercentages():
		cat, _ := distroType.FindCategory(msg.Category) // existence checked above
		categoryCap = types.CategoryCap(typeBudget, cat.Percentage)
	}
	if catRunning.GT(categoryCap) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest,
			"claim would exceed category %q cap for date %q type %q: %s > %s", msg.Category, msg.Date, msg.DistroType, catRunning, categoryCap)
	}

	// Mint on demand, respecting max supply as a final safety bound.
	supply := ms.k.bankKeeper.GetSupply(ctx, params.Denom)
	maxSupply, ok := math.NewIntFromString(params.MaxSupply)
	if !ok {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "invalid max supply")
	}
	if supply.Amount.Add(amount).GT(maxSupply) {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "max supply exceeded")
	}

	coins := sdk.NewCoins(sdk.NewCoin(params.Denom, amount))
	if err := ms.k.bankKeeper.MintCoins(ctx, types.ModuleName, coins); err != nil {
		return nil, err
	}
	if err := ms.k.bankKeeper.SendCoinsFromModuleToAccount(ctx, types.ModuleName, recipient, coins); err != nil {
		return nil, err
	}

	// Record the claimed nonce and the updated type- and category-level totals.
	if err := ms.k.Claimed.Set(ctx, claimedKey); err != nil {
		return nil, err
	}
	ed.ClaimedAmount = newClaimed.String()
	if err := ms.k.Distributions.Set(ctx, distKey, ed); err != nil {
		return nil, err
	}
	if err := ms.k.ClaimTotals.Set(ctx, catKey, types.CategoryClaimTotal{
		Date:       msg.Date,
		Category:   msg.Category,
		Total:      catRunning.String(),
		DistroType: msg.DistroType,
	}); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeClaim,
		sdk.NewAttribute(types.AttributeKeyDate, msg.Date),
		sdk.NewAttribute(types.AttributeKeyDistroType, msg.DistroType),
		sdk.NewAttribute(types.AttributeKeyNonce, strconv.FormatUint(msg.Nonce, 10)),
		sdk.NewAttribute(types.AttributeKeyAddress, msg.Address),
		sdk.NewAttribute(types.AttributeKeyCategory, msg.Category),
		sdk.NewAttribute(types.AttributeKeyAmount, msg.Amount),
	))

	return &types.MsgClaimResponse{}, nil
}

// storedHeaderTotals returns the per-category claim caps and their sum from a
// distribution's stored header (captured at consensus and already validated
// against the type/day budget at submission). The bool reports whether a header
// was present; when it is not (e.g. a LIVE day seeded directly via genesis) it
// returns (nil, typeBudget, false) so callers fall back to the budget caps.
func storedHeaderTotals(ed types.Distribution, typeBudget math.Int) (map[string]math.Int, math.Int, bool) {
	if ed.Header == nil || len(ed.Header.TotalsByCategory) == 0 {
		return nil, typeBudget, false
	}
	totals := make(map[string]math.Int, len(ed.Header.TotalsByCategory))
	sum := math.ZeroInt()
	for category, raw := range ed.Header.TotalsByCategory {
		v, ok := math.NewIntFromString(raw)
		if !ok {
			v = math.ZeroInt()
		}
		totals[category] = v
		sum = sum.Add(v)
	}
	return totals, sum, true
}
