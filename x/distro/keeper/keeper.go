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

	// Votes holds the raw per-signer submitted roots keyed by
	// (date, distro_type, signer). These are retained (never pruned) for audit.
	Votes collections.Map[collections.Triple[string, string, string], types.DistributionVote]
	// Distributions holds the canonical/finalized distribution per (date, type).
	Distributions collections.Map[collections.Pair[string, string], types.Distribution]
	// Claimed is the set of claimed reward nonces keyed by (date, type, nonce).
	Claimed collections.KeySet[collections.Triple[string, string, uint64]]
	// ClaimTotals accumulates the claimed amount per (date, type, category).
	ClaimTotals collections.Map[collections.Triple[string, string, string], types.CategoryClaimTotal]
	// ActiveDistributions is the set of (date, type) pairs whose distribution is
	// non-terminal (VOTING/PENDING/UNDER_REVIEW). The epoch hook iterates this set
	// instead of scanning every distribution ever created. Maintained
	// incrementally: a pair is added when it first opens for voting (or is
	// revived) and removed when it reaches a terminal state (LIVE/EXPIRED).
	ActiveDistributions collections.KeySet[collections.Pair[string, string]]

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
			collections.TripleKeyCodec(collections.StringKey, collections.StringKey, collections.StringKey),
			codec.CollValue[types.DistributionVote](cdc),
		),
		Distributions: collections.NewMap(
			sb, types.DistributionsKeyPrefix, "distributions",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
			codec.CollValue[types.Distribution](cdc),
		),
		Claimed: collections.NewKeySet(
			sb, types.ClaimedKeyPrefix, "claimed",
			collections.TripleKeyCodec(collections.StringKey, collections.StringKey, collections.Uint64Key),
		),
		ClaimTotals: collections.NewMap(
			sb, types.ClaimTotalsKeyPrefix, "claim_totals",
			collections.TripleKeyCodec(collections.StringKey, collections.StringKey, collections.StringKey),
			codec.CollValue[types.CategoryClaimTotal](cdc),
		),
		ActiveDistributions: collections.NewKeySet(
			sb, types.ActiveDistributionsKeyPrefix, "active_distributions",
			collections.PairKeyCodec(collections.StringKey, collections.StringKey),
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
		if err := k.Votes.Set(ctx, collections.Join3(v.Date, v.DistroType, v.Signer), v); err != nil {
			return err
		}
	}

	for _, d := range data.Distributions {
		key := collections.Join(d.Date, d.DistroType)
		if err := k.Distributions.Set(ctx, key, d); err != nil {
			return err
		}
		// Index non-terminal days so the epoch hook can find them. The active set
		// is derived state, so it is not part of GenesisState.
		if isActiveStatus(d.Status) {
			if err := k.ActiveDistributions.Set(ctx, key); err != nil {
				return err
			}
		}
	}

	for _, cr := range data.ClaimedRewards {
		for _, nonce := range cr.Nonces {
			if err := k.Claimed.Set(ctx, collections.Join3(cr.Date, cr.DistroType, nonce)); err != nil {
				return err
			}
		}
	}

	for _, ct := range data.CategoryClaimTotals {
		if err := k.ClaimTotals.Set(ctx, collections.Join3(ct.Date, ct.DistroType, ct.Category), ct); err != nil {
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
	if err := k.Votes.Walk(ctx, nil, func(_ collections.Triple[string, string, string], v types.DistributionVote) (bool, error) {
		votes = append(votes, v)
		return false, nil
	}); err != nil {
		panic(err)
	}

	var distributions []types.Distribution
	if err := k.Distributions.Walk(ctx, nil, func(_ collections.Pair[string, string], d types.Distribution) (bool, error) {
		distributions = append(distributions, d)
		return false, nil
	}); err != nil {
		panic(err)
	}

	// Collect claimed nonces grouped by (date, type), preserving ascending key
	// order. The group key joins date and type with a NUL separator (neither
	// contains NUL) so distinct (date, type) pairs never collide.
	type dateType struct{ date, distroType string }
	claimedByKey := map[string][]uint64{}
	var claimedOrder []dateType
	if err := k.Claimed.Walk(ctx, nil, func(key collections.Triple[string, string, uint64]) (bool, error) {
		gk := key.K1() + "\x00" + key.K2()
		if _, ok := claimedByKey[gk]; !ok {
			claimedOrder = append(claimedOrder, dateType{date: key.K1(), distroType: key.K2()})
		}
		claimedByKey[gk] = append(claimedByKey[gk], key.K3())
		return false, nil
	}); err != nil {
		panic(err)
	}
	claimedRewards := make([]types.ClaimedReward, 0, len(claimedOrder))
	for _, dt := range claimedOrder {
		claimedRewards = append(claimedRewards, types.ClaimedReward{
			Date:       dt.date,
			DistroType: dt.distroType,
			Nonces:     claimedByKey[dt.date+"\x00"+dt.distroType],
		})
	}

	var categoryClaimTotals []types.CategoryClaimTotal
	if err := k.ClaimTotals.Walk(ctx, nil, func(_ collections.Triple[string, string, string], ct types.CategoryClaimTotal) (bool, error) {
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
	return k.Distributions.Walk(ctx, nil, func(key collections.Pair[string, string], d types.Distribution) (bool, error) {
		if isActiveStatus(d.Status) {
			if err := k.ActiveDistributions.Set(ctx, key); err != nil {
				return true, err
			}
		}
		return false, nil
	})
}
