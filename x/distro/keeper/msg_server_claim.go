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

// Claim mints and pays out a reward from a finalized (live) day's distribution.
// The leaf is reconstructed from (nonce, address, total, categories) and verified
// against the canonical root. Funds are minted on demand and sent to the reward
// address. A given (date, nonce) can be claimed at most once.
func (ms msgServer) Claim(goCtx context.Context, msg *types.MsgClaim) (*types.MsgClaimResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if _, err := sdk.AccAddressFromBech32(msg.Claimer); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid claimer address")
	}
	recipient, err := sdk.AccAddressFromBech32(msg.Address)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid reward address")
	}
	// Validate total/categories consistency (positive total, non-empty bounded
	// categories each positive, summing to total). The merkle leaf commits to this
	// map, so the canonical root already pins the expected breakdown — this rejects
	// internally inconsistent claims before proof verification. Shared with
	// MsgClaim.ValidateBasic so the two cannot drift.
	total, err := types.ValidateClaimAmounts(msg.Total, msg.Categories)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, err.Error())
	}

	// amount is the value minted and budget-checked for this claim.
	amount := total

	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	// The day must have reached consensus.
	ed, err := ms.k.Distributions.Get(ctx, msg.Date)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "no distribution for date %q", msg.Date)
	}
	if ed.Status != types.DISTRIBUTION_STATUS_LIVE {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "date %q is not live", msg.Date)
	}

	// Reject double claims.
	claimedKey := collections.Join(msg.Date, msg.Nonce)
	if claimed, err := ms.k.Claimed.Has(ctx, claimedKey); err != nil {
		return nil, err
	} else if claimed {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "reward %d for date %q already claimed", msg.Nonce, msg.Date)
	}

	// Bound the proof length: a valid proof is at most the tree depth, so an
	// over-long proof is malformed and its (un-gas-metered) hash folding should
	// not be performed.
	if len(msg.Proof) > types.MaxProofDepth {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "merkle proof too long: %d exceeds max depth %d", len(msg.Proof), types.MaxProofDepth)
	}

	// Verify the merkle proof against the canonical root.
	leaf := types.LeafHash(msg.Nonce, msg.Address, msg.Total, msg.Categories)
	if !types.VerifyProof(ed.MerkleRoot, leaf, msg.Proof) {
		return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "invalid merkle proof")
	}

	// Enforce the per-day emission budget derived from the halving schedule: the
	// cumulative amount claimed for this day may not exceed the day's allocation.
	// This bounds on-demand minting to the emission curve so a finalized root
	// cannot mint beyond the day's budget.
	budget, err := dateBudget(params, msg.Date)
	if err != nil {
		return nil, err
	}
	claimed, ok := math.NewIntFromString(ed.ClaimedAmount)
	if !ok || ed.ClaimedAmount == "" {
		claimed = math.ZeroInt()
	}
	newClaimed := claimed.Add(amount)
	if newClaimed.GT(budget) {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest,
			"claim would exceed date %q budget: claimed %s + %s > %s", msg.Date, claimed, amount, budget)
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

	// Record the claimed nonce and the updated per-epoch claimed total.
	if err := ms.k.Claimed.Set(ctx, claimedKey); err != nil {
		return nil, err
	}
	ed.ClaimedAmount = newClaimed.String()
	if err := ms.k.Distributions.Set(ctx, msg.Date, ed); err != nil {
		return nil, err
	}

	// Accumulate the per-category claimed totals for this day so they can be
	// queried via Query/ClaimTotalByCategory.
	for category, raw := range msg.Categories {
		catAmount, _ := math.NewIntFromString(raw) // already validated above
		key := collections.Join(msg.Date, category)
		running := catAmount
		if existing, err := ms.k.ClaimTotals.Get(ctx, key); err == nil {
			prev, ok := math.NewIntFromString(existing.Total)
			if !ok {
				prev = math.ZeroInt()
			}
			running = prev.Add(catAmount)
		} else if !errors.Is(err, collections.ErrNotFound) {
			return nil, err
		}
		if err := ms.k.ClaimTotals.Set(ctx, key, types.CategoryClaimTotal{
			Date:     msg.Date,
			Category: category,
			Total:    running.String(),
		}); err != nil {
			return nil, err
		}
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeClaim,
		sdk.NewAttribute(types.AttributeKeyDate, msg.Date),
		sdk.NewAttribute(types.AttributeKeyNonce, strconv.FormatUint(msg.Nonce, 10)),
		sdk.NewAttribute(types.AttributeKeyAddress, msg.Address),
		sdk.NewAttribute(types.AttributeKeyAmount, msg.Total),
	))

	return &types.MsgClaimResponse{}, nil
}
