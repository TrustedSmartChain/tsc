package app

import (
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"math/big"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	sdkmath "cosmossdk.io/math"
	txsigning "cosmossdk.io/x/tx/signing"

	codectypes "github.com/cosmos/cosmos-sdk/codec/types"
	sdk "github.com/cosmos/cosmos-sdk/types"
	signingtypes "github.com/cosmos/cosmos-sdk/types/tx/signing"

	"github.com/cosmos/cosmos-sdk/client"

	evmtypes "github.com/cosmos/evm/x/vm/types"
)

// This file provides backward-compatible decoding for MsgEthereumTx transactions
// from strangelove-ventures/cosmos-evm v0.1.x.
//
// The old proto schema (v0.1.x) used:
//   field 1: google.protobuf.Any data (containing LegacyTx/AccessListTx/DynamicFeeTx)
//   field 2: double size (deprecated, fixed64)
//   field 3: string hash
//   field 4: string from
//
// The new proto schema (v0.5.x) uses:
//   field 5: bytes from
//   field 6: bytes raw (RLP-encoded EthereumTx)
//   fields 1-4 are reserved
//
// When old-format transactions are decoded with the new schema, fields 1-4 are
// silently skipped (they're reserved), resulting in an empty MsgEthereumTx.
// This wrapper TxDecoder detects this case and falls back to legacy decoding.

// legacyMsgEthereumTx mirrors the OLD MsgEthereumTx proto layout.
// It reads fields 1-4 directly via manual protobuf decoding.
type legacyMsgEthereumTx struct {
	Data *codectypes.Any // field 1, bytes (Any message)
	Size float64         // field 2, fixed64 (double)
	Hash string          // field 3, string
	From string          // field 4, string
}

// legacyLegacyTx mirrors the old LegacyTx proto message.
type legacyLegacyTx struct {
	Nonce    uint64
	GasPrice *sdkmath.Int
	GasLimit uint64
	To       string
	Amount   *sdkmath.Int
	Data     []byte
	V        []byte
	R        []byte
	S        []byte
}

// legacyAccessTuple mirrors the old AccessTuple proto message.
type legacyAccessTuple struct {
	Address     string
	StorageKeys []string
}

// legacyAccessListTx mirrors the old AccessListTx proto message.
type legacyAccessListTx struct {
	ChainID  *sdkmath.Int
	Nonce    uint64
	GasPrice *sdkmath.Int
	GasLimit uint64
	To       string
	Amount   *sdkmath.Int
	Data     []byte
	Accesses []legacyAccessTuple
	V        []byte
	R        []byte
	S        []byte
}

// legacyDynamicFeeTx mirrors the old DynamicFeeTx proto message.
type legacyDynamicFeeTx struct {
	ChainID   *sdkmath.Int
	Nonce     uint64
	GasTipCap *sdkmath.Int
	GasFeeCap *sdkmath.Int
	GasLimit  uint64
	To        string
	Amount    *sdkmath.Int
	Data      []byte
	Accesses  []legacyAccessTuple
	V         []byte
	R         []byte
	S         []byte
}

// LegacyAwareTxDecoder wraps the standard TxDecoder and adds fallback
// decoding for old-format MsgEthereumTx transactions.
func LegacyAwareTxDecoder(original sdk.TxDecoder, txConfig client.TxConfig) sdk.TxDecoder {
	return func(txBytes []byte) (sdk.Tx, error) {
		tx, err := original(txBytes)
		if err != nil {
			// The new proto schema marks fields 1-4 as `reserved`. Some
			// protobuf decoders (especially those using the newer
			// google.golang.org/protobuf runtime) reject reserved fields
			// with errUnknownField instead of silently skipping them.
			// When this happens for MsgEthereumTx, fall through to legacy
			// decoding rather than propagating the error.
			if strings.Contains(err.Error(), "MsgEthereumTx") ||
				strings.Contains(err.Error(), "unknown field") ||
				strings.Contains(err.Error(), "errUnknownField") {
				return decodeLegacyEthTx(txBytes, txConfig)
			}
			return nil, err
		}

		msgs := tx.GetMsgs()
		needsConversion := false
		for _, msg := range msgs {
			ethMsg, ok := msg.(*evmtypes.MsgEthereumTx)
			if !ok {
				continue
			}
			// If it's an EVM tx but Raw is nil/empty and From is empty,
			// it was decoded from old format where fields 1-4 were silently dropped.
			if ethMsg.Raw.Transaction == nil && len(ethMsg.From) == 0 {
				needsConversion = true
				break
			}
		}

		if !needsConversion {
			return tx, nil
		}

		// Re-decode using legacy format
		return decodeLegacyEthTx(txBytes, txConfig)
	}
}

