package keeper

import (
	storetypes "cosmossdk.io/core/store"
	"cosmossdk.io/log"

	"github.com/TrustedSmartChain/tsc/v3/x/node/types"
)

type Keeper struct {
	storeService storetypes.KVStoreService
	logger       log.Logger
}

func NewKeeper(storeService storetypes.KVStoreService, logger log.Logger) Keeper {
	return Keeper{
		storeService: storeService,
		logger:       logger.With(log.ModuleKey, "x/"+types.ModuleName),
	}
}
