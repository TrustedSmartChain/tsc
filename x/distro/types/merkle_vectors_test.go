package types_test

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// Golden merkle vectors for off-chain worker implementations.
//
// testdata/merkle_vectors.json is generated FROM the canonical implementation
// in merkle.go (run `go test ./x/distro/types/ -run TestMerkleVectors -update-merkle-vectors`
// to regenerate) and TestMerkleVectors re-derives every leaf hash, root and
// proof from the stored inputs on every run — so the fixture can never silently
// drift from the code. External implementations of the distribution worker
// should validate their merkle code against this file.

// The flag is namespaced because a transitive dependency of the app (imported
// by setup_test.go) already registers a plain "update" flag.
var updateVectors = flag.Bool("update-merkle-vectors", false, "regenerate testdata/merkle_vectors.json from the canonical implementation")

const vectorsPath = "testdata/merkle_vectors.json"

type merkleVectorLeaf struct {
	Nonce      uint64            `json:"nonce"`
	Address    string            `json:"address"`
	Total      string            `json:"total"`
	Categories map[string]string `json:"categories"`
}

type merkleVectorCase struct {
	Name   string             `json:"name"`
	Note   string             `json:"note,omitempty"`
	Leaves []merkleVectorLeaf `json:"leaves"`
	// LeafHashes[i], Proofs[i] correspond to Leaves[i]; all hashes are lowercase hex.
	LeafHashes []string   `json:"leaf_hashes"`
	Root       string     `json:"root"`
	Proofs     [][]string `json:"proofs"`
}

type merkleVectorFile struct {
	Description []string           `json:"description"`
	Cases       []merkleVectorCase `json:"cases"`
}

// vectorInputs are the canonical case inputs; the expected hashes in the JSON
// fixture are computed from these by the implementation under -update.
func vectorInputs() []merkleVectorCase {
	return []merkleVectorCase{
		{
			Name:   "single_leaf",
			Note:   "A single-leaf tree: the root is the leaf hash itself and the proof is empty.",
			Leaves: []merkleVectorLeaf{{Nonce: 0, Address: "tsc1alice", Total: "42", Categories: map[string]string{"node": "42"}}},
		},
		{
			Name: "two_leaves",
			Leaves: []merkleVectorLeaf{
				{Nonce: 0, Address: "tsc1alice", Total: "100", Categories: map[string]string{"node": "100"}},
				{Nonce: 1, Address: "tsc1bob", Total: "200", Categories: map[string]string{"node": "200"}},
			},
		},
		{
			Name: "five_leaves",
			Note: "Odd leaf count: the unpaired trailing node at a level is promoted to the next level unchanged (no duplication), so proofs have unequal lengths.",
			Leaves: []merkleVectorLeaf{
				{Nonce: 0, Address: "tsc1alice", Total: "10", Categories: map[string]string{"node": "10"}},
				{Nonce: 1, Address: "tsc1bob", Total: "20", Categories: map[string]string{"node": "20"}},
				{Nonce: 2, Address: "tsc1carol", Total: "30", Categories: map[string]string{"node": "30"}},
				{Nonce: 3, Address: "tsc1dave", Total: "40", Categories: map[string]string{"node": "40"}},
				{Nonce: 4, Address: "tsc1erin", Total: "50", Categories: map[string]string{"node": "50"}},
			},
		},
		{
			Name: "multi_category",
			Note: "Categories are encoded sorted ascending by raw byte order: 'Zeta' (0x5A...) sorts before 'alpha' (0x61...), and 'a10' sorts before 'a2'. Encoding them in any other order produces a different leaf hash.",
			Leaves: []merkleVectorLeaf{
				{Nonce: 7, Address: "tsc1alice", Total: "600", Categories: map[string]string{"Zeta": "100", "alpha": "200", "beta": "300"}},
				{Nonce: 8, Address: "tsc1bob", Total: "33", Categories: map[string]string{"a10": "11", "a2": "22"}},
				{Nonce: 9, Address: "tsc1carol", Total: "5", Categories: map[string]string{"node": "5"}},
			},
		},
	}
}

func vectorDescription() []string {
	return []string{
		"Golden test vectors for the x/distro merkle scheme. Generated from the canonical",
		"implementation in x/distro/types/merkle.go (`go test ./x/distro/types/ -run TestMerkleVectors -update-merkle-vectors`);",
		"merkle_vectors_test.go re-derives every value from the inputs on each run, so this",
		"file cannot drift from the code. Off-chain workers must reproduce all of it exactly.",
		"",
		"Leaf hash:  sha256(0x00 || uint64BE(nonce) || lp(address) || lp(total) || uint32BE(numCategories)",
		"            || for each (key,value) sorted ascending by raw byte order of key: lp(key) || lp(value))",
		"            where lp(x) = uint32BE(len(x)) || x.",
		"Inner node: sha256(0x01 || min(a,b) || max(a,b))  (children sorted bytewise, so proofs need no direction bits).",
		"",
		"Tree construction: leaves are hashed in the order given; each level pairs adjacent",
		"nodes left to right, and an unpaired trailing node is promoted to the next level",
		"unchanged (no duplication). A leaf's proof is the bottom-up list of sibling hashes;",
		"a level where the node was promoted contributes no sibling. Verification folds the",
		"proof into the leaf with the inner-node rule and compares to the root.",
		"",
		"Empty categories are NOT a valid leaf: the chain rejects claims with an empty",
		"category map (types.ValidateClaimAmounts: 'categories must not be empty'), and the",
		"total must equal the sum of the category amounts. No vector covers an empty map",
		"because no such leaf can ever be claimed.",
	}
}

