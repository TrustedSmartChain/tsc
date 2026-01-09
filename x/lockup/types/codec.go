package types

import (
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/codec/types"
	cryptocodec "github.com/cosmos/cosmos-sdk/crypto/codec"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/cosmos/cosmos-sdk/types/msgservice"
)

var (
	amino    = codec.NewLegacyAmino()
	AminoCdc = codec.NewAminoCodec(amino)
)

func init() {
	RegisterLegacyAminoCodec(amino)
	cryptocodec.RegisterCrypto(amino)
	sdk.RegisterLegacyAminoCodec(amino)
}

// RegisterLegacyAminoCodec registers concrete types on the LegacyAmino codec
func RegisterLegacyAminoCodec(cdc *codec.LegacyAmino) {
	cdc.RegisterConcrete(&MsgLock{}, ModuleName+"/MsgLock", nil)
	cdc.RegisterConcrete(&MsgExtend{}, ModuleName+"/MsgExtend", nil)
	cdc.RegisterConcrete(&MsgSendDelegateAndLock{}, ModuleName+"/MsgSendDelegateAndLock", nil)
	cdc.RegisterConcrete(&MsgMultiSendDelegateAndLock{}, ModuleName+"/MsgMultiSendDelegateAndLock", nil)
}

func RegisterInterfaces(registry types.InterfaceRegistry) {

	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgLock{},
		&MsgExtend{},
		&MsgSendDelegateAndLock{},
		&MsgMultiSendDelegateAndLock{},
	)

	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
