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

// merkleVectorLeaf is a single (category, amount) reward entry — one leaf
// represents one category/amount for one earner on a given (date, distro type).
type merkleVectorLeaf struct {
	Nonce      uint64 `json:"nonce"`
	Date       string `json:"date"`
	DistroType string `json:"distro_type"`
	Address    string `json:"address"`
	Category   string `json:"category"`
	Amount     string `json:"amount"`
	Denom      string `json:"denom"`
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
			Leaves: []merkleVectorLeaf{{Nonce: 0, Date: "2025-07-22", DistroType: "type1", Address: "tsc1alice", Category: "node", Amount: "42", Denom: "aTSC"}},
		},
		{
			Name: "two_leaves",
			Leaves: []merkleVectorLeaf{
				{Nonce: 0, Date: "2025-07-22", DistroType: "type1", Address: "tsc1alice", Category: "node", Amount: "100", Denom: "aTSC"},
				{Nonce: 1, Date: "2025-07-22", DistroType: "type1", Address: "tsc1bob", Category: "node", Amount: "200", Denom: "aTSC"},
			},
		},
		{
			Name: "five_leaves",
			Note: "Odd leaf count: the unpaired trailing node at a level is promoted to the next level unchanged (no duplication), so proofs have unequal lengths.",
			Leaves: []merkleVectorLeaf{
				{Nonce: 0, Date: "2025-07-22", DistroType: "type1", Address: "tsc1alice", Category: "node", Amount: "10", Denom: "aTSC"},
				{Nonce: 1, Date: "2025-07-22", DistroType: "type1", Address: "tsc1bob", Category: "node", Amount: "20", Denom: "aTSC"},
				{Nonce: 2, Date: "2025-07-22", DistroType: "type1", Address: "tsc1carol", Category: "node", Amount: "30", Denom: "aTSC"},
				{Nonce: 3, Date: "2025-07-22", DistroType: "type1", Address: "tsc1dave", Category: "node", Amount: "40", Denom: "aTSC"},
				{Nonce: 4, Date: "2025-07-22", DistroType: "type1", Address: "tsc1erin", Category: "node", Amount: "50", Denom: "aTSC"},
			},
		},
		{
			Name: "mixed_categories",
			Note: "Leaves in different categories for the same earner each get their own nonce and leaf; the leaf commits to (nonce, date, distro_type, address, category, amount, denom), so changing any field changes the hash.",
			Leaves: []merkleVectorLeaf{
				{Nonce: 7, Date: "2025-07-22", DistroType: "type1", Address: "tsc1alice", Category: "team", Amount: "100", Denom: "aTSC"},
				{Nonce: 8, Date: "2025-07-22", DistroType: "type1", Address: "tsc1alice", Category: "node", Amount: "200", Denom: "aTSC"},
				{Nonce: 9, Date: "2025-07-22", DistroType: "type1", Address: "tsc1bob", Category: "node", Amount: "5", Denom: "aTSC"},
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
		"Each leaf is a single (category, amount) reward for one earner on a (date, distro type).",
		"Leaf hash:  sha256(0x00 || uint64BE(nonce) || lp(date) || lp(distro_type) || lp(address) || lp(category) || lp(amount) || lp(denom))",
		"            where lp(x) = uint32BE(len(x)) || x.",
		"Inner node: sha256(0x01 || min(a,b) || max(a,b))  (children sorted bytewise, so proofs need no direction bits).",
		"",
		"Tree construction: leaves are hashed in the order given; each level pairs adjacent",
		"nodes left to right, and an unpaired trailing node is promoted to the next level",
		"unchanged (no duplication). A leaf's proof is the bottom-up list of sibling hashes;",
		"a level where the node was promoted contributes no sibling. Verification folds the",
		"proof into the leaf with the inner-node rule and compares to the root.",
		"",
		"A claimable leaf has a positive amount and a non-empty category and denom; the",
		"submitted tree typically also contains a header leaf (domain prefix 0x02), which is",
		"not shown here — these vectors cover the reward-leaf encoding only.",
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
		leafHashes[i] = types.LeafHash(l.Nonce, l.Date, l.DistroType, l.Address, l.Category, l.Amount, l.Denom)
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
	for _, want := range []string{"single_leaf", "two_leaves", "five_leaves", "mixed_categories"} {
		require.True(t, names[want], "fixture is missing required case %q", want)
	}

	for _, c := range file.Cases {
		t.Run(c.Name, func(t *testing.T) {
			require.NotEmpty(t, c.Leaves)

			// Every fixture leaf must be a claimable reward (positive amount,
			// non-empty category and denom), so the vectors stay representative.
			for _, l := range c.Leaves {
				_, err := types.ValidateClaimAmount(l.Amount)
				require.NoError(t, err, "fixture leaf has an invalid amount")
				require.NotEmpty(t, l.Category, "fixture leaf has an empty category")
				require.NotEmpty(t, l.Denom, "fixture leaf has an empty denom")
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
				bad := types.LeafHash(c.Leaves[i].Nonce+1, c.Leaves[i].Date, c.Leaves[i].DistroType, c.Leaves[i].Address, c.Leaves[i].Category, c.Leaves[i].Amount, c.Leaves[i].Denom)
				require.False(t, types.VerifyProof(root, bad, proof))
			}
		})
	}
}