// decodeLegacyEthTx re-decodes a transaction containing old-format MsgEthereumTx.
// It extracts the inner tx data from the old Any-based format and converts it
// to the new RLP-based format.
func decodeLegacyEthTx(txBytes []byte, txConfig client.TxConfig) (sdk.Tx, error) {
	// The outer Cosmos tx wraps the MsgEthereumTx in a TxBody.Messages[].
	// We need to find the Any-encoded MsgEthereumTx within the Cosmos tx,
	// then manually decode its old-format fields.
	//
	// Strategy: Use the standard decoder's Cosmos Tx envelope parsing to get
	// the raw Any bytes for the MsgEthereumTx message, then manually decode
	// the old fields from that raw data.

	// Parse the outer Cosmos SDK Tx structure to extract the raw Any value bytes.
	// The Cosmos tx is a cosmos.tx.v1beta1.Tx protobuf, where body.messages
	// contains the MsgEthereumTx as an Any.
	rawMsgBytes, err := extractMsgAnyValue(txBytes)
	if err != nil {
		return nil, fmt.Errorf("legacy eth tx: failed to extract msg bytes: %w", err)
	}

	// Decode the old MsgEthereumTx fields from the raw protobuf bytes
	legacyMsg, err := unmarshalLegacyMsgEthereumTx(rawMsgBytes)
	if err != nil {
		return nil, fmt.Errorf("legacy eth tx: failed to decode old format: %w", err)
	}

	// Convert old format to go-ethereum Transaction
	ethTx, from, err := legacyMsgToEthTransaction(legacyMsg)
	if err != nil {
		return nil, fmt.Errorf("legacy eth tx: failed to convert: %w", err)
	}

	// Build a new-format MsgEthereumTx
	newMsg := &evmtypes.MsgEthereumTx{}
	newMsg.FromEthereumTx(ethTx)
	newMsg.From = from.Bytes()

	// Rebuild the Cosmos Tx with the converted message
	builder := txConfig.NewTxBuilder()
	if err := builder.SetMsgs(newMsg); err != nil {
		return nil, fmt.Errorf("legacy eth tx: failed to set msgs: %w", err)
	}

	return builder.GetTx(), nil
}

// extractMsgAnyValue extracts the raw value bytes of the first message Any
// from a Cosmos SDK Tx protobuf.
//
// Cosmos Tx layout:
//
//	field 1: TxBody body
//	  field 1 (repeated): google.protobuf.Any messages
//	    field 1: string type_url
//	    field 2: bytes value  <-- this is what we want
func extractMsgAnyValue(txBytes []byte) ([]byte, error) {
	// Parse Tx -> field 1 (body)
	bodyBytes, err := extractProtoField(txBytes, 1, 2) // field 1, wire type 2 (LEN)
	if err != nil {
		return nil, fmt.Errorf("cannot extract tx body: %w", err)
	}

	// Parse TxBody -> field 1 (messages[0]) - first Any
	anyBytes, err := extractProtoField(bodyBytes, 1, 2) // field 1, wire type 2 (LEN)
	if err != nil {
		return nil, fmt.Errorf("cannot extract first message Any: %w", err)
	}

	// Parse Any -> field 2 (value)
	valueBytes, err := extractProtoField(anyBytes, 2, 2) // field 2, wire type 2 (LEN)
	if err != nil {
		return nil, fmt.Errorf("cannot extract Any value: %w", err)
	}

	return valueBytes, nil
}

