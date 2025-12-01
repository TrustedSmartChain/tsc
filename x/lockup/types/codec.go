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
	cdc.RegisterConcrete(&MsgUpdateParams{}, ModuleName+"/MsgUpdateParams", nil)
	cdc.RegisterConcrete(&MsgLock{}, ModuleName+"/MsgLock", nil)
	cdc.RegisterConcrete(&MsgExtend{}, ModuleName+"/MsgExtend", nil)
	cdc.RegisterConcrete(&MsgSendAndLock{}, ModuleName+"/MsgSendAndLock", nil)
	cdc.RegisterConcrete(&MsgMultiSendAndLock{}, ModuleName+"/MsgMultiSendAndLock", nil)
	cdc.RegisterConcrete(&Account{}, ModuleName+"/Account", nil)
}

func RegisterInterfaces(registry types.InterfaceRegistry) {

	registry.RegisterImplementations(
		(*sdk.Msg)(nil),
		&MsgUpdateParams{},
		&MsgLock{},
		&MsgExtend{},
		&MsgSendAndLock{},
		&MsgMultiSendAndLock{},
	)

	registry.RegisterImplementations(
		(*sdk.AccountI)(nil),
		&Account{},
	)

	msgservice.RegisterMsgServiceDesc(registry, &_Msg_serviceDesc)
}
