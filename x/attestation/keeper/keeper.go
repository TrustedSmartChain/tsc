package keeper

import (
	"context"

	"github.com/cosmos/cosmos-sdk/codec"

	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
	govtypes "github.com/cosmos/cosmos-sdk/x/gov/types"

	"cosmossdk.io/collections"
	storetypes "cosmossdk.io/core/store"
	"cosmossdk.io/log"

	"github.com/TrustedSmartChain/tsc/v3/x/attestation/types"
)

type Keeper struct {
	cdc codec.BinaryCodec

	logger log.Logger

	// state management
	Schema collections.Schema
	Params collections.Item[types.Params]

	// RwaCounters and RwuCounters are the per-node rolling daily attestation
	// counters, incremented by ante admission and re-read by the handlers.
	// Attestation payloads themselves are never stored: they live in txs and
	// events.
	RwaCounters collections.Map[string, types.ActivityCounter]
	RwuCounters collections.Map[string, types.ActivityCounter]

	authority string

	networkKeeper types.NetworkKeeper
	licenseKeeper types.LicenseKeeper
}

// NewKeeper creates a new Keeper instance
func NewKeeper(
	cdc codec.BinaryCodec,
	storeService storetypes.KVStoreService,
	logger log.Logger,
	authority string,
	networkKeeper types.NetworkKeeper,
	licenseKeeper types.LicenseKeeper,
) Keeper {
	logger = logger.With(log.ModuleKey, "x/"+types.ModuleName)

	sb := collections.NewSchemaBuilder(storeService)

	if authority == "" {
		authority = authtypes.NewModuleAddress(govtypes.ModuleName).String()
	}

	k := Keeper{
		cdc:    cdc,
		logger: logger,

		Params:      collections.NewItem(sb, types.ParamsKey, "params", codec.CollValue[types.Params](cdc)),
		RwaCounters: collections.NewMap(sb, types.RwaCountersKey, "rwa_counters", collections.StringKey, codec.CollValue[types.ActivityCounter](cdc)),
		RwuCounters: collections.NewMap(sb, types.RwuCountersKey, "rwu_counters", collections.StringKey, codec.CollValue[types.ActivityCounter](cdc)),

		authority: authority,

		networkKeeper: networkKeeper,
		licenseKeeper: licenseKeeper,
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

func (k Keeper) GetAuthority() string {
	return k.authority
}

// InitGenesis initializes the module's state from a genesis state.
func (k *Keeper) InitGenesis(ctx context.Context, data *types.GenesisState) error {
	if err := data.Validate(); err != nil {
		return err
	}

	if err := k.Params.Set(ctx, data.Params); err != nil {
		return err
	}
	for _, c := range data.RwaCounters {
		if err := k.RwaCounters.Set(ctx, c.NodeAddress, c.Counter); err != nil {
			return err
		}
	}
	for _, c := range data.RwuCounters {
		if err := k.RwuCounters.Set(ctx, c.NodeAddress, c.Counter); err != nil {
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

	exportCounters := func(m collections.Map[string, types.ActivityCounter]) []types.AttestationCounter {
		var out []types.AttestationCounter
		if err := m.Walk(ctx, nil, func(addr string, counter types.ActivityCounter) (bool, error) {
			out = append(out, types.AttestationCounter{NodeAddress: addr, Counter: counter})
			return false, nil
		}); err != nil {
			panic(err)
		}
		return out
	}

	return &types.GenesisState{
		Params:      params,
		RwaCounters: exportCounters(k.RwaCounters),
		RwuCounters: exportCounters(k.RwuCounters),
	}
}