// extractProtoField extracts the first occurrence of a specific field from protobuf bytes.
func extractProtoField(data []byte, targetFieldNum int, targetWireType int) ([]byte, error) {
	i := 0
	for i < len(data) {
		// Read tag
		tag, n := protoDecodeVarint(data[i:])
		if n == 0 {
			return nil, fmt.Errorf("bad varint at offset %d", i)
		}
		i += n

		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x7)

		switch wireType {
		case 0: // varint
			_, n = protoDecodeVarint(data[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad varint value at offset %d", i)
			}
			if fieldNum == targetFieldNum && targetWireType == 0 {
				return data[i : i+n], nil
			}
			i += n
		case 1: // 64-bit
			if i+8 > len(data) {
				return nil, io.ErrUnexpectedEOF
			}
			if fieldNum == targetFieldNum && targetWireType == 1 {
				return data[i : i+8], nil
			}
			i += 8
		case 2: // length-delimited
			length, n := protoDecodeVarint(data[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad length at offset %d", i)
			}
			i += n
			end := i + int(length)
			if end > len(data) {
				return nil, io.ErrUnexpectedEOF
			}
			if fieldNum == targetFieldNum && targetWireType == 2 {
				return data[i:end], nil
			}
			i = end
		case 5: // 32-bit
			if i+4 > len(data) {
				return nil, io.ErrUnexpectedEOF
			}
			if fieldNum == targetFieldNum && targetWireType == 5 {
				return data[i : i+4], nil
			}
			i += 4
		default:
			return nil, fmt.Errorf("unsupported wire type %d at offset %d", wireType, i)
		}
	}
	return nil, fmt.Errorf("field %d not found", targetFieldNum)
}

// protoDecodeVarint decodes a protobuf varint from the given bytes.
// Returns the value and number of bytes consumed.
func protoDecodeVarint(data []byte) (uint64, int) {
	var val uint64
	for i := 0; i < len(data) && i < 10; i++ {
		b := data[i]
		val |= uint64(b&0x7F) << (uint(i) * 7)
		if b < 0x80 {
			return val, i + 1
		}
	}
	return 0, 0
}

// unmarshalLegacyMsgEthereumTx decodes the old-format MsgEthereumTx from raw protobuf bytes.
func unmarshalLegacyMsgEthereumTx(data []byte) (*legacyMsgEthereumTx, error) {
	msg := &legacyMsgEthereumTx{}
	i := 0
	for i < len(data) {
		tag, n := protoDecodeVarint(data[i:])
		if n == 0 {
			return nil, fmt.Errorf("bad varint at offset %d", i)
		}
		i += n
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x7)

		switch fieldNum {
		case 1: // Data (Any) - wire type 2 (LEN)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for Data field: %d", wireType)
			}
			length, n := protoDecodeVarint(data[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad length for Data")
			}
			i += n
			end := i + int(length)
			if end > len(data) {
				return nil, io.ErrUnexpectedEOF
			}
			// Parse the Any message to get type_url and value
			any, err := unmarshalAny(data[i:end])
			if err != nil {
				return nil, fmt.Errorf("failed to unmarshal Any: %w", err)
			}
			msg.Data = any
			i = end

		case 2: // Size_ (double/fixed64) - wire type 1
			if wireType != 1 {
				return nil, fmt.Errorf("wrong wire type for Size field: %d", wireType)
			}
			if i+8 > len(data) {
				return nil, io.ErrUnexpectedEOF
			}
			bits := binary.LittleEndian.Uint64(data[i:])
			msg.Size = math.Float64frombits(bits)
			i += 8

		case 3: // Hash (string) - wire type 2 (LEN)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for Hash field: %d", wireType)
			}
			length, n := protoDecodeVarint(data[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad length for Hash")
			}
			i += n
			end := i + int(length)
			if end > len(data) {
				return nil, io.ErrUnexpectedEOF
			}
			msg.Hash = string(data[i:end])
			i = end

		case 4: // From (string) - wire type 2 (LEN)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for From field: %d", wireType)
			}
			length, n := protoDecodeVarint(data[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad length for From")
			}
			i += n
			end := i + int(length)
			if end > len(data) {
				return nil, io.ErrUnexpectedEOF
			}
			msg.From = string(data[i:end])
			i = end

		default:
			// Skip unknown fields
			switch wireType {
			case 0:
				_, n = protoDecodeVarint(data[i:])
				if n == 0 {
					return nil, fmt.Errorf("bad varint skip at %d", i)
				}
				i += n
			case 1:
				i += 8
			case 2:
				length, n := protoDecodeVarint(data[i:])
				if n == 0 {
					return nil, fmt.Errorf("bad length skip at %d", i)
				}
				i += n + int(length)
			case 5:
				i += 4
			default:
				return nil, fmt.Errorf("unsupported wire type %d", wireType)
			}
		}
	}
	return msg, nil
}

