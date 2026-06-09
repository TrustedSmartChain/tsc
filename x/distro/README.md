# x/distro

The `distro` module governs token distribution for Trusted Smart Chain. A node
network reaches on-chain consensus on a daily reward set and users claim against
it with merkle proofs. Emission follows the halving schedule encoded in module
`Params` (`distribution_start_date`, `max_supply`, `months_in_halving_period`):
each day's claimable total is capped at that day's halving allocation, and
claims mint on demand. (The legacy centralized `MsgMint` mint-to-receiving-address
flow has been retired in favor of this mechanism.)

All distribution state is keyed by **date** (`YYYY-MM-DD`). The `x/epochs` daily
hook is the time trigger; the module translates the fired epoch number into its
calendar day (`distribution_start_date` is day 1) and keys everything by that
date. Because `YYYY-MM-DD` sorts chronologically, ordered iteration over days is
just ordered iteration over keys.

## Decentralized distribution lifecycle

Each day moves through:

```
VOTING ──consensus──▶ PENDING ──review delay──▶ LIVE (claimable)
  │   ▲                  │
  │   │ gov revival      │ challenge (bond escrowed)
  │   │ (fresh window)   ▼
  │   │             UNDER_REVIEW ──re-vote──▶ PENDING (timer resumes, not reset)
  │ no consensus         • same root         → bond burned (frivolous)
  │ within window        • new root          → bond refunded, corrected root adopted
  ▼                      • no re-consensus   → times out after vote_window_days:
EXPIRED                    bond burned, original root restored, back to PENDING
  │
  └──MsgReviveDistribution (gov)──▶ VOTING
```

Each day's transition runs in its own cache context, so a single failing day
can neither roll back the others nor wedge the module — it is logged and
retried. A VOTING day that does not reach consensus within `vote_window_days`
is marked `EXPIRED` and stops being tallied. The voting window is measured from
`voting_since_date` (the day voting last opened), so a revived day gets a fresh
full window rather than being measured from its stale calendar date.

- **Submit** — each node runs the deterministic daily calculation and submits its
  merkle root via `MsgSubmitDistributionRoot`. The signer must hold a license of
  `params.distribution_license_type_id` that was **valid on the submitted day**.
  Votes are stored by `(date, signer)` and retained.
- **Tally** — at each epoch end the votes are tallied two ways: license-weighted
  and stake-weighted (per-address stake; a validator operator counts only its
  self-delegation). A root passing **both** thresholds (`license_tally_threshold`,
  `stake_tally_threshold`, default `0.667`) becomes the canonical root and enters
  `PENDING`.

  **License eligibility is judged as of the distribution day**, not the current
  block: a vote counts (and the license denominator includes a holder) only if the
  license's `[start_date, end_date]` window contains that day. So a license minted
  *after* a day cannot retroactively vote on, or challenge, that day's
  distribution. (Revocation closes `end_date` at the revocation day, so the window
  alone captures historical validity.) **Stake weight, by contrast, is read at the
  current block** — there is no per-day stake snapshot — so within the (bounded)
  voting window a voter's stake weight reflects its live bonded amount.
- **Review delay / challenge** — a `PENDING` root auto-promotes to `LIVE` after
  `review_delay_days` days. During that window any license holder may
  `MsgChallengeDistribution`, escrowing `challenge_bond` and reopening voting. The
  re-vote is the judge: re-confirming the same root burns the bond (frivolous);
  a corrected root refunds it and is adopted. If the re-vote never reaches
  consensus, the review **times out** after `vote_window_days` (measured from the
  challenge): the pre-challenge root is restored, the bond is burned, and the day
  returns to `PENDING` (resuming its original review timer). This bounds how long
  a challenge can hold a day under review, so a stalled re-vote can never lock the
  bond or the distribution permanently. If an upheld challenge's refund cannot be
  delivered (an unparseable or send-blocked challenger address), the bond is
  burned rather than erroring, so the day's transition always completes.
- **Revival** — an `EXPIRED` day (one that never reached consensus and forfeited
  its rewards) can be reopened by governance via `MsgReviveDistribution`. This is
  authority-gated because it un-forfeits a lapsed day. The day returns to
  `VOTING` with a fresh full `vote_window_days` window measured from the revival
  day; if it still fails to reach consensus it simply re-expires, and may be
  revived again. A revived day that reaches consensus mints from its original
  calendar day's halving budget.
- **Claim** — users call `MsgClaim` with a merkle proof of
  `(nonce, address, total, categories)` against the canonical root. `total` must
  equal the sum of the `categories` amounts, and the leaf commits to the full
  category breakdown, so it cannot be tampered with. The proof length and the
  category count are bounded (`MaxProofDepth`, `MaxCategories`) so a malformed
  claim cannot force unbounded pre-verification work. Rewards are minted on demand
  and each `(date, nonce)` can be claimed once. The cumulative amount claimed per
  day is capped at that day's halving budget (with `max_supply` as a final bound),
  so a finalized root can never mint beyond the day's emission allocation. Each
  claim also accumulates per-category running totals for the day (see
  `ClaimTotalByCategory`).

