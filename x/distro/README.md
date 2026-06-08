# x/distro

The `distro` module governs token distribution for Trusted Smart Chain. A node
network reaches on-chain consensus on a daily reward set and users claim against
it with merkle proofs. Emission follows the halving schedule encoded in module
`Params` (`distribution_start_date`, `max_supply`, `months_in_halving_period`):
each epoch's claimable total is capped at that day's halving allocation, and
claims mint on demand. (The legacy centralized `MsgMint` mint-to-receiving-address
flow has been retired in favor of this mechanism.)

## Decentralized distribution lifecycle

Each epoch (day, via `x/epochs`) moves through:

```
VOTING ──consensus──▶ PENDING ──review delay──▶ LIVE (claimable)
  │                      │
  │ no consensus         │ challenge (bond escrowed)
  │ within window        ▼
  ▼                 UNDER_REVIEW ──re-vote──▶ PENDING (timer resumes, not reset)
EXPIRED                 • same root  → bond burned (frivolous)
                        • new root   → bond refunded, corrected root adopted
```

Each epoch-end transition runs in its own cache context, so a single failing
epoch can neither roll back the others nor wedge the module — it is logged and
retried. A VOTING epoch that does not reach consensus within
`vote_window_epochs` is marked `EXPIRED` and stops being tallied.

- **Submit** — each node runs the deterministic daily calculation and submits its
  merkle root via `MsgSubmitDistributionRoot`. The signer must hold ≥1 active
  license of `params.distribution_license_type_id`. Votes are stored by
  `(epoch, signer)` and retained.
- **Tally** — at each epoch end the votes are tallied two ways: license-weighted
  and stake-weighted (per-address stake; a validator operator counts only its
  self-delegation). A root passing **both** thresholds (`license_tally_threshold`,
  `stake_tally_threshold`, default `0.667`) becomes the canonical root and enters
  `PENDING`.
- **Review delay / challenge** — a `PENDING` root auto-promotes to `LIVE` after
  `distribution_review_delay` epochs. During that window any license holder may
  `MsgChallengeDistribution`, escrowing `challenge_bond` and reopening voting. The
  re-vote is the judge: re-confirming the same root burns the bond (frivolous);
  a corrected root refunds it and is adopted.
- **Claim** — users call `MsgClaim` with a merkle proof of `(nonce, address,
  amount)` against the canonical root. Rewards are minted on demand and each
  `(epoch, nonce)` can be claimed once. The cumulative amount claimed per epoch
  is capped at that epoch's halving budget (with `max_supply` as a final bound),
  so a finalized root can never mint beyond the day's emission allocation.

Epoch-distribution `status` values: `VOTING` (0), `LIVE` (1), `PENDING` (2),
`UNDER_REVIEW` (3), `EXPIRED` (4).

## Parameters

| Param | Type | Default | Meaning |
|---|---|---|---|
| `denom` | string | `aTSC` | Distribution denom (must equal the base denom). |
| `max_supply` | string int | `21000000000000000000000000` | Hard supply ceiling; final bound on claim minting. |
| `distribution_start_date` | string `YYYY-MM-DD` | `2025-07-22` | Day 1 of the halving schedule. |
| `months_in_halving_period` | uint64 | `48` | Length of each halving period. |
| `distribution_license_type_id` | string | `tsc.node` | x/licenses `LicenseType` id a voter/challenger must hold (≥1 active). |
| `license_tally_threshold` | string dec (0,1] | `0.667` | License-weighted fraction a root must reach. |
| `stake_tally_threshold` | string dec (0,1] | `0.667` | Stake-weighted fraction (of total bonded) a root must reach. |
| `epoch_identifier` | string | `day` | x/epochs identifier whose `AfterEpochEnd` drives the lifecycle. |
| `distribution_review_delay` | uint64 | `3` | Epochs a root stays `PENDING` (challengeable) before auto-promoting to `LIVE`. `0` = instant. |
| `challenge_bond` | string int | `10000000000000000000` | Bond escrowed to challenge a `PENDING` root; burned if frivolous, else refunded. |
| `vote_window_epochs` | uint64 | `7` | Epochs a `VOTING` epoch may stay open before it `EXPIRED`s. |
| `minting_address`, `receiving_address` | string | — | Legacy; retained for genesis/param compatibility, unused since `MsgMint` was retired. |

## State

Collections (all under the `distro` store key):

