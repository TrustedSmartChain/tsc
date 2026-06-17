package types_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// FuzzLeafHashNoPanic checks LeafHash never panics on arbitrary inputs and is
// deterministic. The seed corpus runs under `go test`; extended fuzzing with
// `go test -fuzz=FuzzLeafHashNoPanic`.
func FuzzLeafHashNoPanic(f *testing.F) {
	f.Add(uint64(0), "2025-07-22", "type1", "tsc1addr", "node", "1000", "aTSC")
	f.Add(uint64(1<<63), "", "", "", "", "", "")
	f.Fuzz(func(t *testing.T, nonce uint64, date, distroType, addr, category, amount, denom string) {
		h1 := types.LeafHash(nonce, date, distroType, addr, category, amount, denom)
		h2 := types.LeafHash(nonce, date, distroType, addr, category, amount, denom)
		require.Equal(t, h1, h2, "LeafHash must be deterministic")
		require.Len(t, h1, 32)
	})
}

// FuzzVerifyProofNoPanic checks VerifyProof tolerates arbitrary bytes without
// panicking (e.g. odd-length or empty hashes from a malformed claim).
func FuzzVerifyProofNoPanic(f *testing.F) {
	f.Add([]byte("root"), []byte("leaf"), []byte("sibling"))
	f.Add([]byte{}, []byte{}, []byte{})
	f.Fuzz(func(t *testing.T, root, leaf, sibling []byte) {
		_ = types.VerifyProof(root, leaf, [][]byte{sibling})
	})
}

// FuzzMerkleRoundTrip checks the commutative scheme's core property for
// arbitrary leaves: a correct proof always verifies, regardless of sibling
// order, and a leaf does not verify against the wrong root.
func FuzzMerkleRoundTrip(f *testing.F) {
	f.Add(uint64(0), "a", uint64(1), "b")
	f.Add(uint64(7), "tsc1x", uint64(7), "tsc1y")
	f.Fuzz(func(t *testing.T, n0 uint64, a0 string, n1 uint64, a1 string) {
		leaf0 := types.LeafHash(n0, "2025-07-22", "type1", a0, "c", "1", "aTSC")
		leaf1 := types.LeafHash(n1, "2025-07-22", "type1", a1, "c", "1", "aTSC")
		root := types.HashPair(leaf0, leaf1)

		// Both leaves verify against the root with the other as the sole sibling,
		// in either internal order (commutative hashing).
		require.True(t, types.VerifyProof(root, leaf0, [][]byte{leaf1}))
		require.True(t, types.VerifyProof(root, leaf1, [][]byte{leaf0}))

		// Unless the two leaves are identical, a leaf must not verify against the
		// root with an empty proof.
		if string(leaf0) != string(leaf1) {
			require.False(t, types.VerifyProof(root, leaf0, nil))
		}
	})
}
