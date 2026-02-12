package app

import (
	"crypto/ecdsa"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"

	sdkmath "cosmossdk.io/math"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
)

func TestUnmarshalLegacyMsgEthereumTx(t *testing.T) {
	// Build a legacy MsgEthereumTx protobuf by hand:
	// Field 1: Any data with type_url="/cosmos.evm.vm.v1.LegacyTx"
	// Field 3: Hash string
	// Field 4: From string

	// First build a simple LegacyTx inner protobuf
	// LegacyTx: nonce=1, gas=21000
	ltData := encodeProtoVarint(1, 1)                       // nonce = 1
	ltData = append(ltData, encodeProtoVarint(3, 21000)...) // gas = 21000

	// Build the Any: type_url (field 1) + value (field 2)
	typeUrl := "/cosmos.evm.vm.v1.LegacyTx"
	anyData := encodeProtoString(1, typeUrl)
	anyData = append(anyData, encodeProtoBytes(2, ltData)...)

	// Build the MsgEthereumTx:
	// field 1 (Data): LEN
	// field 3 (Hash): string
	// field 4 (From): string
	msgData := encodeProtoBytes(1, anyData)
	hashStr := "0xdeadbeef"
	msgData = append(msgData, encodeProtoString(3, hashStr)...)
	fromStr := "0x1234567890abcdef1234567890abcdef12345678"
	msgData = append(msgData, encodeProtoString(4, fromStr)...)

	legacy, err := unmarshalLegacyMsgEthereumTx(msgData)
	require.NoError(t, err)
	require.NotNil(t, legacy.Data)
	require.Equal(t, "/cosmos.evm.vm.v1.LegacyTx", legacy.Data.TypeUrl)
	require.Equal(t, hashStr, legacy.Hash)
	require.Equal(t, fromStr, legacy.From)
}

func TestUnmarshalLegacyTx(t *testing.T) {
	// Build a LegacyTx proto with nonce=5, gas=21000 using proper encoding helpers
	data := encodeProtoVarint(1, 5)                     // nonce = 5
	data = append(data, encodeProtoVarint(3, 21000)...) // gas = 21000

	lt, err := unmarshalLegacyTx(data)
	require.NoError(t, err)
	require.Equal(t, uint64(5), lt.Nonce)
	require.Equal(t, uint64(21000), lt.GasLimit)
}

func TestDecodeSigBytes(t *testing.T) {
	vb := big.NewInt(28).Bytes()
	rb := big.NewInt(12345).Bytes()
	sb := big.NewInt(67890).Bytes()

	v, r, s := decodeSigBytes(vb, rb, sb)
	require.Equal(t, big.NewInt(28), v)
	require.Equal(t, big.NewInt(12345), r)
	require.Equal(t, big.NewInt(67890), s)
}

func TestSdkIntToBigInt(t *testing.T) {
	val := sdkmath.NewInt(42)
	result := sdkIntToBigInt(&val)
	require.Equal(t, big.NewInt(42), result)

	result = sdkIntToBigInt(nil)
	require.Equal(t, big.NewInt(0), result)
}

