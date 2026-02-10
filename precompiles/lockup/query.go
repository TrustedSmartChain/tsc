package lockup

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"

	sdk "github.com/cosmos/cosmos-sdk/types"

	lockuptypes "github.com/TrustedSmartChain/tsc/x/lockup/types"
)

const (
	// LocksMethod defines the ABI method name for the lockup Locks query.
	LocksMethod = "locks"
	// TotalLockedAmountMethod defines the ABI method name for the lockup TotalLockedAmount query.
	TotalLockedAmountMethod = "totalLockedAmount"
)

// LockInfoOutput represents a lock entry returned to the EVM caller.
type LockInfoOutput struct {
	UnlockDate string   `abi:"unlockDate"`
	Denom      string   `abi:"denom"`
	Amount     *big.Int `abi:"amount"`
}

// Locks returns the active locks for a specific address.
func (p Precompile) Locks(
	ctx sdk.Context,
	method *abi.Method,
	_ *vm.Contract,
	args []interface{},
) ([]byte, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 1, len(args))
	}

	lockAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, fmt.Errorf("invalid lock address: %v", args[0])
	}

	bech32Addr := sdk.AccAddress(lockAddress.Bytes()).String()

	// Query locks for the address
	res, err := p.lockupQuerier.Locks(ctx, &lockuptypes.QueryLocksRequest{
		Address: bech32Addr,
	})
	if err != nil {
		return nil, err
	}

	// Convert the response to the output format
	locks := make([]LockInfoOutput, len(res.Locks))
	for i, lock := range res.Locks {
		locks[i] = LockInfoOutput{
			UnlockDate: lock.UnlockDate,
			Denom:      lock.Amount.Denom,
			Amount:     lock.Amount.Amount.BigInt(),
		}
	}

	return method.Outputs.Pack(locks)
}

// TotalLockedAmount returns the total locked amount across all accounts.
func (p Precompile) TotalLockedAmount(
	ctx sdk.Context,
	method *abi.Method,
	_ *vm.Contract,
	args []interface{},
) ([]byte, error) {
	if len(args) != 0 {
		return nil, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 0, len(args))
	}

	// Query total locked amount
	res, err := p.lockupQuerier.TotalLockedAmount(ctx, &lockuptypes.QueryTotalLockedAmountRequest{})
	if err != nil {
		return nil, err
	}

	return method.Outputs.Pack(res.TotalLocked.Denom, res.TotalLocked.Amount.BigInt())
}
