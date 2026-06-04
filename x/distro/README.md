# x/distro

The `distro` module governs token distribution for Trusted Smart Chain. It has
two layers:

1. **Scheduled minting** — a halving-based emission schedule (`MsgMint`,
   governed by module `Params`).
2. **Decentralized distribution** — a node network reaches on-chain consensus on
   a daily reward set and users claim against it with merkle proofs.

## Decentralized distribution lifecycle

Each epoch (day, via `x/epochs`) moves through:

```
VOTING ──consensus──▶ PENDING ──review delay──▶ LIVE (claimable)
                         │
                    challenge (bond escrowed)
                         ▼
                   UNDER_REVIEW ──re-vote──▶ PENDING
                       • same root  → bond burned (frivolous)
                       • new root   → bond refunded, corrected root adopted
```

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
  amount)` against the canonical root. Rewards are minted on demand (max-supply
  checked) and each `(epoch, nonce)` can be claimed once.

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