// unmarshalAny decodes a google.protobuf.Any from raw bytes.
func unmarshalAny(data []byte) (*codectypes.Any, error) {
	any := &codectypes.Any{}
	i := 0
	for i < len(data) {
		tag, n := protoDecodeVarint(data[i:])
		if n == 0 {
			return nil, fmt.Errorf("bad varint in Any at %d", i)
		}
		i += n
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x7)

		if wireType != 2 {
			// Skip non-LEN fields
			switch wireType {
			case 0:
				_, n = protoDecodeVarint(data[i:])
				i += n
			case 1:
				i += 8
			case 5:
				i += 4
			default:
				return nil, fmt.Errorf("unsupported wire type %d in Any", wireType)
			}
			continue
		}

		length, n := protoDecodeVarint(data[i:])
		if n == 0 {
			return nil, fmt.Errorf("bad length in Any at %d", i)
		}
		i += n
		end := i + int(length)
		if end > len(data) {
			return nil, io.ErrUnexpectedEOF
		}

		switch fieldNum {
		case 1: // type_url
			any.TypeUrl = string(data[i:end])
		case 2: // value
			any.Value = make([]byte, end-i)
			copy(any.Value, data[i:end])
		}
		i = end
	}
	return any, nil
}

// legacyMsgToEthTransaction converts a legacy MsgEthereumTx to an ethtypes.Transaction.
func legacyMsgToEthTransaction(msg *legacyMsgEthereumTx) (*ethtypes.Transaction, common.Address, error) {
	if msg.Data == nil {
		return nil, common.Address{}, fmt.Errorf("legacy MsgEthereumTx has nil Data")
	}

	var from common.Address
	if msg.From != "" {
		from = common.HexToAddress(msg.From)
	}

	switch msg.Data.TypeUrl {
	case "/cosmos.evm.vm.v1.LegacyTx":
		return decodeLegacyTxInner(msg.Data.Value, from)
	case "/cosmos.evm.vm.v1.AccessListTx":
		return decodeAccessListTxInner(msg.Data.Value, from)
	case "/cosmos.evm.vm.v1.DynamicFeeTx":
		return decodeDynamicFeeTxInner(msg.Data.Value, from)
	default:
		return nil, common.Address{}, fmt.Errorf("unknown legacy tx data type: %s", msg.Data.TypeUrl)
	}
}

// decodeLegacyTxInner decodes the old LegacyTx protobuf and converts to ethtypes.Transaction.
func decodeLegacyTxInner(data []byte, from common.Address) (*ethtypes.Transaction, common.Address, error) {
	lt, err := unmarshalLegacyTx(data)
	if err != nil {
		return nil, common.Address{}, err
	}

	var to *common.Address
	if lt.To != "" {
		addr := common.HexToAddress(lt.To)
		to = &addr
	}

	v, r, s := decodeSigBytes(lt.V, lt.R, lt.S)

	tx := ethtypes.NewTx(&ethtypes.LegacyTx{
		Nonce:    lt.Nonce,
		GasPrice: sdkIntToBigInt(lt.GasPrice),
		Gas:      lt.GasLimit,
		To:       to,
		Value:    sdkIntToBigInt(lt.Amount),
		Data:     lt.Data,
		V:        v,
		R:        r,
		S:        s,
	})

	if from == (common.Address{}) {
		signer := ethtypes.LatestSignerForChainID(tx.ChainId())
		sender, err := ethtypes.Sender(signer, tx)
		if err == nil {
			from = sender
		}
	}

	return tx, from, nil
}