Distribution `status` values: `VOTING` (0), `LIVE` (1), `PENDING` (2),
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
| `review_delay_days` | uint64 | `3` | Days a root stays `PENDING` (challengeable) before auto-promoting to `LIVE`. Must be ≥ 1 (a zero delay would remove the challenge window). |
| `challenge_bond` | string int | `10000000000000000000` | Bond escrowed to challenge a `PENDING` root; burned if frivolous, else refunded. |
| `vote_window_days` | uint64 | `7` | Days a `VOTING` day may stay open before it `EXPIRED`s. |
| `minting_address`, `receiving_address` | string | — | Legacy; retained for genesis/param compatibility, unused since `MsgMint` was retired. |

## State

Collections (all under the `distro` store key):

- `Params` — `Item[Params]`.
- `Votes` — `Map[(date string, signer string) → DistributionVote]`. The raw
  per-signer submitted roots; **retained, never pruned** (audit trail).
- `Distributions` — `Map[date string → Distribution]`. The canonical per-day
  record: `merkle_root`, `status`, `license_tally`, `stake_tally`,
  `finalized_height`, `pending_since_date`, `challenger`, `challenge_bond`,
  `claimed_amount`, `voting_since_date` (when voting last (re)opened; the voting
  window is measured from this).
- `Claimed` — `KeySet[(date string, nonce uint64)]`. Spent reward nonces.
- `ClaimTotals` — `Map[(date string, category string) → CategoryClaimTotal]`.
  The cumulative amount claimed per reward category for a day, accumulated as
  rewards are claimed.
- `ActiveDistributions` — `KeySet[date string]`. The set of non-terminal
  (`VOTING`/`PENDING`/`UNDER_REVIEW`) days. The epoch hook iterates this set
  instead of scanning every distribution ever created, so per-epoch work is
  `O(open days)` rather than `O(all days)`. It is derived state (rebuilt from
  distribution statuses on genesis import and at the introducing upgrade), so it
  is **not** part of `GenesisState`.

All five are imported/exported via genesis; `GenesisState.Validate` enforces
structural consistency (roots present for non-`VOTING`/`EXPIRED` days, a
challenger on `UNDER_REVIEW`, parseable bond/claimed amounts, and that
`ClaimedRewards` only reference `LIVE` days).

## Messages

| Msg | Signer | Notes |
|---|---|---|
| `MsgSubmitDistributionRoot` | `signer` | License-gated; `date` must be in `[current − vote_window_days, current]` and ≥ the start date. Opens/updates a `VOTING` day. |
| `MsgChallengeDistribution` | `challenger` | License-gated; only on a `PENDING` day; escrows `challenge_bond`. |
| `MsgClaim` | `claimer` | Permissionless (proof-gated); pays the leaf's `address` the `total`, broken down by `categories` (which must sum to `total`). Requires `LIVE`. |
| `MsgReviveDistribution` | gov authority | Reopens an `EXPIRED` day for voting with a fresh window. Authority-gated (submitted via a gov proposal). |
| `MsgUpdateParams` | gov authority | Updates `Params`. |

## Queries

- `Params` — module parameters.
- `Distribution(date)` — the canonical record for a day.
- `DistributionVotes(date)` — submitted votes for a day (paginated).
- `Claimed(date, nonce)` — whether a reward nonce has been claimed.
- `ClaimsByDate(date)` — the reward nonces claimed for a day, ascending (paginated).
- `ClaimTotalByCategory(date)` — the cumulative claimed amount per category for a
  day.
- `Distributions` — all distributions, date-ordered and paginated.
- `ActiveDistributions` — the in-flight (`VOTING`/`PENDING`/`UNDER_REVIEW`) days,
  resolved from the active index (paginated).
- `Audit` — runs the module invariants against current state and returns each
  result (see Invariants).

## Invariants

The module registers two invariants (exercised by the simulation runner; no
crisis module is wired, so there is no runtime halt-on-break). The same checks
are exposed at runtime through `Query/Audit` so operators can verify a live node:

- **claim-budget** — for every day, cumulative `claimed_amount ≤ dateBudget(date)`.
  Guards against minting beyond the halving emission for any day.
- **bond-solvency** — the module account's balance is `≥` the sum of all
  outstanding (`UNDER_REVIEW`) `challenge_bond`s, i.e. every escrowed bond is
  fully backed and refundable.

## Merkle scheme

The off-chain node app and the chain must agree byte-for-byte. The tree is a
domain-separated SHA-256 binary tree with **commutative (sorted-pair)** inner
hashing, so a proof is just the ordered list of sibling hashes — no direction
bits. The leaf commits to the full per-category breakdown; categories are
emitted in ascending key order so the leaf is independent of map iteration order:

- **Leaf**:
  `sha256(0x00 || uint64BE(nonce) || lp(addr) || lp(total) || uint32BE(numCategories) || for each (key,value) sorted by key: lp(key) || lp(value))`
  where `lp(x) = uint32BE(len(x)) || x`.
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

**Node — vote each day** using its own key. The node builds a
`MsgSubmitDistributionRoot` whose `signer` is the **owner**, then wraps it in
`MsgExec`:

```bash
tscd tx authz exec submit_for_owner.json --from <node_key>
# submit_for_owner.json contains a tx with:
#   MsgSubmitDistributionRoot{ signer: <owner_addr>, date, merkle_root }
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
- **No double counting** — votes are keyed by `(date, owner)`, so an owner that
  both delegates and votes directly still contributes a single weight.
