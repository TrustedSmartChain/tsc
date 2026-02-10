package lockup

import (
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"

	sdk "github.com/cosmos/cosmos-sdk/types"

	lockuptypes "github.com/TrustedSmartChain/tsc/x/lockup/types"
)

const (
	// EventTypeLock defines the event type for the lockup Lock transaction.
	EventTypeLock = "Lock"
	// EventTypeLockExtended defines the event type for the lockup LockExtended transaction.
	EventTypeLockExtended = "LockExtended"
	// EventTypeSendDelegateAndLock defines the event type for the lockup SendDelegateAndLock transaction.
	EventTypeSendDelegateAndLock = "SendDelegateAndLock"
)

// EventLock defines the event data for the lockup Lock transaction.
type EventLock struct {
	LockAddress common.Address
	UnlockDate  string
	Amount      *big.Int
}

// EventLockExtended defines the event data for the lockup LockExtended transaction.
type EventLockExtended struct {
	LockAddress   common.Address
	OldUnlockDate string
	NewUnlockDate string
	Amount        *big.Int
}

// EventSendDelegateAndLock defines the event data for the lockup SendDelegateAndLock transaction.
type EventSendDelegateAndLock struct {
	FromAddress      common.Address
	ToAddress        common.Address
	ValidatorAddress string
	UnlockDate       string
	Amount           *big.Int
}

// EmitLockEvent creates a new event emitted on a Lock transaction.
func (p Precompile) EmitLockEvent(ctx sdk.Context, stateDB vm.StateDB, msg *lockuptypes.MsgLock, lockAddr common.Address) error {
	// Prepare the event topics
	event := p.Events[EventTypeLock]
	topics := make([]common.Hash, 2)

	// The first topic is always the signature of the event.
	topics[0] = event.ID

	var err error
	topics[1], err = cmn.MakeTopic(lockAddr)
	if err != nil {
		return err
	}

	// Pack the arguments to be used as the Data field
	arguments := abi.Arguments{event.Inputs[1], event.Inputs[2]}
	packed, err := arguments.Pack(msg.UnlockDate, msg.Amount.Amount.BigInt())
	if err != nil {
		return err
	}

	stateDB.AddLog(&ethtypes.Log{
		Address:     p.Address(),
		Topics:      topics,
		Data:        packed,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec // G115
	})

	return nil
}

// EmitLockExtendedEvent creates a new event emitted on an Extend transaction.
func (p Precompile) EmitLockExtendedEvent(ctx sdk.Context, stateDB vm.StateDB, address string, ext *lockuptypes.Extension, lockAddr common.Address) error {
	// Prepare the event topics
	event := p.Events[EventTypeLockExtended]
	topics := make([]common.Hash, 2)

	// The first topic is always the signature of the event.
	topics[0] = event.ID

	var err error
	topics[1], err = cmn.MakeTopic(lockAddr)
	if err != nil {
		return err
	}

	// Pack the arguments to be used as the Data field
	arguments := abi.Arguments{event.Inputs[1], event.Inputs[2], event.Inputs[3]}
	packed, err := arguments.Pack(ext.FromDate, ext.ToDate, ext.Amount.Amount.BigInt())
	if err != nil {
		return err
	}

	stateDB.AddLog(&ethtypes.Log{
		Address:     p.Address(),
		Topics:      topics,
		Data:        packed,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec // G115
	})

	return nil
}

// EmitSendDelegateAndLockEvent creates a new event emitted on a SendDelegateAndLock transaction.
func (p Precompile) EmitSendDelegateAndLockEvent(ctx sdk.Context, stateDB vm.StateDB, msg *lockuptypes.MsgSendDelegateAndLock, fromAddr, toAddr common.Address) error {
	// Prepare the event topics
	event := p.Events[EventTypeSendDelegateAndLock]
	topics := make([]common.Hash, 3)

	// The first topic is always the signature of the event.
	topics[0] = event.ID

	var err error
	topics[1], err = cmn.MakeTopic(fromAddr)
	if err != nil {
		return err
	}

	topics[2], err = cmn.MakeTopic(toAddr)
	if err != nil {
		return err
	}

	// Pack the arguments to be used as the Data field
	arguments := abi.Arguments{event.Inputs[2], event.Inputs[3], event.Inputs[4]}
	packed, err := arguments.Pack(msg.ValidatorAddress, msg.UnlockDate, msg.Amount.Amount.BigInt())
	if err != nil {
		return err
	}

	stateDB.AddLog(&ethtypes.Log{
		Address:     p.Address(),
		Topics:      topics,
		Data:        packed,
		BlockNumber: uint64(ctx.BlockHeight()), //nolint:gosec // G115
	})

	return nil
}