- `Params` — `Item[Params]`.
- `Votes` — `Map[(epoch int64, signer string) → DistributionVote]`. The raw
  per-signer submitted roots; **retained, never pruned** (audit trail).
- `EpochDistributions` — `Map[epoch int64 → EpochDistribution]`. The canonical
  per-epoch record: `merkle_root`, `status`, `license_tally`, `stake_tally`,
  `finalized_height`, `pending_since_epoch`, `challenger`, `challenge_bond`,
  `claimed_amount`.
- `Claimed` — `KeySet[(epoch int64, nonce uint64)]`. Spent reward nonces.

All four are imported/exported via genesis; `GenesisState.Validate` enforces
structural consistency (roots present for non-`VOTING`/`EXPIRED` epochs, a
challenger on `UNDER_REVIEW`, parseable bond/claimed amounts, and that
`ClaimedRewards` only reference `LIVE` epochs).

## Messages

| Msg | Signer | Notes |
|---|---|---|
| `MsgSubmitDistributionRoot` | `signer` | License-gated; epoch must be in `[current − vote_window_epochs, current]`. Opens/updates a `VOTING` epoch. |
| `MsgChallengeDistribution` | `challenger` | License-gated; only on a `PENDING` epoch; escrows `challenge_bond`. |
| `MsgClaim` | `claimer` | Permissionless (proof-gated); pays the leaf's `address`. Requires `LIVE`. |
| `MsgUpdateParams` | gov authority | Updates `Params`. |

## Queries

- `Params` — module parameters.
- `EpochDistribution(epoch)` — the canonical record for an epoch.
- `DistributionVotes(epoch)` — all submitted votes for an epoch.
- `Claimed(epoch, nonce)` — whether a reward nonce has been claimed.

## Merkle scheme

The off-chain node app and the chain must agree byte-for-byte. The tree is a
domain-separated SHA-256 binary tree with **commutative (sorted-pair)** inner
hashing, so a proof is just the ordered list of sibling hashes — no direction
bits:

- **Leaf**: `sha256(0x00 || uint64BE(nonce) || uint32BE(len(addr)) || addr || uint32BE(len(amount)) || amount)`
- **Inner node**: `sha256(0x01 || min(a,b) || max(a,b))`
- **Verify**: fold the proof siblings into the leaf with the inner-node rule and
  compare to the canonical root.

See [types/merkle.go](types/merkle.go) (and `merkle_test.go` for vectors).

## Vote delegation via x/authz

Nodes run the distribution app and sign transactions, but operators should not
place their account keys on the nodes. Instead, each node has its own key and an
owner authorizes that node to vote on its behalf using the standard `x/authz`
module — no distro-specific delegation type is required.

The owner's licenses and stake are what count, because the vote is attributed to
the **inner message signer** (the owner), not the node that executes it.

**Owner — grant once** (the only step that uses the owner's key):

```bash
# Authorize the node to submit roots on the owner's behalf.
tscd tx authz grant <node_addr> generic \
  --msg-type-url /distro.v1.MsgSubmitDistributionRoot \
  --from <owner_key> [--expiration <unix_ts>]

# Optional: also authorize challenges.
tscd tx authz grant <node_addr> generic \
  --msg-type-url /distro.v1.MsgChallengeDistribution \
  --from <owner_key>
```

**Node — vote each epoch** using its own key. The node builds a
`MsgSubmitDistributionRoot` whose `signer` is the **owner**, then wraps it in
`MsgExec`:

```bash
tscd tx authz exec submit_for_owner.json --from <node_key>
# submit_for_owner.json contains a tx with:
#   MsgSubmitDistributionRoot{ signer: <owner_addr>, epoch, merkle_root }
```

`x/authz` verifies the node holds a grant from that owner and dispatches the
inner message with `signer = owner`; the distro handler checks the **owner's**
license and records the vote under the owner.

Notes:

- **Combined licenses + stake** — automatic. The vote carries the owner's whole
  identity, so both its license weight and stake weight are counted.
- **One node, many owners** — the node holds a grant from each owner and can
  batch several `MsgSubmitDistributionRoot` (one per owner) into a single
  `MsgExec`.
- **Revoke** — `tscd tx authz revoke <node_addr> /distro.v1.MsgSubmitDistributionRoot --from <owner_key>`;
  grants may also carry an expiration.
- **No double counting** — votes are keyed by `(epoch, owner)`, so an owner that
  both delegates and votes directly still contributes a single weight.