func TestConvertLegacyAccessList(t *testing.T) {
	tuples := []legacyAccessTuple{
		{
			Address:     "0x1234567890abcdef1234567890abcdef12345678",
			StorageKeys: []string{"0x0000000000000000000000000000000000000000000000000000000000000001"},
		},
	}

	result := convertLegacyAccessList(tuples)
	require.Len(t, result, 1)
	require.Equal(t, common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678"), result[0].Address)
	require.Len(t, result[0].StorageKeys, 1)

	// Empty access list
	result = convertLegacyAccessList(nil)
	require.Nil(t, result)
}

func TestLegacyMsgToEthTransaction(t *testing.T) {
	// Test with LegacyTx
	nonce := uint64(5)
	gasLimit := uint64(21000)
	gasPrice := sdkmath.NewIntFromBigInt(big.NewInt(1e9))
	amount := sdkmath.NewIntFromBigInt(big.NewInt(1e18))
	to := "0x1234567890AbcdEF1234567890aBcdef12345678"

	// Build LegacyTx proto bytes
	ltBytes := buildLegacyTxProtoBytes(nonce, &gasPrice, gasLimit, to, &amount, nil, nil, nil, nil)

	// Build Any wrapping it
	anyData := &codectypes.Any{
		TypeUrl: "/cosmos.evm.vm.v1.LegacyTx",
		Value:   ltBytes,
	}

	fromAddr := "0xDeaDbeefdEAdbeefdEadbEEFdeadbeEFdEaDbeeF"
	msg := &legacyMsgEthereumTx{
		Data: anyData,
		From: fromAddr,
	}

	ethTx, from, err := legacyMsgToEthTransaction(msg)
	require.NoError(t, err)
	require.NotNil(t, ethTx)
	require.Equal(t, nonce, ethTx.Nonce())
	require.Equal(t, gasLimit, ethTx.Gas())
	require.Equal(t, common.HexToAddress(fromAddr), from)

	toAddr := common.HexToAddress(to)
	require.Equal(t, &toAddr, ethTx.To())
}

func TestExtractProtoField(t *testing.T) {
	// Build a simple protobuf: field 1 (string "hello"), field 2 (string "world")
	data := encodeProtoString(1, "hello")
	data = append(data, encodeProtoString(2, "world")...)

	val, err := extractProtoField(data, 1, 2)
	require.NoError(t, err)
	require.Equal(t, "hello", string(val))

	val, err = extractProtoField(data, 2, 2)
	require.NoError(t, err)
	require.Equal(t, "world", string(val))

	_, err = extractProtoField(data, 3, 2)
	require.Error(t, err)
}

func TestUnmarshalAny(t *testing.T) {
	typeUrl := "/cosmos.evm.vm.v1.LegacyTx"
	value := []byte{0x08, 0x05} // some proto bytes

	data := encodeProtoString(1, typeUrl)
	data = append(data, encodeProtoBytes(2, value)...)

	any, err := unmarshalAny(data)
	require.NoError(t, err)
	require.Equal(t, typeUrl, any.TypeUrl)
	require.Equal(t, value, any.Value)
}

func TestRoundTripLegacyTx(t *testing.T) {
	// Create a go-ethereum LegacyTx, marshal it to the old protobuf format,
	// then decode it back using our legacy decoder.
	chainID := big.NewInt(8878788)
	privKey, _ := generateTestKey()
	signer := ethtypes.NewEIP155Signer(chainID)

	to := common.HexToAddress("0x1234567890abcdef1234567890abcdef12345678")
	tx := ethtypes.NewTx(&ethtypes.LegacyTx{
		Nonce:    42,
		GasPrice: big.NewInt(1e9),
		Gas:      21000,
		To:       &to,
		Value:    big.NewInt(1e18),
		Data:     nil,
	})

	signedTx, err := ethtypes.SignTx(tx, signer, privKey)
	require.NoError(t, err)

	// Extract signature
	v, r, s := signedTx.RawSignatureValues()

	// Build LegacyTx protobuf
	gasPrice := sdkmath.NewIntFromBigInt(big.NewInt(1e9))
	amount := sdkmath.NewIntFromBigInt(big.NewInt(1e18))
	ltBytes := buildLegacyTxProtoBytes(42, &gasPrice, 21000, to.Hex(), &amount, nil, v.Bytes(), r.Bytes(), s.Bytes())

	anyData := &codectypes.Any{
		TypeUrl: "/cosmos.evm.vm.v1.LegacyTx",
		Value:   ltBytes,
	}

	sender, err := ethtypes.Sender(signer, signedTx)
	require.NoError(t, err)

	msg := &legacyMsgEthereumTx{
		Data: anyData,
		Hash: signedTx.Hash().Hex(),
		From: sender.Hex(),
	}

	ethTx, from, err := legacyMsgToEthTransaction(msg)
	require.NoError(t, err)
	require.Equal(t, uint64(42), ethTx.Nonce())
	require.Equal(t, uint64(21000), ethTx.Gas())
	require.Equal(t, &to, ethTx.To())
	require.Equal(t, sender, from)

	// Verify the signature is preserved and recoverable
	recoveredSender, err := ethtypes.Sender(signer, ethTx)
	require.NoError(t, err)
	require.Equal(t, sender, recoveredSender)
}

// ========== Test helpers ==========

func generateTestKey() (*ecdsa.PrivateKey, error) {
	return crypto.HexToECDSA("fad9c8855b740a0b7ed4c221dbad0f33a83a49cad6b3fe8d5817ac83d38b6a19")
}

// buildLegacyTxProtoBytes builds an old-format LegacyTx protobuf.
func buildLegacyTxProtoBytes(nonce uint64, gasPrice *sdkmath.Int, gas uint64, to string, amount *sdkmath.Int, data []byte, v, r, s []byte) []byte {
	var buf []byte

	if nonce != 0 {
		buf = append(buf, encodeProtoVarint(1, nonce)...)
	}
	if gasPrice != nil {
		gasPriceBytes, _ := gasPrice.Marshal()
		buf = append(buf, encodeProtoBytes(2, gasPriceBytes)...)
	}
	if gas != 0 {
		buf = append(buf, encodeProtoVarint(3, gas)...)
	}
	if to != "" {
		buf = append(buf, encodeProtoString(4, to)...)
	}
	if amount != nil {
		amountBytes, _ := amount.Marshal()
		buf = append(buf, encodeProtoBytes(5, amountBytes)...)
	}
	if len(data) > 0 {
		buf = append(buf, encodeProtoBytes(6, data)...)
	}
	if len(v) > 0 {
		buf = append(buf, encodeProtoBytes(7, v)...)
	}
	if len(r) > 0 {
		buf = append(buf, encodeProtoBytes(8, r)...)
	}
	if len(s) > 0 {
		buf = append(buf, encodeProtoBytes(9, s)...)
	}

	return buf
}

func encodeProtoVarint(fieldNum int, val uint64) []byte {
	tag := uint64(fieldNum)<<3 | 0 // wire type 0 = varint
	var buf []byte
	buf = appendVarint(buf, tag)
	buf = appendVarint(buf, val)
	return buf
}

func encodeProtoString(fieldNum int, val string) []byte {
	return encodeProtoBytes(fieldNum, []byte(val))
}

func encodeProtoBytes(fieldNum int, val []byte) []byte {
	tag := uint64(fieldNum)<<3 | 2 // wire type 2 = LEN
	var buf []byte
	buf = appendVarint(buf, tag)
	buf = appendVarint(buf, uint64(len(val)))
	buf = append(buf, val...)
	return buf
}

func appendVarint(buf []byte, val uint64) []byte {
	for val >= 0x80 {
		buf = append(buf, byte(val&0x7F|0x80))
		val >>= 7
	}
	buf = append(buf, byte(val))
	return buf
}