// decodeAccessListTxInner decodes the old AccessListTx protobuf and converts to ethtypes.Transaction.
func decodeAccessListTxInner(data []byte, from common.Address) (*ethtypes.Transaction, common.Address, error) {
	alt, err := unmarshalAccessListTx(data)
	if err != nil {
		return nil, common.Address{}, err
	}

	var to *common.Address
	if alt.To != "" {
		addr := common.HexToAddress(alt.To)
		to = &addr
	}

	v, r, s := decodeSigBytes(alt.V, alt.R, alt.S)

	tx := ethtypes.NewTx(&ethtypes.AccessListTx{
		ChainID:    sdkIntToBigInt(alt.ChainID),
		Nonce:      alt.Nonce,
		GasPrice:   sdkIntToBigInt(alt.GasPrice),
		Gas:        alt.GasLimit,
		To:         to,
		Value:      sdkIntToBigInt(alt.Amount),
		Data:       alt.Data,
		AccessList: convertLegacyAccessList(alt.Accesses),
		V:          v,
		R:          r,
		S:          s,
	})

	if from == (common.Address{}) {
		signer := ethtypes.LatestSignerForChainID(tx.ChainId())
		sender, err := ethtypes.Sender(signer, tx)
		if err == nil {
			from = sender
		}
	}

	return tx, from, nil
}

// decodeDynamicFeeTxInner decodes the old DynamicFeeTx protobuf and converts to ethtypes.Transaction.
func decodeDynamicFeeTxInner(data []byte, from common.Address) (*ethtypes.Transaction, common.Address, error) {
	dft, err := unmarshalDynamicFeeTx(data)
	if err != nil {
		return nil, common.Address{}, err
	}

	var to *common.Address
	if dft.To != "" {
		addr := common.HexToAddress(dft.To)
		to = &addr
	}

	v, r, s := decodeSigBytes(dft.V, dft.R, dft.S)

	tx := ethtypes.NewTx(&ethtypes.DynamicFeeTx{
		ChainID:    sdkIntToBigInt(dft.ChainID),
		Nonce:      dft.Nonce,
		GasTipCap:  sdkIntToBigInt(dft.GasTipCap),
		GasFeeCap:  sdkIntToBigInt(dft.GasFeeCap),
		Gas:        dft.GasLimit,
		To:         to,
		Value:      sdkIntToBigInt(dft.Amount),
		Data:       dft.Data,
		AccessList: convertLegacyAccessList(dft.Accesses),
		V:          v,
		R:          r,
		S:          s,
	})

	if from == (common.Address{}) {
		signer := ethtypes.LatestSignerForChainID(tx.ChainId())
		sender, err := ethtypes.Sender(signer, tx)
		if err == nil {
			from = sender
		}
	}

	return tx, from, nil
}

