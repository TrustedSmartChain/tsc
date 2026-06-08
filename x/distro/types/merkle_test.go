package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// cat is a small helper for a single-category breakdown whose amount equals the
// leaf total.
func cat(amount string) map[string]string {
	return map[string]string{"type1": amount}
}

func TestLeafHashDeterministic(t *testing.T) {
	a := types.LeafHash(1, "tsc1abc", "1000", cat("1000"))
	b := types.LeafHash(1, "tsc1abc", "1000", cat("1000"))
	require.Equal(t, a, b)
	require.Len(t, a, 32)
}

func TestLeafHashCategoryOrderIndependent(t *testing.T) {
	// The leaf is built from categories sorted by key, so map iteration order
	// must not affect the hash.
	a := types.LeafHash(1, "tsc1abc", "3000", map[string]string{"type1": "1000", "type2": "2000"})
	b := types.LeafHash(1, "tsc1abc", "3000", map[string]string{"type2": "2000", "type1": "1000"})
	require.Equal(t, a, b)
}

func TestLeafHashCommitsToCategories(t *testing.T) {
	// Same total, different category breakdown => different leaf.
	require.NotEqual(t,
		types.LeafHash(1, "tsc1abc", "3000", map[string]string{"type1": "1000", "type2": "2000"}),
		types.LeafHash(1, "tsc1abc", "3000", map[string]string{"type1": "2000", "type2": "1000"}),
	)
	// Same breakdown values, different category names => different leaf.
	require.NotEqual(t,
		types.LeafHash(1, "tsc1abc", "1000", map[string]string{"type1": "1000"}),
		types.LeafHash(1, "tsc1abc", "1000", map[string]string{"type2": "1000"}),
	)
}

func TestLeafHashFieldBoundaryUnambiguous(t *testing.T) {
	// Length-prefixing must keep ("ab","c") distinct from ("a","bc").
	require.NotEqual(t,
		types.LeafHash(1, "ab", "c", nil),
		types.LeafHash(1, "a", "bc", nil),
	)
	// Different nonce => different leaf.
	require.NotEqual(t,
		types.LeafHash(1, "tsc1abc", "1000", cat("1000")),
		types.LeafHash(2, "tsc1abc", "1000", cat("1000")),
	)
}

func TestHashPairCommutative(t *testing.T) {
	a := types.LeafHash(1, "a", "1", cat("1"))
	b := types.LeafHash(2, "b", "2", cat("2"))
	require.Equal(t, types.HashPair(a, b), types.HashPair(b, a))
}

func TestVerifyProofSingleLeaf(t *testing.T) {
	leaf := types.LeafHash(0, "tsc1only", "42", cat("42"))
	// A single-leaf tree: the root is the leaf, proof is empty.
	require.True(t, types.VerifyProof(leaf, leaf, nil))
}

func TestVerifyProofFourLeaves(t *testing.T) {
	l0 := types.LeafHash(0, "tsc1a", "10", cat("10"))
	l1 := types.LeafHash(1, "tsc1b", "20", cat("20"))
	l2 := types.LeafHash(2, "tsc1c", "30", cat("30"))
	l3 := types.LeafHash(3, "tsc1d", "40", cat("40"))

	n01 := types.HashPair(l0, l1)
	n23 := types.HashPair(l2, l3)
	root := types.HashPair(n01, n23)

	// Each leaf has a 2-element proof: its sibling, then the opposite subtree.
	require.True(t, types.VerifyProof(root, l0, [][]byte{l1, n23}))
	require.True(t, types.VerifyProof(root, l1, [][]byte{l0, n23}))
	require.True(t, types.VerifyProof(root, l2, [][]byte{l3, n01}))
	require.True(t, types.VerifyProof(root, l3, [][]byte{l2, n01}))
}

func TestVerifyProofRejectsTampering(t *testing.T) {
	l0 := types.LeafHash(0, "tsc1a", "10", cat("10"))
	l1 := types.LeafHash(1, "tsc1b", "20", cat("20"))
	l2 := types.LeafHash(2, "tsc1c", "30", cat("30"))
	l3 := types.LeafHash(3, "tsc1d", "40", cat("40"))
	root := types.HashPair(types.HashPair(l0, l1), types.HashPair(l2, l3))

	// Wrong amount for the claimed leaf.
	bad := types.LeafHash(0, "tsc1a", "9999", cat("9999"))
	require.False(t, types.VerifyProof(root, bad, [][]byte{l1, types.HashPair(l2, l3)}))

	// Correct leaf, corrupted sibling.
	require.False(t, types.VerifyProof(root, l0, [][]byte{l2, types.HashPair(l2, l3)}))

	// Correct leaf and proof against the wrong root.
	require.False(t, types.VerifyProof(l1, l0, [][]byte{l1, types.HashPair(l2, l3)}))
}
