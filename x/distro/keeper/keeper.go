package keeper

import (
	"context"

	"github.com/cosmos/cosmos-sdk/codec"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	"cosmossdk.io/collections"
	storetypes "cosmossdk.io/core/store"
	"cosmossdk.io/log"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

type Keeper struct {
	cdc codec.BinaryCodec

	logger log.Logger

	// state management
	Schema collections.Schema
	Params collections.Item[types.Params]

	// Votes holds the raw per-signer submitted roots keyed by (date, signer).
	// These are retained (never pruned) for audit.
	Votes collections.Map[collections.Pair[string, string], types.DistributionVote]
	// Distributions holds the canonical/finalized distribution per day (date).
	Distributions collections.Map[string, types.Distribution]
	// Claimed is the set of claimed reward nonces keyed by (date, nonce).
	Claimed collections.KeySet[collections.Pair[string, uint64]]
	// ClaimTotals accumulates the claimed amount per (date, category).
	ClaimTotals collections.Map[collections.Pair[string, string], types.CategoryClaimTotal]
	// ActiveDistributions is the set of dates whose distribution is non-terminal
	// (VOTING/PENDING/UNDER_REVIEW). The epoch hook iterates this set instead of
	// scanning every distribution ever created. Maintained incrementally: a date
	// is added when it first opens for voting (or is revived) and removed when it
	// reaches a terminal state (LIVE/EXPIRED).
	ActiveDistributions collections.KeySet[string]

	authority string

	accountKeeper  types.AccountKeeper
	bankKeeper     types.BankKeeper
	stakingKeeper  types.StakingKeeper
	licensesKeeper types.LicensesKeeper
	epochsKeeper   types.EpochsKeeper
}

// NewKeeper creates a new Keeper instance
func NewKeeper(
	cdc codec.BinaryCodec,
	storeService storetypes.KVStoreService,
	logger log.Logger,
	authority string,
	accountKeeper types.AccountKeeper,
	bankKeeper types.BankKeeper,
	stakingKeeper types.StakingKeeper,
	licensesKeeper types.LicensesKeeper,
	epochsKeeper types.EpochsKeeper,
) Keeper {
	logger = logger.With(log.ModuleKey, "x/"+types.ModuleName)

	sb := collections.NewSchemaBuilder(storeService)

	if authority == "" {
		authority = authtypes.NewModuleAddress(govtypes.ModuleName).String()
	}

	k := Keeper{
		cdc:    cdc,
		logger: logger,

		Params: collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		Votes: collections.NewMap(
			sb, types.VotesKeyPrefix, "votes",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
			codec.CollValue[types.DistributionVote](cdc),
		),
		Distributions: collections.NewMap(
			sb, types.DistributionsKeyPrefix, "distributions",
			collections.StringKey,
			codec.CollValue[types.Distribution](cdc),
		),
		Claimed: collections.NewKeySet(
			sb, types.ClaimedKeyPrefix, "claimed",
			collections.PairKeyCodec(collections.StringKey, collections.Uint64Key),
		),
		ClaimTotals: collections.NewMap(
			sb, types.ClaimTotalsKeyPrefix, "claim_totals",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
			codec.CollValue[types.CategoryClaimTotal](cdc),
		),
		ActiveDistributions: collections.NewKeySet(
			sb, types.ActiveDistributionsKeyPrefix, "active_distributions",
			collections.StringKey,
		),

		authority:      authority,
		accountKeeper:  accountKeeper,
		bankKeeper:     bankKeeper,
		stakingKeeper:  stakingKeeper,
		licensesKeeper: licensesKeeper,
		epochsKeeper:   epochsKeeper,
	}

	schema, err := sb.Build()
	if err != nil {
		panic(err)
	}

	k.Schema = schema

	return k
}

func (k Keeper) Logger() log.Logger {
	return k.logger
}

// InitGenesis initializes the module's state from a genesis state.
func (k *Keeper) InitGenesis(ctx context.Context, data *types.GenesisState) error {

	if err := data.Params.Validate(); err != nil {
		return err
	}

	if err := k.Params.Set(ctx, data.Params); err != nil {
		return err
	}

	for _, v := range data.Votes {
		if err := k.Votes.Set(ctx, collections.Join(v.Date, v.Signer), v); err != nil {
			return err
		}
	}

	for _, d := range data.Distributions {
		if err := k.Distributions.Set(ctx, d.Date, d); err != nil {
			return err
		}
		// Index non-terminal days so the epoch hook can find them. The active set
		// is derived state, so it is not part of GenesisState.
		if isActiveStatus(d.Status) {
			if err := k.ActiveDistributions.Set(ctx, d.Date); err != nil {
				return err
			}
		}
	}

	for _, cr := range data.ClaimedRewards {
		for _, nonce := range cr.Nonces {
			if err := k.Claimed.Set(ctx, collections.Join(cr.Date, nonce)); err != nil {
				return err
			}
		}
	}

	for _, ct := range data.CategoryClaimTotals {
		if err := k.ClaimTotals.Set(ctx, collections.Join(ct.Date, ct.Category), ct); err != nil {
			return err
		}
	}

	return nil
}

// ExportGenesis exports the module's state to a genesis state.
func (k *Keeper) ExportGenesis(ctx context.Context) *types.GenesisState {
	params, err := k.Params.Get(ctx)
	if err != nil {
		panic(err)
	}

	var votes []types.DistributionVote
	if err := k.Votes.Walk(ctx, nil, func(_ collections.Pair[string, string], v types.DistributionVote) (bool, error) {
		votes = append(votes, v)
		return false, nil
	}); err != nil {
		panic(err)
	}

	var distributions []types.Distribution
	if err := k.Distributions.Walk(ctx, nil, func(_ string, d types.Distribution) (bool, error) {
		distributions = append(distributions, d)
		return false, nil
	}); err != nil {
		panic(err)
	}

	// Collect claimed nonces grouped by date, preserving ascending key order.
	claimedByDate := map[string][]uint64{}
	var claimedDateOrder []string
	if err := k.Claimed.Walk(ctx, nil, func(key collections.Pair[string, uint64]) (bool, error) {
		date := key.K1()
		if _, ok := claimedByDate[date]; !ok {
			claimedDateOrder = append(claimedDateOrder, date)
		}
		claimedByDate[date] = append(claimedByDate[date], key.K2())
		return false, nil
	}); err != nil {
		panic(err)
	}
	claimedRewards := make([]types.ClaimedReward, 0, len(claimedDateOrder))
	for _, date := range claimedDateOrder {
		claimedRewards = append(claimedRewards, types.ClaimedReward{Date: date, Nonces: claimedByDate[date]})
	}

	var categoryClaimTotals []types.CategoryClaimTotal
	if err := k.ClaimTotals.Walk(ctx, nil, func(_ collections.Pair[string, string], ct types.CategoryClaimTotal) (bool, error) {
		categoryClaimTotals = append(categoryClaimTotals, ct)
		return false, nil
	}); err != nil {
		panic(err)
	}

	return &types.GenesisState{
		Params:              params,
		Votes:               votes,
		Distributions:       distributions,
		ClaimedRewards:      claimedRewards,
		CategoryClaimTotals: categoryClaimTotals,
	}
}

// isActiveStatus reports whether a distribution status is non-terminal and so
// still needs processing by the epoch hook.
func isActiveStatus(s types.DistributionStatus) bool {
	switch s {
	case types.DISTRIBUTION_STATUS_VOTING,
		types.DISTRIBUTION_STATUS_PENDING,
		types.DISTRIBUTION_STATUS_UNDER_REVIEW:
		return true
	default:
		return false
	}
}

// RebuildActiveDistributions repopulates the active-distribution index from the
// Distributions map. New days maintain the index incrementally; this is for the
// one-time backfill at the upgrade that introduces the index, so any pre-existing
// non-terminal days are picked up by the epoch hook.
func (k *Keeper) RebuildActiveDistributions(ctx context.Context) error {
	return k.Distributions.Walk(ctx, nil, func(date string, d types.Distribution) (bool, error) {
		if isActiveStatus(d.Status) {
			if err := k.ActiveDistributions.Set(ctx, date); err != nil {
				return true, err
			}
		}
		return false, nil
	})
}
