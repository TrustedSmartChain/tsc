package types

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"sort"
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
	// leafPrefix domain-separates reward-leaf hashes from inner-node hashes.
	leafPrefix byte = 0x00
	// innerPrefix domain-separates inner-node hashes from leaf hashes.
	innerPrefix byte = 0x01
	// headerPrefix domain-separates header-leaf hashes from reward leaves and
	// inner nodes, so a header leaf can never be reinterpreted as a reward leaf.
	headerPrefix byte = 0x02
)

// MaxProofDepth bounds the number of sibling hashes a claim proof may carry. A
// proof's length equals the tree depth (log2 of the leaf count), so 64 supports
// up to 2^64 leaves — far beyond any real distribution. The cap rejects
// pathologically long proofs whose hash folding would otherwise be CPU work that
// is not gas-metered per sibling.
const MaxProofDepth = 64

// MaxCategories bounds how many reward categories a single claim leaf may carry.
// The leaf commits to the category map, so an over-sized map cannot make a claim
// succeed — but the handler validates and sorts the map (in LeafHash) before the
// proof is verified, so the cap stops a bogus claim from forcing that work. Real
// distributions use a small, fixed set of categories; 100 is a generous bound.
const MaxCategories = 100

// LeafHash returns the leaf hash for a single reward entry.
//
// The leaf preimage is:
//
//	0x00 || uint64BE(nonce)
//	     || uint32BE(len(addr))  || addr
//	     || uint32BE(len(total)) || total
//	     || uint32BE(numCategories)
//	     || for each (key,value) sorted ascending by key:
//	          uint32BE(len(key)) || key || uint32BE(len(value)) || value
//
// Length-prefixing every variable-length field makes the boundaries between
// them unambiguous. Categories are emitted in ascending key order so the leaf
// is independent of the map's iteration order. The leaf commits to the full
// category breakdown, so a claimer cannot present a different breakdown than the
// one the canonical root was built from.
//
// IMPORTANT: the off-chain node that builds the distribution roots MUST encode
// the categories in this exact (sorted, length-prefixed) form or every on-chain
// proof will fail.
func LeafHash(nonce uint64, addr, total string, categories map[string]string) []byte {
	buf := make([]byte, 0, 1+8+4+len(addr)+4+len(total)+4)
	buf = append(buf, leafPrefix)

	var n [8]byte
	binary.BigEndian.PutUint64(n[:], nonce)
	buf = append(buf, n[:]...)

	buf = appendLenPrefixed(buf, []byte(addr))
	buf = appendLenPrefixed(buf, []byte(total))

	keys := make([]string, 0, len(categories))
	for k := range categories {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var count [4]byte
	binary.BigEndian.PutUint32(count[:], uint32(len(keys)))
	buf = append(buf, count[:]...)

	for _, k := range keys {
		buf = appendLenPrefixed(buf, []byte(k))
		buf = appendLenPrefixed(buf, []byte(categories[k]))
	}

	sum := sha256.Sum256(buf)
	return sum[:]
}

// HeaderLeafHash returns the hash of the header leaf for a distribution. The
// header leaf commits to the distribution's type, date, version, and the
// authoritative per-category totals, so a submitted root that includes this leaf
// is bound to exactly those totals (validated against the params percentages).
//
// The leaf preimage is:
//
//	0x02 || uint32BE(len(distroType)) || distroType
//	     || uint32BE(len(date))       || date
//	     || uint64BE(version)
//	     || uint32BE(numCategories)
//	     || for each (key,value) sorted ascending by key:
//	          uint32BE(len(key)) || key || uint32BE(len(value)) || value
//
// The encoding mirrors LeafHash (length-prefixed, ascending-sorted categories)
// under a distinct domain prefix. Off-chain producers MUST encode the header leaf
// this way or submission proofs will fail.
func HeaderLeafHash(distroType, date string, version uint64, totalsByCategory map[string]string) []byte {
	buf := make([]byte, 0, 1+4+len(distroType)+4+len(date)+8+4)
	buf = append(buf, headerPrefix)

	buf = appendLenPrefixed(buf, []byte(distroType))
	buf = appendLenPrefixed(buf, []byte(date))

	var v [8]byte
	binary.BigEndian.PutUint64(v[:], version)
	buf = append(buf, v[:]...)

	keys := make([]string, 0, len(totalsByCategory))
	for k := range totalsByCategory {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var count [4]byte
	binary.BigEndian.PutUint32(count[:], uint32(len(keys)))
	buf = append(buf, count[:]...)

	for _, k := range keys {
		buf = appendLenPrefixed(buf, []byte(k))
		buf = appendLenPrefixed(buf, []byte(totalsByCategory[k]))
	}

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
