package keeper

import (
	"fmt"

	sdk "github.com/cosmos/cosmos-sdk/types"

	"github.com/TrustedSmartChain/tsc/v4/x/distro/types"
)

// Migrator handles in-place store migrations for the distro module.
type Migrator struct {
	keeper Keeper
}

func NewMigrator(k Keeper) Migrator {
	return Migrator{keeper: k}
}

// Migrate1to2 drops the max_supply param (field 4), replaced in v4.2 by the
// compiled-in types.MaxSupply. Decoding v1 params skips the unknown field, so
// re-writing them is enough to remove it from state.
func (m Migrator) Migrate1to2(ctx sdk.Context) error {
	params, err := m.keeper.Params.Get(ctx)
	if err != nil {
		return fmt.Errorf("read distro params: %w", err)
	}
	if err := params.Validate(); err != nil {
		return fmt.Errorf("distro params after dropping max_supply: %w", err)
	}
	if err := m.keeper.Params.Set(ctx, params); err != nil {
		return err
	}
	m.keeper.Logger().Info("Removed the distro max_supply param; the supply cap is fixed", "max_supply", types.MaxSupply().String())
	return nil
}