// unmarshalLegacyTx decodes the old LegacyTx protobuf fields.
// Field layout:
//
//	1: varint nonce
//	2: LEN gas_price (cosmossdk.io/math.Int)
//	3: varint gas
//	4: LEN to (string)
//	5: LEN value/amount (cosmossdk.io/math.Int)
//	6: LEN data (bytes)
//	7: LEN v (bytes)
//	8: LEN r (bytes)
//	9: LEN s (bytes)
func unmarshalLegacyTx(data []byte) (*legacyLegacyTx, error) {
	tx := &legacyLegacyTx{}
	i := 0
	for i < len(data) {
		tag, n := protoDecodeVarint(data[i:])
		if n == 0 {
			return nil, fmt.Errorf("bad varint at %d", i)
		}
		i += n
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x7)

		switch fieldNum {
		case 1: // nonce (varint)
			if wireType != 0 {
				return nil, fmt.Errorf("wrong wire type for nonce: %d", wireType)
			}
			v, n := protoDecodeVarint(data[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad varint for nonce")
			}
			tx.Nonce = v
			i += n
		case 2: // gas_price (LEN, cosmossdk.io/math.Int)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for gas_price: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			v := sdkmath.Int{}
			if err := v.Unmarshal(b); err != nil {
				return nil, fmt.Errorf("failed to unmarshal gas_price: %w", err)
			}
			tx.GasPrice = &v
		case 3: // gas (varint)
			if wireType != 0 {
				return nil, fmt.Errorf("wrong wire type for gas: %d", wireType)
			}
			v, n := protoDecodeVarint(data[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad varint for gas")
			}
			tx.GasLimit = v
			i += n
		case 4: // to (LEN, string)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for to: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.To = string(b)
		case 5: // value/amount (LEN, cosmossdk.io/math.Int)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for amount: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			v := sdkmath.Int{}
			if err := v.Unmarshal(b); err != nil {
				return nil, fmt.Errorf("failed to unmarshal amount: %w", err)
			}
			tx.Amount = &v
		case 6: // data (LEN, bytes)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for data: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.Data = b
		case 7: // v (LEN, bytes)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for V: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.V = b
		case 8: // r (LEN, bytes)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for R: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.R = b
		case 9: // s (LEN, bytes)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for S: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.S = b
		default:
			n, err := skipProtoField(data[i:], wireType)
			if err != nil {
				return nil, err
			}
			i += n
		}
	}
	return tx, nil
}

// unmarshalAccessListTx decodes the old AccessListTx protobuf fields.
// Field layout:
//
//	1: LEN chain_id (cosmossdk.io/math.Int)
//	2: varint nonce
//	3: LEN gas_price (cosmossdk.io/math.Int)
//	4: varint gas
//	5: LEN to (string)
//	6: LEN value/amount (cosmossdk.io/math.Int)
//	7: LEN data (bytes)
//	8: LEN accesses (repeated AccessTuple)
//	9: LEN v (bytes)
//	10: LEN r (bytes)
//	11: LEN s (bytes)
func unmarshalAccessListTx(data []byte) (*legacyAccessListTx, error) {
	tx := &legacyAccessListTx{}
	i := 0
	for i < len(data) {
		tag, n := protoDecodeVarint(data[i:])
		if n == 0 {
			return nil, fmt.Errorf("bad varint at %d", i)
		}
		i += n
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x7)

		switch fieldNum {
		case 1: // chain_id
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for chain_id: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			v := sdkmath.Int{}
			if err := v.Unmarshal(b); err != nil {
				return nil, fmt.Errorf("failed to unmarshal chain_id: %w", err)
			}
			tx.ChainID = &v
		case 2: // nonce
			if wireType != 0 {
				return nil, fmt.Errorf("wrong wire type for nonce: %d", wireType)
			}
			v, n := protoDecodeVarint(data[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad varint for nonce")
			}
			tx.Nonce = v
			i += n
		case 3: // gas_price
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for gas_price: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			v := sdkmath.Int{}
			if err := v.Unmarshal(b); err != nil {
				return nil, fmt.Errorf("failed to unmarshal gas_price: %w", err)
			}
			tx.GasPrice = &v
		case 4: // gas
			if wireType != 0 {
				return nil, fmt.Errorf("wrong wire type for gas: %d", wireType)
			}
			v, n := protoDecodeVarint(data[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad varint for gas")
			}
			tx.GasLimit = v
			i += n
		case 5: // to
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for to: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.To = string(b)
		case 6: // amount
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for amount: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			v := sdkmath.Int{}
			if err := v.Unmarshal(b); err != nil {
				return nil, fmt.Errorf("failed to unmarshal amount: %w", err)
			}
			tx.Amount = &v
		case 7: // data
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for data: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.Data = b
		case 8: // accesses (repeated)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for accesses: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			at, err := unmarshalAccessTuple(b)
			if err != nil {
				return nil, err
			}
			tx.Accesses = append(tx.Accesses, *at)
		case 9: // v
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for V: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.V = b
		case 10: // r
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for R: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.R = b
		case 11: // s
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for S: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.S = b
		default:
			n, err := skipProtoField(data[i:], wireType)
			if err != nil {
				return nil, err
			}
			i += n
		}
	}
	return tx, nil
}

// unmarshalDynamicFeeTx decodes the old DynamicFeeTx protobuf fields.
// Field layout:
//
//	1: LEN chain_id (cosmossdk.io/math.Int)
//	2: varint nonce
//	3: LEN gas_tip_cap (cosmossdk.io/math.Int)
//	4: LEN gas_fee_cap (cosmossdk.io/math.Int)
//	5: varint gas
//	6: LEN to (string)
//	7: LEN value/amount (cosmossdk.io/math.Int)
//	8: LEN data (bytes)
//	9: LEN accesses (repeated AccessTuple)
//	10: LEN v (bytes)
//	11: LEN r (bytes)
//	12: LEN s (bytes)
func unmarshalDynamicFeeTx(data []byte) (*legacyDynamicFeeTx, error) {
	tx := &legacyDynamicFeeTx{}
	i := 0
	for i < len(data) {
		tag, n := protoDecodeVarint(data[i:])
		if n == 0 {
			return nil, fmt.Errorf("bad varint at %d", i)
		}
		i += n
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x7)

		switch fieldNum {
		case 1: // chain_id
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for chain_id: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			v := sdkmath.Int{}
			if err := v.Unmarshal(b); err != nil {
				return nil, fmt.Errorf("failed to unmarshal chain_id: %w", err)
			}
			tx.ChainID = &v
		case 2: // nonce
			if wireType != 0 {
				return nil, fmt.Errorf("wrong wire type for nonce: %d", wireType)
			}
			v, n := protoDecodeVarint(data[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad varint for nonce")
			}
			tx.Nonce = v
			i += n
		case 3: // gas_tip_cap
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for gas_tip_cap: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			v := sdkmath.Int{}
			if err := v.Unmarshal(b); err != nil {
				return nil, fmt.Errorf("failed to unmarshal gas_tip_cap: %w", err)
			}
			tx.GasTipCap = &v
		case 4: // gas_fee_cap
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for gas_fee_cap: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			v := sdkmath.Int{}
			if err := v.Unmarshal(b); err != nil {
				return nil, fmt.Errorf("failed to unmarshal gas_fee_cap: %w", err)
			}
			tx.GasFeeCap = &v
		case 5: // gas
			if wireType != 0 {
				return nil, fmt.Errorf("wrong wire type for gas: %d", wireType)
			}
			v, n := protoDecodeVarint(data[i:])
			if n == 0 {
				return nil, fmt.Errorf("bad varint for gas")
			}
			tx.GasLimit = v
			i += n
		case 6: // to
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for to: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.To = string(b)
		case 7: // amount
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for amount: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			v := sdkmath.Int{}
			if err := v.Unmarshal(b); err != nil {
				return nil, fmt.Errorf("failed to unmarshal amount: %w", err)
			}
			tx.Amount = &v
		case 8: // data
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for data: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.Data = b
		case 9: // accesses (repeated)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for accesses: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			at, err := unmarshalAccessTuple(b)
			if err != nil {
				return nil, err
			}
			tx.Accesses = append(tx.Accesses, *at)
		case 10: // v
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for V: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.V = b
		case 11: // r
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for R: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.R = b
		case 12: // s
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for S: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			tx.S = b
		default:
			n, err := skipProtoField(data[i:], wireType)
			if err != nil {
				return nil, err
			}
			i += n
		}
	}
	return tx, nil
}

// unmarshalAccessTuple decodes an AccessTuple proto message.
// Field layout:
//
//	1: LEN address (string)
//	2: LEN storage_keys (repeated string)
func unmarshalAccessTuple(data []byte) (*legacyAccessTuple, error) {
	at := &legacyAccessTuple{}
	i := 0
	for i < len(data) {
		tag, n := protoDecodeVarint(data[i:])
		if n == 0 {
			return nil, fmt.Errorf("bad varint in AccessTuple at %d", i)
		}
		i += n
		fieldNum := int(tag >> 3)
		wireType := int(tag & 0x7)

		switch fieldNum {
		case 1: // address
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for address: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			at.Address = string(b)
		case 2: // storage_keys (repeated)
			if wireType != 2 {
				return nil, fmt.Errorf("wrong wire type for storage_keys: %d", wireType)
			}
			b, n, err := readLenDelimited(data[i:])
			if err != nil {
				return nil, err
			}
			i += n
			at.StorageKeys = append(at.StorageKeys, string(b))
		default:
			n, err := skipProtoField(data[i:], wireType)
			if err != nil {
				return nil, err
			}
			i += n
		}
	}
	return at, nil
}

// Helper functions

func readLenDelimited(data []byte) ([]byte, int, error) {
	length, n := protoDecodeVarint(data)
	if n == 0 {
		return nil, 0, fmt.Errorf("bad length varint")
	}
	end := n + int(length)
	if end > len(data) {
		return nil, 0, io.ErrUnexpectedEOF
	}
	return data[n:end], end, nil
}

func skipProtoField(data []byte, wireType int) (int, error) {
	switch wireType {
	case 0: // varint
		_, n := protoDecodeVarint(data)
		if n == 0 {
			return 0, fmt.Errorf("bad varint skip")
		}
		return n, nil
	case 1: // 64-bit
		return 8, nil
	case 2: // LEN
		length, n := protoDecodeVarint(data)
		if n == 0 {
			return 0, fmt.Errorf("bad length skip")
		}
		return n + int(length), nil
	case 5: // 32-bit
		return 4, nil
	default:
		return 0, fmt.Errorf("unsupported wire type %d", wireType)
	}
}

func sdkIntToBigInt(v *sdkmath.Int) *big.Int {
	if v == nil {
		return big.NewInt(0)
	}
	return v.BigInt()
}

func decodeSigBytes(vb, rb, sb []byte) (*big.Int, *big.Int, *big.Int) {
	v := new(big.Int)
	r := new(big.Int)
	s := new(big.Int)
	if len(vb) > 0 {
		v.SetBytes(vb)
	}
	if len(rb) > 0 {
		r.SetBytes(rb)
	}
	if len(sb) > 0 {
		s.SetBytes(sb)
	}
	return v, r, s
}

func convertLegacyAccessList(tuples []legacyAccessTuple) ethtypes.AccessList {
	if len(tuples) == 0 {
		return nil
	}
	al := make(ethtypes.AccessList, len(tuples))
	for i, t := range tuples {
		al[i] = ethtypes.AccessTuple{
			Address: common.HexToAddress(t.Address),
		}
		for _, key := range t.StorageKeys {
			al[i].StorageKeys = append(al[i].StorageKeys, common.HexToHash(key))
		}
	}
	return al
}

// ---------------------------------------------------------------------------
// legacyAwareTxConfig wraps a client.TxConfig and overrides TxDecoder()
// (and TxJSONDecoder()) to use the legacy-aware decoder. This ensures that
// ALL consumers of TxConfig — including gRPC tx query services, API gateway
// routes, and the JSON-RPC backend — use the legacy-aware decoder, not just
// BaseApp's block processing.
// ---------------------------------------------------------------------------

// legacyAwareTxConfig implements client.TxConfig by delegating to an inner
// TxConfig but overriding TxDecoder to handle old-format MsgEthereumTx.
type legacyAwareTxConfig struct {
	inner client.TxConfig
}

var _ client.TxConfig = (*legacyAwareTxConfig)(nil)

// NewLegacyAwareTxConfig wraps a standard TxConfig so that its TxDecoder
// and TxJSONDecoder transparently handle old-format MsgEthereumTx.
func NewLegacyAwareTxConfig(inner client.TxConfig) client.TxConfig {
	return &legacyAwareTxConfig{inner: inner}
}

// --- TxEncodingConfig methods (overridden) ---

func (c *legacyAwareTxConfig) TxDecoder() sdk.TxDecoder {
	return LegacyAwareTxDecoder(c.inner.TxDecoder(), c.inner)
}

func (c *legacyAwareTxConfig) TxJSONDecoder() sdk.TxDecoder {
	// JSON queries also need legacy awareness — wrap the JSON decoder too.
	return LegacyAwareTxDecoder(c.inner.TxJSONDecoder(), c.inner)
}

func (c *legacyAwareTxConfig) TxEncoder() sdk.TxEncoder {
	return c.inner.TxEncoder()
}

func (c *legacyAwareTxConfig) TxJSONEncoder() sdk.TxEncoder {
	return c.inner.TxJSONEncoder()
}

func (c *legacyAwareTxConfig) MarshalSignatureJSON(sigs []signingtypes.SignatureV2) ([]byte, error) {
	return c.inner.MarshalSignatureJSON(sigs)
}

func (c *legacyAwareTxConfig) UnmarshalSignatureJSON(bz []byte) ([]signingtypes.SignatureV2, error) {
	return c.inner.UnmarshalSignatureJSON(bz)
}

// --- TxConfig methods (delegated) ---

func (c *legacyAwareTxConfig) NewTxBuilder() client.TxBuilder {
	return c.inner.NewTxBuilder()
}

func (c *legacyAwareTxConfig) WrapTxBuilder(tx sdk.Tx) (client.TxBuilder, error) {
	return c.inner.WrapTxBuilder(tx)
}

func (c *legacyAwareTxConfig) SignModeHandler() *txsigning.HandlerMap {
	return c.inner.SignModeHandler()
}

func (c *legacyAwareTxConfig) SigningContext() *txsigning.Context {
	return c.inner.SigningContext()
}
