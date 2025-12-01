package types

import (
	cryptotypes "github.com/cosmos/cosmos-sdk/crypto/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	authtypes "github.com/cosmos/cosmos-sdk/x/auth/types"
)

func NewLongTermStakeAccount(baseAccount *authtypes.BaseAccount) *Account {
	return &Account{
		BaseAccount: baseAccount,
		Lockups:     map[string]*Lockup{},
	}
}

// Implement sdk.AccountI interface
func (a *Account) GetAddress() sdk.AccAddress {
	if a.BaseAccount != nil {
		return a.BaseAccount.GetAddress()
	}
	return nil
}

func (a *Account) SetAddress(addr sdk.AccAddress) error {
	if a.BaseAccount != nil {
		return a.BaseAccount.SetAddress(addr)
	}
	return nil
}

func (a *Account) GetPubKey() cryptotypes.PubKey {
	if a.BaseAccount != nil {
		return a.BaseAccount.GetPubKey()
	}
	return nil
}

func (a *Account) SetPubKey(pubKey cryptotypes.PubKey) error {
	if a.BaseAccount != nil {
		return a.BaseAccount.SetPubKey(pubKey)
	}
	return nil
}

func (a *Account) GetAccountNumber() uint64 {
	if a.BaseAccount != nil {
		return a.BaseAccount.GetAccountNumber()
	}
	return 0
}

func (a *Account) SetAccountNumber(accNumber uint64) error {
	if a.BaseAccount != nil {
		return a.BaseAccount.SetAccountNumber(accNumber)
	}
	return nil
}

func (a *Account) GetSequence() uint64 {
	if a.BaseAccount != nil {
		return a.BaseAccount.GetSequence()
	}
	return 0
}

func (a *Account) SetSequence(seq uint64) error {
	if a.BaseAccount != nil {
		return a.BaseAccount.SetSequence(seq)
	}
	return nil
}
