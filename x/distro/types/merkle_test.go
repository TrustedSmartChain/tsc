package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// Fixed reward-leaf fields used across the merkle tests.
const (
	leafDenom = "aTSC"
	leafDate  = "2025-07-22"
	leafType  = "type1"
)

func TestLeafHashDeterministic(t *testing.T) {
	a := types.LeafHash(1, leafDate, leafType, "tsc1abc", "type1", "1000", leafDenom)
	b := types.LeafHash(1, leafDate, leafType, "tsc1abc", "type1", "1000", leafDenom)
	require.Equal(t, a, b)
	require.Len(t, a, 32)
}

func TestLeafHashCommitsToFields(t *testing.T) {
	base := types.LeafHash(1, leafDate, leafType, "tsc1abc", "type1", "1000", leafDenom)
	// Each committed field changes the leaf when changed.
	require.NotEqual(t, base, types.LeafHash(2, leafDate, leafType, "tsc1abc", "type1", "1000", leafDenom)) // nonce
	require.NotEqual(t, base, types.LeafHash(1, leafDate, leafType, "tsc1xyz", "type1", "1000", leafDenom)) // address
	require.NotEqual(t, base, types.LeafHash(1, leafDate, leafType, "tsc1abc", "type2", "1000", leafDenom)) // category
	require.NotEqual(t, base, types.LeafHash(1, leafDate, leafType, "tsc1abc", "type1", "2000", leafDenom)) // amount
	require.NotEqual(t, base, types.LeafHash(1, leafDate, leafType, "tsc1abc", "type1", "1000", "other"))   // denom
}

func TestLeafHashFieldBoundaryUnambiguous(t *testing.T) {
	// Length-prefixing must keep ("ab","c") distinct from ("a","bc") across the
	// address/category boundary.
	require.NotEqual(t,
		types.LeafHash(1, leafDate, leafType, "ab", "c", "1", leafDenom),
		types.LeafHash(1, leafDate, leafType, "a", "bc", "1", leafDenom),
	)
}

func TestHeaderLeafHashDeterministic(t *testing.T) {
	totals := map[string]string{"a": "1", "b": "2"}
	h1 := types.HeaderLeafHash("type1", "2025-07-22", 3, totals)
	h2 := types.HeaderLeafHash("type1", "2025-07-22", 3, map[string]string{"b": "2", "a": "1"})
	require.Equal(t, h1, h2, "header leaf must be independent of map iteration order")
}

func TestHeaderLeafHashDomainSeparated(t *testing.T) {
	// A header leaf and a reward leaf with otherwise-similar inputs must not
	// collide: the header uses a distinct domain prefix (0x02).
	header := types.HeaderLeafHash("type1", "2025-07-22", 0, map[string]string{"x": "1"})
	reward := types.LeafHash(0, leafDate, leafType, "type1", "2025-07-22", "x", "1")
	require.NotEqual(t, header, reward)
}

func TestHeaderLeafHashCommitsToFields(t *testing.T) {
	base := types.HeaderLeafHash("type1", "2025-07-22", 1, map[string]string{"x": "1"})
	require.NotEqual(t, base, types.HeaderLeafHash("type2", "2025-07-22", 1, map[string]string{"x": "1"}))
	require.NotEqual(t, base, types.HeaderLeafHash("type1", "2025-07-23", 1, map[string]string{"x": "1"}))
	require.NotEqual(t, base, types.HeaderLeafHash("type1", "2025-07-22", 2, map[string]string{"x": "1"}))
	require.NotEqual(t, base, types.HeaderLeafHash("type1", "2025-07-22", 1, map[string]string{"x": "2"}))
}

// TestHeaderLeafInclusion checks a header leaf can be proven against a tree root
// using VerifyProof, mirroring the submit-time inclusion check.
func TestHeaderLeafInclusion(t *testing.T) {
	header := types.HeaderLeafHash("type1", "2025-07-22", 1, map[string]string{"type1": "100"})
	reward := types.LeafHash(0, leafDate, leafType, "tsc1a", "type1", "100", leafDenom)
	root := types.HashPair(header, reward)
	require.True(t, types.VerifyProof(root, header, [][]byte{reward}))
	require.True(t, types.VerifyProof(root, reward, [][]byte{header}))
}

func TestHashPairCommutative(t *testing.T) {
	a := types.LeafHash(1, leafDate, leafType, "a", "type1", "1", leafDenom)
	b := types.LeafHash(2, leafDate, leafType, "b", "type1", "2", leafDenom)
	require.Equal(t, types.HashPair(a, b), types.HashPair(b, a))
}

func TestVerifyProofSingleLeaf(t *testing.T) {
	leaf := types.LeafHash(0, leafDate, leafType, "tsc1only", "type1", "42", leafDenom)
	// A single-leaf tree: the root is the leaf, proof is empty.
	require.True(t, types.VerifyProof(leaf, leaf, nil))
}

func TestVerifyProofFourLeaves(t *testing.T) {
	l0 := types.LeafHash(0, leafDate, leafType, "tsc1a", "type1", "10", leafDenom)
	l1 := types.LeafHash(1, leafDate, leafType, "tsc1b", "type1", "20", leafDenom)
	l2 := types.LeafHash(2, leafDate, leafType, "tsc1c", "type1", "30", leafDenom)
	l3 := types.LeafHash(3, leafDate, leafType, "tsc1d", "type1", "40", leafDenom)

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
	l0 := types.LeafHash(0, leafDate, leafType, "tsc1a", "type1", "10", leafDenom)
	l1 := types.LeafHash(1, leafDate, leafType, "tsc1b", "type1", "20", leafDenom)
	l2 := types.LeafHash(2, leafDate, leafType, "tsc1c", "type1", "30", leafDenom)
	l3 := types.LeafHash(3, leafDate, leafType, "tsc1d", "type1", "40", leafDenom)
	root := types.HashPair(types.HashPair(l0, l1), types.HashPair(l2, l3))

	// Wrong amount for the claimed leaf.
	bad := types.LeafHash(0, leafDate, leafType, "tsc1a", "type1", "9999", leafDenom)
	require.False(t, types.VerifyProof(root, bad, [][]byte{l1, types.HashPair(l2, l3)}))

	// Correct leaf, corrupted sibling.
	require.False(t, types.VerifyProof(root, l0, [][]byte{l2, types.HashPair(l2, l3)}))

	// Correct leaf and proof against the wrong root.
	require.False(t, types.VerifyProof(l1, l0, [][]byte{l1, types.HashPair(l2, l3)}))
}
