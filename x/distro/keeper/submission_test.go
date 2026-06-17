package keeper_test

import (
	"strconv"

	"github.com/TrustedSmartChain/tsc/v3/x/distro/types"
)

// defaultType is the distribution type id seeded by types.DefaultParams(): a
// single type claiming 100% of the day's budget with one uncapped category
// ("default") and the license + stake consensus thresholds.
const defaultType = types.DefaultDistributionTypeID

// defaultHeaderTotals is a totals_by_category map valid for the default type
// (whose configured categories are "type1" and "type2"), kept well within any
// day budget. Submit validation restricts header categories to the type's
// configured set, so header totals must name configured categories.
func defaultHeaderTotals() map[string]string {
	return map[string]string{"type1": "1"}
}

// rewardLeaf describes a single (category, amount) reward entry (a LeafHash
// input) for buildSubmission. Each leaf represents one category/amount; the denom
// is testDenom (the module denom).
type rewardLeaf struct {
	nonce    uint64
	addr     string
	category string
	amount   string
}

// headerTotalsFromRewards sums the reward leaves' amounts per category, yielding
// header totals_by_category that exactly cover the rewards. Claim caps are taken
// from the stored header, so claim tests use this so every in-tree reward is
// claimable (and the header stays within the day budget). Inputs are small
// integers in tests, so int64 accumulation is sufficient.
func headerTotalsFromRewards(rewards []rewardLeaf) map[string]string {
	sums := map[string]int64{}
	for _, r := range rewards {
		n, _ := strconv.ParseInt(r.amount, 10, 64)
		sums[r.category] += n
	}
	totals := make(map[string]string, len(sums))
	for cat, n := range sums {
		totals[cat] = strconv.FormatInt(n, 10)
	}
	return totals
}

// buildMerkleTree returns the root and per-leaf inclusion proofs for the given
// ordered leaf hashes, using the canonical adjacent-pairing / sorted-pair scheme
// (the same construction the off-chain worker and merkle vectors use).
func buildMerkleTree(leafHashes [][]byte) (root []byte, proofs [][][]byte) {
	type node struct {
		hash   []byte
		leaves []int
	}

	proofs = make([][][]byte, len(leafHashes))
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

// buildSubmission builds a merkle tree whose first leaf is the header leaf
// (distroType, date, version, totals) followed by the given reward leaves, and
// returns the root, the header inclusion proof, and the per-reward inclusion
// proofs (aligned to the rewards slice). It is the single helper tests use to
// produce a MsgSubmitDistributionRoot that passes header verification, and the
// claim proofs that verify against the resulting root.
func buildSubmission(distroType, date string, version uint64, totals map[string]string, rewards []rewardLeaf) (root []byte, headerProof [][]byte, rewardProofs [][][]byte) {
	leaves := make([][]byte, 0, len(rewards)+1)
	leaves = append(leaves, types.HeaderLeafHash(distroType, date, version, totals))
	for _, r := range rewards {
		leaves = append(leaves, types.LeafHash(r.nonce, date, distroType, r.addr, r.category, r.amount, testDenom))
	}
	root, proofs := buildMerkleTree(leaves)
	headerProof = proofs[0]
	if len(proofs) > 1 {
		rewardProofs = proofs[1:]
	}
	return root, headerProof, rewardProofs
}

// headerOnlySubmission builds the simplest valid submission for consensus tests
// that never claim: a tree containing only the default-type header leaf, so the
// root equals that leaf and the header proof is empty. version distinguishes
// otherwise-identical roots (e.g. competing roots in a no-consensus test).
func headerOnlySubmission(date string, version uint64) (root []byte, headerProof [][]byte) {
	root, headerProof, _ = buildSubmission(defaultType, date, version, defaultHeaderTotals(), nil)
	return root, headerProof
}