// buildTree constructs the canonical tree over the given leaf hashes and
// returns the root plus each leaf's proof (bottom-up sibling hashes). Levels
// pair adjacent nodes; an unpaired trailing node is promoted unchanged.
func buildTree(leafHashes [][]byte) ([]byte, [][][]byte) {
	type node struct {
		hash   []byte
		leaves []int // original leaf indices under this node
	}

	proofs := make([][][]byte, len(leafHashes))
	level := make([]node, len(leafHashes))
	for i, h := range leafHashes {
		level[i] = node{hash: h, leaves: []int{i}}
	}

	for len(level) > 1 {
		next := make([]node, 0, (len(level)+1)/2)
		for i := 0; i+1 < len(level); i += 2 {
			a, b := level[i], level[i+1]
			for _, li := range a.leaves {
				proofs[li] = append(proofs[li], b.hash)
			}
			for _, li := range b.leaves {
				proofs[li] = append(proofs[li], a.hash)
			}
			merged := append(append([]int{}, a.leaves...), b.leaves...)
			next = append(next, node{hash: types.HashPair(a.hash, b.hash), leaves: merged})
		}
		if len(level)%2 == 1 {
			next = append(next, level[len(level)-1])
		}
		level = next
	}
	return level[0].hash, proofs
}

// computeCase fills in the expected hashes for a case's inputs using the
// canonical implementation.
func computeCase(c merkleVectorCase) merkleVectorCase {
	leafHashes := make([][]byte, len(c.Leaves))
	c.LeafHashes = make([]string, len(c.Leaves))
	for i, l := range c.Leaves {
		leafHashes[i] = types.LeafHash(l.Nonce, l.Address, l.Total, l.Categories)
		c.LeafHashes[i] = hex.EncodeToString(leafHashes[i])
	}

	root, proofs := buildTree(leafHashes)
	c.Root = hex.EncodeToString(root)
	c.Proofs = make([][]string, len(proofs))
	for i, p := range proofs {
		c.Proofs[i] = make([]string, len(p))
		for j, sib := range p {
			c.Proofs[i][j] = hex.EncodeToString(sib)
		}
	}
	return c
}

func TestMerkleVectors(t *testing.T) {
	if *updateVectors {
		file := merkleVectorFile{Description: vectorDescription()}
		for _, c := range vectorInputs() {
			file.Cases = append(file.Cases, computeCase(c))
		}
		data, err := json.MarshalIndent(file, "", "  ")
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Dir(vectorsPath), 0o755))
		require.NoError(t, os.WriteFile(vectorsPath, append(data, '\n'), 0o644))
		t.Logf("regenerated %s", vectorsPath)
	}

	data, err := os.ReadFile(vectorsPath)
	require.NoError(t, err, "fixture missing — regenerate with: go test ./x/distro/types/ -run TestMerkleVectors -update-merkle-vectors")

	var file merkleVectorFile
	require.NoError(t, json.Unmarshal(data, &file))

	// The fixture must keep covering the canonical shapes.
	names := map[string]bool{}
	for _, c := range file.Cases {
		names[c.Name] = true
	}
	for _, want := range []string{"single_leaf", "two_leaves", "five_leaves", "multi_category"} {
		require.True(t, names[want], "fixture is missing required case %q", want)
	}

	for _, c := range file.Cases {
		t.Run(c.Name, func(t *testing.T) {
			require.NotEmpty(t, c.Leaves)

			// Every fixture leaf must be a claimable breakdown (positive amounts
			// summing to total), so the vectors stay representative of real claims.
			for _, l := range c.Leaves {
				_, err := types.ValidateClaimAmounts(l.Total, l.Categories)
				require.NoError(t, err, "fixture leaf is not a valid claim")
			}

			// Re-derive everything from the stored inputs and compare to the
			// stored expectations.
			recomputed := computeCase(merkleVectorCase{Name: c.Name, Leaves: c.Leaves})
			require.Equal(t, c.LeafHashes, recomputed.LeafHashes, "leaf hashes drifted from merkle.go")
			require.Equal(t, c.Root, recomputed.Root, "root drifted from merkle.go")
			require.Equal(t, c.Proofs, recomputed.Proofs, "proofs drifted from merkle.go")

			// Every stored proof must verify against the stored root via the
			// on-chain verifier.
			root, err := hex.DecodeString(c.Root)
			require.NoError(t, err)
			for i, leafHex := range c.LeafHashes {
				leaf, err := hex.DecodeString(leafHex)
				require.NoError(t, err)
				proof := make([][]byte, len(c.Proofs[i]))
				for j, sibHex := range c.Proofs[i] {
					proof[j], err = hex.DecodeString(sibHex)
					require.NoError(t, err)
				}
				require.True(t, types.VerifyProof(root, leaf, proof), "leaf %d proof does not verify", i)

				// And a tampered leaf must not verify with the same proof.
				bad := types.LeafHash(c.Leaves[i].Nonce+1, c.Leaves[i].Address, c.Leaves[i].Total, c.Leaves[i].Categories)
				require.False(t, types.VerifyProof(root, bad, proof))
			}
		})
	}
}
