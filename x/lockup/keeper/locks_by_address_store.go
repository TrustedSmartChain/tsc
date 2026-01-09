package keeper

import (
	"time"

	"cosmossdk.io/math"
	"github.com/TrustedSmartChain/tsc/x/lockup/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	sdkerrors "github.com/cosmos/cosmos-sdk/types/errors"
)

// GetLocksByAddressKey creates the key for storing locks by address
// Key: LocksByAddressKey + Address
func (k Keeper) GetLocksByAddressKey(addr sdk.AccAddress) []byte {
	return append(types.LocksByAddressKey, addr.Bytes()...)
}

func (k Keeper) GetLockedAmountByAddress(ctx sdk.Context, addr sdk.AccAddress) (*math.Int, error) {
	locks, err := k.GetLocksByAddress(ctx, addr)
	if err != nil {
		return nil, err
	}

	blockTime := ctx.BlockTime()
	blockDay := time.Date(blockTime.Year(), blockTime.Month(), blockTime.Day(), 0, 0, 0, 0, time.UTC)

	totalLocked := math.ZeroInt()
	for _, lock := range locks {
		if types.IsLocked(blockDay, lock.UnlockDate) {
			totalLocked = totalLocked.Add(lock.Amount)
		}
	}

	return &totalLocked, nil
}

// SetLockByAddress adds a lock to an address key
func (k Keeper) SetLockByAddress(ctx sdk.Context, addr sdk.AccAddress, lock *types.Lock) error {
	store := k.storeService.OpenKVStore(ctx)
	key := k.GetLocksByAddressKey(addr)

	locksList := &types.Locks{}
	bz, err := store.Get(key)
	if err != nil {
		return err
	}

	if bz != nil {
		if err := locksList.Unmarshal(bz); err != nil {
			return err
		}
	}

	// before appending insert the lock into the correct position based on unlock date
	inserted := false
	for i, existingLock := range locksList.Locks {
		if lock.UnlockDate < existingLock.UnlockDate {
			locksList.Locks = append(locksList.Locks[:i], append([]*types.Lock{lock}, locksList.Locks[i:]...)...)
			inserted = true
			break
		}
	}
	if !inserted {
		locksList.Locks = append(locksList.Locks, lock)
	}

	bz, err = locksList.Marshal()
	if err != nil {
		return err
	}

	return store.Set(key, bz)
}

// SetLocksByAddress stores all locks for an address in a single key-value pair
func (k Keeper) SetLocksByAddress(ctx sdk.Context, addr sdk.AccAddress, locks []*types.Lock) error {
	store := k.storeService.OpenKVStore(ctx)
	key := k.GetLocksByAddressKey(addr)

	if len(locks) == 0 {
		return store.Delete(key)
	}

	locksList := types.Locks{Locks: locks}
	bz, err := locksList.Marshal()
	if err != nil {
		return err
	}

	return store.Set(key, bz)
}

// GetLocksByAddress retrieves all locks for an address
func (k Keeper) GetLocksByAddress(ctx sdk.Context, addr sdk.AccAddress) ([]*types.Lock, error) {
	store := k.storeService.OpenKVStore(ctx)
	key := k.GetLocksByAddressKey(addr)

	bz, err := store.Get(key)
	if err != nil {
		return nil, err
	}

	if bz == nil {
		return []*types.Lock{}, nil
	}

	locksList := &types.Locks{}
	if err := locksList.Unmarshal(bz); err != nil {
		return nil, err
	}

	return locksList.Locks, nil
}

// GetLockByAddressAndDate retrieves a lock for a specific address and unlock date
func (k Keeper) GetLockByAddressAndDate(ctx sdk.Context, addr sdk.AccAddress, unlockDate string) (*types.Lock, int, bool) {
	locks, err := k.GetLocksByAddress(ctx, addr)
	if err != nil {
		return nil, -1, false
	}

	for idx, lock := range locks {
		if lock.UnlockDate == unlockDate {
			return lock, idx, true
		}
	}

	return nil, -1, false
}

// DeleteLocksByAddress removes all locks for an address
func (k Keeper) DeleteLocksByAddress(ctx sdk.Context, addr sdk.AccAddress) error {
	store := k.storeService.OpenKVStore(ctx)
	key := k.GetLocksByAddressKey(addr)
	return store.Delete(key)
}

// DeleteLockByAddressAndIndex removes a specific lock for an address by its index
func (k Keeper) DeleteLockByAddressAndIndex(ctx sdk.Context, addr sdk.AccAddress, index int) error {
	locks, err := k.GetLocksByAddress(ctx, addr)
	if err != nil {
		return err
	}

	if index < 0 || index >= len(locks) {
		return sdkerrors.ErrInvalidRequest.Wrapf("lock doesn't exist at index %d", index)
	}

	locks = append(locks[:index], locks[index+1:]...)
	return k.SetLocksByAddress(ctx, addr, locks)
}

func (k Keeper) UpdateLockByAddressAndIndex(ctx sdk.Context, addr sdk.AccAddress, index int, lock *types.Lock) error {
	locks, err := k.GetLocksByAddress(ctx, addr)
	if err != nil {
		return err
	}

	if index < 0 || index >= len(locks) {
		return sdkerrors.ErrInvalidRequest.Wrapf("lock doesn't exist at index %d", index)
	}

	locks[index] = lock
	return k.SetLocksByAddress(ctx, addr, locks)
}
