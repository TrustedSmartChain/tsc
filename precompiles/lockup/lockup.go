package lockup

import (
	"bytes"
	"fmt"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	_ "embed"

	cmn "github.com/cosmos/evm/precompiles/common"

	"cosmossdk.io/log"
	storetypes "cosmossdk.io/store/types"

	sdk "github.com/cosmos/cosmos-sdk/types"

	lockupkeeper "github.com/TrustedSmartChain/tsc/x/lockup/keeper"
	lockuptypes "github.com/TrustedSmartChain/tsc/x/lockup/types"
)

var _ vm.PrecompiledContract = &Precompile{}

const (
	// LockupPrecompileAddress defines the precompile address for the lockup module.
	LockupPrecompileAddress = "0x0000000000000000000000000000000000000900"
)

var (
	// Embed abi json file to the executable binary. Needed when importing as dependency.
	//
	//go:embed abi.json
	f   []byte
	ABI abi.ABI
)

func init() {
	var err error
	ABI, err = abi.JSON(bytes.NewReader(f))
	if err != nil {
		panic(err)
	}
}

// Precompile defines the precompiled contract for lockup.
type Precompile struct {
	cmn.Precompile
	abi.ABI
	lockupKeeper    lockupkeeper.Keeper
	lockupMsgServer lockuptypes.MsgServer
	lockupQuerier   lockuptypes.QueryServer
	stakingKeeper   lockuptypes.StakingKeeper
}

// NewPrecompile creates a new lockup Precompile instance as a
// PrecompiledContract interface.
func NewPrecompile(
	lockupKeeper lockupkeeper.Keeper,
	lockupMsgServer lockuptypes.MsgServer,
	lockupQuerier lockuptypes.QueryServer,
	stakingKeeper lockuptypes.StakingKeeper,
	bankKeeper cmn.BankKeeper,
) *Precompile {
	return &Precompile{
		Precompile: cmn.Precompile{
			KvGasConfig:           storetypes.KVGasConfig(),
			TransientKVGasConfig:  storetypes.TransientGasConfig(),
			ContractAddress:       common.HexToAddress(LockupPrecompileAddress),
			BalanceHandlerFactory: cmn.NewBalanceHandlerFactory(bankKeeper),
		},
		ABI:             ABI,
		lockupKeeper:    lockupKeeper,
		lockupMsgServer: lockupMsgServer,
		lockupQuerier:   lockupQuerier,
		stakingKeeper:   stakingKeeper,
	}
}

// Address defines the address of the lockup precompile contract.
func (p Precompile) Address() common.Address {
	return p.ContractAddress
}

// Logger returns a precompile-specific logger.
func (p Precompile) Logger(ctx sdk.Context) log.Logger {
	return ctx.Logger().With("precompile", "lockup")
}

// RequiredGas returns the required bare minimum gas to execute the precompile.
func (p Precompile) RequiredGas(input []byte) uint64 {
	// NOTE: This check avoid panicking when trying to decode the method ID
	if len(input) < 4 {
		return 0
	}

	methodID := input[:4]
	method, err := p.MethodById(methodID)
	if err != nil {
		// This should never happen since this method is going to fail during Run
		return 0
	}

	return p.Precompile.RequiredGas(input, p.IsTransaction(method))
}

// Run executes the precompile contract lockup methods defined in the ABI.
func (p Precompile) Run(evm *vm.EVM, contract *vm.Contract, readonly bool) ([]byte, error) {
	return p.RunNativeAction(evm, contract, func(ctx sdk.Context) ([]byte, error) {
		return p.Execute(ctx, evm.StateDB, contract, readonly)
	})
}

// Execute runs the precompile contract lockup methods.
func (p Precompile) Execute(ctx sdk.Context, stateDB vm.StateDB, contract *vm.Contract, readOnly bool) ([]byte, error) {
	method, args, err := cmn.SetupABI(p.ABI, contract, readOnly, p.IsTransaction)
	if err != nil {
		return nil, err
	}

	var bz []byte

	switch method.Name {
	// Lockup transactions
	case LockMethod:
		bz, err = p.Lock(ctx, contract, stateDB, method, args)
	case ExtendMethod:
		bz, err = p.Extend(ctx, contract, stateDB, method, args)
	case SendDelegateAndLockMethod:
		bz, err = p.SendDelegateAndLock(ctx, contract, stateDB, method, args)
	// Lockup queries
	case LocksMethod:
		bz, err = p.Locks(ctx, method, contract, args)
	case TotalLockedAmountMethod:
		bz, err = p.TotalLockedAmount(ctx, method, contract, args)
	default:
		return nil, fmt.Errorf(cmn.ErrUnknownMethod, method.Name)
	}

	if err != nil {
		return nil, err
	}

	return bz, nil
}

// IsTransaction checks if the given method name corresponds to a write operation.
func (p Precompile) IsTransaction(method *abi.Method) bool {
	switch method.Name {
	case LockMethod, ExtendMethod, SendDelegateAndLockMethod:
		return true
	default:
		return false
	}
}
