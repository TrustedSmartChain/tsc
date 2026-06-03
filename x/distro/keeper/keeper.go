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

	// Votes holds the raw per-signer submitted roots keyed by (epoch, signer).
	// These are retained (never pruned) for audit.
	Votes collections.Map[collections.Pair[int64, string], types.DistributionVote]
	// EpochDistributions holds the canonical/finalized distribution per epoch.
	EpochDistributions collections.Map[int64, types.EpochDistribution]
	// Claimed is the set of claimed reward nonces keyed by (epoch, nonce).
	Claimed collections.KeySet[collections.Pair[int64, uint64]]

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
			collections.PairKeyCodec(collections.Int64Key, collections.StringKey),
			codec.CollValue[types.DistributionVote](cdc),
		),
		EpochDistributions: collections.NewMap(
			sb, types.EpochDistributionsKeyPrefix, "epoch_distributions",
			collections.Int64Key,
			codec.CollValue[types.EpochDistribution](cdc),
		),
		Claimed: collections.NewKeySet(
			sb, types.ClaimedKeyPrefix, "claimed",
			collections.PairKeyCodec(collections.Int64Key, collections.Uint64Key),
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
		if err := k.Votes.Set(ctx, collections.Join(v.Epoch, v.Signer), v); err != nil {
			return err
		}
	}

	for _, ed := range data.EpochDistributions {
		if err := k.EpochDistributions.Set(ctx, ed.Epoch, ed); err != nil {
			return err
		}
	}

	for _, cr := range data.ClaimedRewards {
		for _, nonce := range cr.Nonces {
			if err := k.Claimed.Set(ctx, collections.Join(cr.Epoch, nonce)); err != nil {
				return err
			}
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
	if err := k.Votes.Walk(ctx, nil, func(_ collections.Pair[int64, string], v types.DistributionVote) (bool, error) {
		votes = append(votes, v)
		return false, nil
	}); err != nil {
		panic(err)
	}

	var epochDistributions []types.EpochDistribution
	if err := k.EpochDistributions.Walk(ctx, nil, func(_ int64, ed types.EpochDistribution) (bool, error) {
		epochDistributions = append(epochDistributions, ed)
		return false, nil
	}); err != nil {
		panic(err)
	}

	// Collect claimed nonces grouped by epoch, preserving ascending key order.
	claimedByEpoch := map[int64][]uint64{}
	var claimedEpochOrder []int64
	if err := k.Claimed.Walk(ctx, nil, func(key collections.Pair[int64, uint64]) (bool, error) {
		epoch := key.K1()
		if _, ok := claimedByEpoch[epoch]; !ok {
			claimedEpochOrder = append(claimedEpochOrder, epoch)
		}
		claimedByEpoch[epoch] = append(claimedByEpoch[epoch], key.K2())
		return false, nil
	}); err != nil {
		panic(err)
	}
	claimedRewards := make([]types.ClaimedReward, 0, len(claimedEpochOrder))
	for _, epoch := range claimedEpochOrder {
		claimedRewards = append(claimedRewards, types.ClaimedReward{Epoch: epoch, Nonces: claimedByEpoch[epoch]})
	}

	return &types.GenesisState{
		Params:             params,
		Votes:              votes,
		EpochDistributions: epochDistributions,
		ClaimedRewards:     claimedRewards,
	}
}
