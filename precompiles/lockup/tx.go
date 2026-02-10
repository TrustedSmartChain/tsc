package lockup

import (
	"fmt"
	"math/big"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/vm"

	cmn "github.com/cosmos/evm/precompiles/common"

	sdkmath "cosmossdk.io/math"

	sdk "github.com/cosmos/cosmos-sdk/types"

	lockuptypes "github.com/TrustedSmartChain/tsc/x/lockup/types"
)

const (
	// LockMethod defines the ABI method name for the lockup Lock transaction.
	LockMethod = "lock"
	// ExtendMethod defines the ABI method name for the lockup Extend transaction.
	ExtendMethod = "extend"
	// SendDelegateAndLockMethod defines the ABI method name for the lockup SendDelegateAndLock transaction.
	SendDelegateAndLockMethod = "sendDelegateAndLock"
)

// Lock performs a lock of tokens for a specific address until an unlock date.
func (p *Precompile) Lock(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom, err := p.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	msg, lockHexAddr, err := NewMsgLock(args, bondDenom)
	if err != nil {
		return nil, err
	}

	p.Logger(ctx).Debug(
		"tx called",
		"method", method.Name,
		"args", fmt.Sprintf(
			"{ lock_address: %s, unlock_date: %s, amount: %s }",
			lockHexAddr,
			msg.UnlockDate,
			msg.Amount.Amount,
		),
	)

	msgSender := contract.Caller()
	if msgSender != lockHexAddr {
		return nil, fmt.Errorf(cmn.ErrRequesterIsNotMsgSender, msgSender.String(), lockHexAddr.String())
	}

	// Execute the transaction using the message server
	if _, err = p.lockupMsgServer.Lock(ctx, msg); err != nil {
		return nil, err
	}

	// Emit the event for the lock transaction
	if err = p.EmitLockEvent(ctx, stateDB, msg, lockHexAddr); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

// Extend performs an extension of existing lock durations.
func (p *Precompile) Extend(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom, err := p.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	msg, lockHexAddr, err := NewMsgExtend(args, bondDenom)
	if err != nil {
		return nil, err
	}

	p.Logger(ctx).Debug(
		"tx called",
		"method", method.Name,
		"args", fmt.Sprintf(
			"{ lock_address: %s, extensions_count: %d }",
			lockHexAddr,
			len(msg.Extensions),
		),
	)

	msgSender := contract.Caller()
	if msgSender != lockHexAddr {
		return nil, fmt.Errorf(cmn.ErrRequesterIsNotMsgSender, msgSender.String(), lockHexAddr.String())
	}

	// Execute the transaction using the message server
	if _, err = p.lockupMsgServer.Extend(ctx, msg); err != nil {
		return nil, err
	}

	// Emit events for each extension
	for _, ext := range msg.Extensions {
		if err = p.EmitLockExtendedEvent(ctx, stateDB, msg.Address, ext, lockHexAddr); err != nil {
			return nil, err
		}
	}

	return method.Outputs.Pack(true)
}

// SendDelegateAndLock sends tokens to an address, delegates them, and locks them.
func (p *Precompile) SendDelegateAndLock(
	ctx sdk.Context,
	contract *vm.Contract,
	stateDB vm.StateDB,
	method *abi.Method,
	args []interface{},
) ([]byte, error) {
	bondDenom, err := p.stakingKeeper.BondDenom(ctx)
	if err != nil {
		return nil, err
	}

	msg, fromHexAddr, toHexAddr, err := NewMsgSendDelegateAndLock(args, contract.Caller(), bondDenom)
	if err != nil {
		return nil, err
	}

	p.Logger(ctx).Debug(
		"tx called",
		"method", method.Name,
		"args", fmt.Sprintf(
			"{ from_address: %s, to_address: %s, validator_address: %s, unlock_date: %s, amount: %s }",
			fromHexAddr,
			msg.ToAddress,
			msg.ValidatorAddress,
			msg.UnlockDate,
			msg.Amount.Amount,
		),
	)

	// Execute the transaction using the message server
	if _, err = p.lockupMsgServer.SendDelegateAndLock(ctx, msg); err != nil {
		return nil, err
	}

	// Emit the event for the send delegate and lock transaction
	if err = p.EmitSendDelegateAndLockEvent(ctx, stateDB, msg, fromHexAddr, toHexAddr); err != nil {
		return nil, err
	}

	return method.Outputs.Pack(true)
}

// NewMsgLock creates a new MsgLock from the provided arguments.
// args: [lockAddress (address), unlockDate (string), amount (uint256)]
func NewMsgLock(args []interface{}, bondDenom string) (*lockuptypes.MsgLock, common.Address, error) {
	if len(args) != 3 {
		return nil, common.Address{}, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 3, len(args))
	}

	lockAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, common.Address{}, fmt.Errorf("invalid lock address: %v", args[0])
	}

	unlockDate, ok := args[1].(string)
	if !ok {
		return nil, common.Address{}, fmt.Errorf("invalid unlock date: %v", args[1])
	}

	amount, ok := args[2].(*big.Int)
	if !ok {
		return nil, common.Address{}, fmt.Errorf(cmn.ErrInvalidAmount, args[2])
	}

	amt := sdkmath.NewIntFromBigInt(amount)
	if !amt.IsPositive() {
		return nil, common.Address{}, fmt.Errorf("lock amount must be positive")
	}

	bech32Addr := sdk.AccAddress(lockAddress.Bytes()).String()

	msg := &lockuptypes.MsgLock{
		Address:    bech32Addr,
		UnlockDate: unlockDate,
		Amount:     sdk.NewCoin(bondDenom, amt),
	}

	return msg, lockAddress, nil
}

