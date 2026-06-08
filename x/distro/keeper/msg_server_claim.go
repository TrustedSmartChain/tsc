package keeper

import (
	"context"
	"strconv"

	"cosmossdk.io/collections"
	errorsmod "cosmossdk.io/errors"
	"cosmossdk.io/math"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// Claim mints and pays out a reward from a finalized (live) epoch distribution.
// The leaf is reconstructed from (nonce, address, amount) and verified against
// the canonical root. Funds are minted on demand and sent to the reward
// address. A given (epoch, nonce) can be claimed at most once.
func (ms msgServer) Claim(goCtx context.Context, msg *types.MsgClaim) (*types.MsgClaimResponse, error) {
	ctx := sdk.UnwrapSDKContext(goCtx)

	if _, err := sdk.AccAddressFromBech32(msg.Claimer); err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid claimer address")
	}
	recipient, err := sdk.AccAddressFromBech32(msg.Address)
	if err != nil {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidAddress, "invalid reward address")
	}
	amount, ok := math.NewIntFromString(msg.Amount)
	if !ok {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "amount is not a valid integer")
	}
	if !amount.IsPositive() {
		return nil, errorsmod.Wrap(sdkerrors.ErrInvalidRequest, "amount must be positive")
	}

	params, err := ms.k.Params.Get(ctx)
	if err != nil {
		return nil, err
	}

	// The epoch must have reached consensus.
	ed, err := ms.k.EpochDistributions.Get(ctx, msg.Epoch)
	if err != nil {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "no distribution for epoch %d", msg.Epoch)
	}
	if ed.Status != types.DISTRIBUTION_STATUS_LIVE {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "epoch %d is not live", msg.Epoch)
	}

	// Reject double claims.
	claimedKey := collections.Join(msg.Epoch, msg.Nonce)
	if claimed, err := ms.k.Claimed.Has(ctx, claimedKey); err != nil {
		return nil, err
	} else if claimed {
		return nil, errorsmod.Wrapf(sdkerrors.ErrInvalidRequest, "reward %d for epoch %d already claimed", msg.Nonce, msg.Epoch)
	}

	// Verify the merkle proof against the canonical root.
	leaf := types.LeafHash(msg.Nonce, msg.Address, msg.Amount)
	if !types.VerifyProof(ed.MerkleRoot, leaf, msg.Proof) {
		return nil, errorsmod.Wrap(sdkerrors.ErrUnauthorized, "invalid merkle proof")
	}

	// Enforce the per-epoch emission budget derived from the halving schedule:
	// the cumulative amount claimed for this epoch may not exceed the epoch's
	// allocation. This bounds on-demand minting to the emission curve so a
	// finalized root cannot mint beyond the day's budget.
	budget, err := epochBudget(params, msg.Epoch)
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
			"claim would exceed epoch %d budget: claimed %s + %s > %s", msg.Epoch, claimed, amount, budget)
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
	if err := ms.k.EpochDistributions.Set(ctx, msg.Epoch, ed); err != nil {
		return nil, err
	}

	ctx.EventManager().EmitEvent(sdk.NewEvent(
		types.EventTypeClaim,
		sdk.NewAttribute(types.AttributeKeyEpoch, strconv.FormatInt(msg.Epoch, 10)),
		sdk.NewAttribute(types.AttributeKeyNonce, strconv.FormatUint(msg.Nonce, 10)),
		sdk.NewAttribute(types.AttributeKeyAddress, msg.Address),
		sdk.NewAttribute(types.AttributeKeyAmount, msg.Amount),
	))

	return &types.MsgClaimResponse{}, nil
}
