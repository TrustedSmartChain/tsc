# Trusted Smart Chain

`tscd` is the node daemon and CLI for Trusted Smart Chain (TSC), a Cosmos SDK chain with a built-in EVM.

- **Cosmos SDK** v0.53, **CometBFT** v0.38, **cosmos/evm** v0.6
- **Native token:** TSC, base denom `aTSC` (18 decimals), fixed supply of 21,000,000 TSC
- **Addresses:** bech32 prefix `tsc`, `eth_secp256k1` keys, so one key works for Cosmos and EVM transactions

## Networks

| Network | Chain ID | EVM chain ID |
|---|---|---|
| Mainnet | `tsc_87878-1` | `87878` |
| Testnet | `tsc_8878788-4` | `8878788` |

Both IDs come from genesis. The EVM chain ID is the number after `_` in the chain ID, so one binary serves every network. A node won't start if `--chain-id` or `evm.evm-chain-id` in `app.toml` contradicts genesis.

## Modules

TSC-specific modules in [x/](x/):

- **`x/distro`**: issues new TSC on a halving schedule, capped at the fixed 21,000,000 TSC supply. It is the only module that mints.
- **`x/lockup`**: locks tokens until a date, optionally sending and delegating them in the same transaction.
- **`x/attestation`**: active nodes report RWA and RWU contract supply here. It enforces daily per-node limits on gasless attestations and records when each node last attested.

From the nodelabs SDK:

- **`x/license`**: license types and the licenses issued to operators.
- **`x/network`**: node types, activation keys, node activation and status. It also admits allowlisted messages with a zero fee (gasless) under per-node quotas.

The app also includes the standard SDK modules (bank, staking, gov, slashing, authz, feegrant, upgrade and others), IBC with ICA, and the EVM, fee market and ERC-20 modules from cosmos/evm. Unordered Cosmos transactions are enabled.

## Building

Requires Go 1.24.

```sh
make install        # installs tscd into $GOPATH/bin
make build          # builds ./build/tscd
make build-linux    # static linux/amd64 binary at ./build/tscd-linux, built in Docker
```

`make build-linux` produces the same fully static binary as a release. It has no host dependencies. If Docker runs out of disk, the build can fail while leaving an older `build/tscd-linux` in place, so check the binary's version before shipping it.

## Running a local chain

```sh
make sh-testnet               # one validator
make sh-testnet-multi-node    # two validators
```

Both wrap [scripts/test_node.sh](scripts/test_node.sh) and [scripts/test_2node.sh](scripts/test_2node.sh). They create keys, write genesis and start the chain in `~/.tsc`. To customize, set environment variables when running the script directly:

```sh
CHAIN_ID="tsc_8878788-1" HOME_DIR="~/.tsc" BLOCK_TIME="1000ms" CLEAN=true sh scripts/test_node.sh
```

`CLEAN=true` wipes `HOME_DIR` and starts a fresh chain. The script header lists the remaining options, such as ports and block time.

Default local endpoints:

| Service | Address |
|---|---|
| CometBFT RPC | `localhost:26657` |
| REST | `localhost:1317` |
| gRPC | `localhost:9090` |
| EVM JSON-RPC | `localhost:8545` |

## Testing

```sh
make test           # unit tests
make test-race      # unit tests with the race detector
make test-cover     # unit tests with coverage
```

The `app` package's tests each build a full app, and cosmos/evm sets its chain config once per process. Running two of them in one process panics with "chainConfig already set", so run `app` tests one at a time with `-run`.

[scripts/test_upgrade.sh](scripts/test_upgrade.sh) and [scripts/test_upgrade_docker.sh](scripts/test_upgrade_docker.sh) run a local chain on an older release and upgrade it through governance.

## Protobuf

Proto definitions are in [proto/](proto/). Generated Go code goes to `x/*/types` (gogoproto) and [api/](api/) (pulsar).

```sh
make proto-gen      # regenerate Go code (runs in Docker)
make proto-lint
```

`make proto-gen` also sweeps up any proto sources under the gitignored `build/` directory, such as the old release sources the upgrade scripts download. Move `build/` aside before regenerating, and revert any unrelated `.pb.go` changes the generator makes.

## Upgrades

Each chain upgrade has a handler in `app/upgrades_*.go`, named after the upgrade (`v4.2`, for example). Mainnet upgrades are applied by a governance software-upgrade proposal with that name. Operators running cosmovisor stage the new binary at `$DAEMON_HOME/cosmovisor/upgrades/<name>/bin/tscd`.

## Releases

Release binaries are built by the [Build Linux Binary](.github/workflows/build.yml) workflow, which is started manually:

- **`none`**: build and upload a workflow artifact only.
- **`nightly`**: replace the rolling `nightly` pre-release.
- **`release`**: publish `v<version>` from the selected commit, with `tscd-linux` and its SHA-256 checksum attached.