// NewMsgExtend creates a new MsgExtend from the provided arguments.
// args: [lockAddress (address), extensions (LockExtension[])]
func NewMsgExtend(args []interface{}, bondDenom string) (*lockuptypes.MsgExtend, common.Address, error) {
	if len(args) != 2 {
		return nil, common.Address{}, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 2, len(args))
	}

	lockAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, common.Address{}, fmt.Errorf("invalid lock address: %v", args[0])
	}

	bech32Addr := sdk.AccAddress(lockAddress.Bytes()).String()

	// Parse extensions from ABI-encoded data, enforcing the bond denomination.
	extensions, err := parseExtensions(args[1], bondDenom)
	if err != nil {
		return nil, common.Address{}, fmt.Errorf("failed to parse extensions: %w", err)
	}

	msg := &lockuptypes.MsgExtend{
		Address:    bech32Addr,
		Extensions: extensions,
	}

	return msg, lockAddress, nil
}

// parseExtensions converts raw ABI extension data to lockup Extension types.
// The ABI decodes tuple[] as a slice of anonymous structs matching the Solidity struct layout.
// The bondDenom parameter overrides any user-supplied denom to prevent denom-confusion attacks.
func parseExtensions(raw interface{}, bondDenom string) ([]*lockuptypes.Extension, error) {
	type coinInput struct {
		Denom  string   `abi:"denom"`
		Amount *big.Int `abi:"amount"`
	}

	type extensionInput struct {
		FromDate string    `abi:"fromDate"`
		ToDate   string    `abi:"toDate"`
		Amount   coinInput `abi:"amount"`
	}

	slice, ok := raw.([]extensionInput)
	if !ok {
		return nil, fmt.Errorf("invalid extensions type: %T", raw)
	}

	if len(slice) == 0 {
		return nil, fmt.Errorf("extensions must not be empty")
	}

	extensions := make([]*lockuptypes.Extension, len(slice))
	for i, ext := range slice {
		amt := sdkmath.NewIntFromBigInt(ext.Amount.Amount)
		if !amt.IsPositive() {
			return nil, fmt.Errorf("extension amount must be positive, got %s", amt.String())
		}

		// Enforce bond denom regardless of user-supplied value
		extensions[i] = &lockuptypes.Extension{
			FromDate: ext.FromDate,
			ToDate:   ext.ToDate,
			Amount:   sdk.NewCoin(bondDenom, amt),
		}
	}

	return extensions, nil
}

// NewMsgSendDelegateAndLock creates a new MsgSendDelegateAndLock from the provided arguments.
// The fromAddress is derived from contract.Caller().
// args: [toAddress (address), validatorAddress (string), unlockDate (string), amount (uint256)]
func NewMsgSendDelegateAndLock(args []interface{}, caller common.Address, bondDenom string) (*lockuptypes.MsgSendDelegateAndLock, common.Address, common.Address, error) {
	if len(args) != 4 {
		return nil, common.Address{}, common.Address{}, fmt.Errorf(cmn.ErrInvalidNumberOfArgs, 4, len(args))
	}

	toAddress, ok := args[0].(common.Address)
	if !ok {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("invalid to address: %v", args[0])
	}

	validatorAddress, ok := args[1].(string)
	if !ok {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("invalid validator address: %v", args[1])
	}

	unlockDate, ok := args[2].(string)
	if !ok {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("invalid unlock date: %v", args[2])
	}

	amount, ok := args[3].(*big.Int)
	if !ok {
		return nil, common.Address{}, common.Address{}, fmt.Errorf(cmn.ErrInvalidAmount, args[3])
	}

	amt := sdkmath.NewIntFromBigInt(amount)
	if !amt.IsPositive() {
		return nil, common.Address{}, common.Address{}, fmt.Errorf("send delegate and lock amount must be positive")
	}

	fromBech32 := sdk.AccAddress(caller.Bytes()).String()
	toBech32 := sdk.AccAddress(toAddress.Bytes()).String()

	msg := &lockuptypes.MsgSendDelegateAndLock{
		FromAddress:      fromBech32,
		ToAddress:        toBech32,
		ValidatorAddress: validatorAddress,
		UnlockDate:       unlockDate,
		Amount:           sdk.NewCoin(bondDenom, amt),
	}

	return msg, caller, toAddress, nil
}
