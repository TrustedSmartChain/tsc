package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
)

// Merkle scheme for the decentralized distribution.
//
// The tree is a domain-separated binary SHA-256 merkle tree using
// commutative (sorted-pair) inner hashing, so a proof is just the ordered list
// of sibling hashes — no left/right direction bits are required.
//
// IMPORTANT: the off-chain node app that produces the distribution roots MUST
// build the tree with exactly this encoding, or every on-chain proof will fail.
// The leaf preimage and hashing rules below are the canonical specification;
// the merkle_test.go vectors pin them down.
const (
	// leafPrefix domain-separates leaf hashes from inner-node hashes.
	leafPrefix byte = 0x00
	// innerPrefix domain-separates inner-node hashes from leaf hashes.
	innerPrefix byte = 0x01
)

// LeafHash returns the leaf hash for a single reward entry.
//
// The leaf preimage is:
//
//	0x00 || uint64BE(nonce) || uint32BE(len(addr)) || addr || uint32BE(len(amount)) || amount
//
// Length-prefixing addr and amount makes the boundary between the two
// variable-length fields unambiguous.
func LeafHash(nonce uint64, addr, amount string) []byte {
	buf := make([]byte, 0, 1+8+4+len(addr)+4+len(amount))
	buf = append(buf, leafPrefix)

	var n [8]byte
	binary.BigEndian.PutUint64(n[:], nonce)
	buf = append(buf, n[:]...)

	buf = appendLenPrefixed(buf, []byte(addr))
	buf = appendLenPrefixed(buf, []byte(amount))

	sum := sha256.Sum256(buf)
	return sum[:]
}

// HashPair returns the inner-node hash of two child hashes. The children are
// ordered (sorted) before hashing so the scheme is commutative and proofs do
// not need direction information.
func HashPair(a, b []byte) []byte {
	buf := make([]byte, 0, 1+len(a)+len(b))
	buf = append(buf, innerPrefix)
	if bytes.Compare(a, b) <= 0 {
		buf = append(buf, a...)
		buf = append(buf, b...)
	} else {
		buf = append(buf, b...)
		buf = append(buf, a...)
	}
	sum := sha256.Sum256(buf)
	return sum[:]
}

// VerifyProof folds the ordered sibling hashes in proof into leaf and reports
// whether the result equals root.
func VerifyProof(root, leaf []byte, proof [][]byte) bool {
	computed := leaf
	for _, sibling := range proof {
		computed = HashPair(computed, sibling)
	}
	return bytes.Equal(computed, root)
}

func appendLenPrefixed(buf, b []byte) []byte {
	var l [4]byte
	binary.BigEndian.PutUint32(l[:], uint32(len(b)))
	buf = append(buf, l[:]...)
	return append(buf, b...)
}
