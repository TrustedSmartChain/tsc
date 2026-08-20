#!/bin/bash
# Test the software-upgrade path from the released v2.0.4 to the v3 upgrade in
# this working tree, end to end: set up a single-validator chain on the old
# binary, submit + pass a gov software-upgrade proposal named "v3", let the old
# node halt at the upgrade height, swap binaries, and verify the v3 handler ran.
#
# Examples:
#   sh scripts/test_upgrade.sh
#   KEEP_RUNNING=true sh scripts/test_upgrade.sh          # leave the upgraded node up
#   OLD_VERSION=v2.0.3 UPGRADE_DELTA=80 sh scripts/test_upgrade.sh
#
# The old binary comes from the GitHub release: the prebuilt tscd-linux asset
# when the host can run it, otherwise built from the release source tarball
# (cached under build/upgrade-test, so the from-source path is paid once).
# OLD_BINARY=/path/to/tscd skips the fetch entirely.
#
# This is deliberately NOT test_node.sh's genesis: the chain is initialized by
# the v2 binary with v2's own genesis shape (no license/network/attestation
# state, no catalog seeds) so that everything v3-shaped arrives the way it
# would on the real network — through the upgrade handler.
#
# HOME_DIR is wiped on every run. It defaults to a dedicated directory, and the
# ports default to a 3xxxx range, so a dev chain from test_node.sh can keep
# running alongside.

set -eu

export CHAIN_ID=${CHAIN_ID:-"tsc_8878788-1"}
export EVM_CHAIN_ID=${EVM_CHAIN_ID:-"8878788"}
export MONIKER="localvalidator"
export KEYALGO="eth_secp256k1"
export KEYRING=${KEYRING:-"test"}
export HOME_DIR=$(eval echo "${HOME_DIR:-"~/.tsc-upgrade-test"}")
export DENOM=${DENOM:-aTSC}

export OLD_VERSION=${OLD_VERSION:-"v2.0.4"}
export UPGRADE_NAME=${UPGRADE_NAME:-"v3"}
export GITHUB_REPO=${GITHUB_REPO:-"TrustedSmartChain/tsc"}

# Offset ports so this test never fights a test_node.sh chain on the defaults.
export RPC=${RPC:-"36657"}
export P2P=${P2P:-"36656"}
export GRPC=${GRPC:-"39090"}
export GRPC_WEB=${GRPC_WEB:-"39091"}
export REST=${REST:-"31317"}
export PROFF=${PROFF:-"36060"}
export ROSETTA=${ROSETTA:-"38080"}
export JSONRPC=${JSONRPC:-"38545"}
export JSONRPC_WS=${JSONRPC_WS:-"38546"}

export BLOCK_TIME=${BLOCK_TIME:-"1000ms"}
# The whole vote has to finish before the upgrade height arrives: if the plan
# height is already in the past when the proposal passes, MsgSoftwareUpgrade
# errors and gov marks the proposal FAILED. 30s voting at ~1.1s blocks passes
# around block 35; 60 blocks leaves real margin. Raise both together if you
# slow BLOCK_TIME down.
export VOTING_PERIOD=${VOTING_PERIOD:-"30s"}
export EXPEDITED_VOTING_PERIOD=${EXPEDITED_VOTING_PERIOD:-"15s"}
export UPGRADE_DELTA=${UPGRADE_DELTA:-"60"}

export DEPOSIT=${DEPOSIT:-"10000000"}          # 10x the 1000000 min_deposit below
export GAS_PRICES=${GAS_PRICES:-"1000000000"}  # 1 gwei — clears any fee floor, costs ~0.001 TSC per tx
export KEEP_RUNNING=${KEEP_RUNNING:-"false"}

export KEY="acc0"
export KEY2="acc1"
export KEY3="minting"

NODE="tcp://127.0.0.1:$RPC"

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(dirname "$SCRIPT_DIR")
WORK_DIR=${WORK_DIR:-"$REPO_ROOT/build/upgrade-test"}
OLD_BIN=${OLD_BINARY:-"$WORK_DIR/tscd-$OLD_VERSION"}
NEW_BIN=${NEW_BINARY:-"$WORK_DIR/tscd-$UPGRADE_NAME"}
OLD_LOG="$WORK_DIR/node-$OLD_VERSION.log"
NEW_LOG="$WORK_DIR/node-$UPGRADE_NAME.log"

command -v jq   > /dev/null 2>&1 || { echo >&2 "jq not installed. More info: https://stedolan.github.io/jq/download/"; exit 1; }
command -v curl > /dev/null 2>&1 || { echo >&2 "curl not installed."; exit 1; }

NODE_PID=""
die() { echo >&2 ""; echo >&2 "ERROR: $*"; exit 1; }
step() { echo ""; echo "=== $* ==="; }

cleanup() {
  code=$?
  if [ -n "$NODE_PID" ] && kill -0 "$NODE_PID" 2>/dev/null; then
    kill "$NODE_PID" 2>/dev/null || true
    wait "$NODE_PID" 2>/dev/null || true
  fi
  if [ $code -ne 0 ]; then
    echo >&2 "Node logs: $OLD_LOG"
    echo >&2 "           $NEW_LOG"
  fi
}
trap cleanup EXIT

rpc_height() {
  h=$(curl -s "http://127.0.0.1:$RPC/status" 2>/dev/null | jq -r '.result.sync_info.latest_block_height // "0"' 2>/dev/null) || h="0"
  case "$h" in ''|null) h="0" ;; esac
  echo "$h"
}

# wait_for_height <target> <timeout-seconds>
wait_for_height() {
  target=$1; timeout=$2; waited=0
  while :; do
    h=$(rpc_height)
    if [ "$h" -ge "$target" ] 2>/dev/null; then echo "$h"; return 0; fi
    [ $waited -ge $timeout ] && die "timed out after ${timeout}s waiting for height $target (at $h)"
    sleep 2; waited=$((waited+2))
  done
}

# wait_for_tx <binary> <txhash> <label> — blocks until the tx is in a block,
# dies if it never lands or landed with a non-zero code. Echoes the tx JSON.
wait_for_tx() {
  bin=$1; hash=$2; label=$3; out=""
  for _ in $(seq 1 30); do
    out=$("$bin" q tx "$hash" --node "$NODE" -o json 2>/dev/null) && break
    out=""
    sleep 1
  done
  [ -n "$out" ] || die "$label tx $hash never made it into a block"
  code=$(echo "$out" | jq -r '.code')
  [ "$code" = "0" ] || die "$label tx failed (code $code): $(echo "$out" | jq -r '.raw_log')"
  echo "$out"
}

# ---------------------------------------------------------------------------
step "Fetching $OLD_VERSION binary"

mkdir -p "$WORK_DIR"
# OLD_FROM_RELEASE=true forces the release-asset download even when uname
# does not say linux/x86_64 — the docker wrapper uses it to run the amd64
# release binary under binfmt emulation inside a native-arch container.
if [ -x "$OLD_BIN" ]; then
  echo "Using cached $OLD_BIN"
elif [ "$(uname -s)" = "Linux" ] && { [ "$(uname -m)" = "x86_64" ] || [ "${OLD_FROM_RELEASE:-false}" = "true" ]; }; then
  echo "Downloading tscd-linux release asset..."
  curl -fL "https://github.com/$GITHUB_REPO/releases/download/$OLD_VERSION/tscd-linux" -o "$OLD_BIN" \
    || die "failed to download the $OLD_VERSION release asset"
  chmod +x "$OLD_BIN"
else
  # The release only ships a linux/amd64 binary, so everywhere else the old
  # binary is built from the release source tarball. The tarball has no .git,
  # so the Makefile's git-describe VERSION comes out empty — harmless here,
  # upgrade scheduling keys off handler names, not version strings.
  command -v go > /dev/null 2>&1 || die "go toolchain required to build $OLD_VERSION from source"
  SRC_DIR="$WORK_DIR/src-$OLD_VERSION"
  if [ ! -f "$SRC_DIR/Makefile" ]; then
    echo "Downloading $OLD_VERSION source tarball..."
    mkdir -p "$SRC_DIR"
    curl -fL "https://github.com/$GITHUB_REPO/archive/refs/tags/$OLD_VERSION.tar.gz" \
      | tar -xz -C "$SRC_DIR" --strip-components=1 \
      || die "failed to download/extract the $OLD_VERSION source tarball"
  fi
  echo "Building $OLD_VERSION from source (cached after the first run)..."
  make -C "$SRC_DIR" build
  cp "$SRC_DIR/build/tscd" "$OLD_BIN"
fi
echo "old binary: $("$OLD_BIN" version 2>&1 | head -1 || true)"

# ---------------------------------------------------------------------------
step "Building $UPGRADE_NAME binary from the working tree"

# NEW_BINARY is how the docker wrapper runs this: the upgraded binary is baked
# into the image, and there is no source tree inside the container to build.
if [ -n "${NEW_BINARY:-}" ]; then
  echo "NEW_BINARY set — using $NEW_BIN"
else
  make -C "$REPO_ROOT" build
  cp "$REPO_ROOT/build/tscd" "$NEW_BIN"
fi
echo "new binary: $("$NEW_BIN" version 2>&1 | head -1 || true)"

# ---------------------------------------------------------------------------
step "Initializing a fresh $OLD_VERSION chain at $HOME_DIR"

# Refuse to start on a port something else is already answering on — an
# already-running node would make every check below lie.
if curl -s "http://127.0.0.1:$RPC/status" > /dev/null 2>&1; then
  die "something is already serving RPC on port $RPC — stop it or set RPC="
fi

if [ ${#HOME_DIR} -le 2 ]; then
  die "HOME_DIR must be more than 2 characters long"
fi
rm -rf "$HOME_DIR" && echo "Removed $HOME_DIR"

add_key() {
  echo "$2" | "$OLD_BIN" keys add "$1" --keyring-backend $KEYRING --algo $KEYALGO --home "$HOME_DIR" --recover
}

# These are the v2-era test keys, not test_node.sh's current ones — the chain
# is initialized by the v2 binary, so its own conventions apply. KEY3 matters:
# it derives tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4, the exact address the
# v3 handler seeds as license/network module owner, so the post-upgrade runbook
# (grants, license types, node types) is signable from this keyring.
# tsc140fehngcrxvhdt84x729p3f0qmkmea8nynd44p
add_key $KEY "decorate bright ozone fork gallery riot bus exhaust worth way bone indoor calm squirrel merry zero scheme cotton until shop any excess stage laundry"
# tsc1r6yue0vuyj9m7xw78npspt9drq2tmtvggwt5x2
add_key $KEY2 "wealth flavor believe regret funny network recall kiss grape useless pepper cram hint member few certain unveil rather brick bargain curious require crowd raise"
# tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4
add_key $KEY3 "tilt steel wet bottom afraid return thrive wrestle camera bitter tape pretty"

"$OLD_BIN" init $MONIKER --chain-id "$CHAIN_ID" --default-denom $DENOM --home "$HOME_DIR"

update_test_genesis () {
  cat "$HOME_DIR/config/genesis.json" | jq "$1" > "$HOME_DIR/config/tmp_genesis.json" && mv "$HOME_DIR/config/tmp_genesis.json" "$HOME_DIR/config/genesis.json"
}

# v2's own test genesis, with the voting periods parameterized. Nothing
# v3-shaped goes in here on purpose.
update_test_genesis '.consensus_params["block"]["max_gas"]="100000000"'

update_test_genesis `printf '.app_state["gov"]["params"]["min_deposit"]=[{"denom":"%s","amount":"1000000"}]' $DENOM`
update_test_genesis `printf '.app_state["gov"]["params"]["voting_period"]="%s"' $VOTING_PERIOD`
update_test_genesis `printf '.app_state["gov"]["params"]["expedited_voting_period"]="%s"' $EXPEDITED_VOTING_PERIOD`

update_test_genesis `printf '.app_state["evm"]["params"]["evm_denom"]="%s"' $DENOM`
update_test_genesis '.app_state["evm"]["params"]["active_static_precompiles"]=["0x0000000000000000000000000000000000000100","0x0000000000000000000000000000000000000400","0x0000000000000000000000000000000000000800","0x0000000000000000000000000000000000000801","0x0000000000000000000000000000000000000802","0x0000000000000000000000000000000000000803","0x0000000000000000000000000000000000000804","0x0000000000000000000000000000000000000805","0x0000000000000000000000000000000000000900"]'
update_test_genesis '.app_state["feemarket"]["params"]["no_base_fee"]=true'
update_test_genesis '.app_state["feemarket"]["params"]["base_fee"]="0.000000000000000000"'

update_test_genesis `printf '.app_state["staking"]["params"]["bond_denom"]="%s"' $DENOM`
update_test_genesis '.app_state["staking"]["params"]["min_commission_rate"]="0.050000000000000000"'

update_test_genesis `printf '.app_state["mint"]["params"]["mint_denom"]="%s"' $DENOM`

update_test_genesis `printf '.app_state["crisis"]["constant_fee"]={"denom":"%s","amount":"1000"}' $DENOM`

update_test_genesis '.consensus["params"]["abci"]["vote_extensions_enable_height"]="1"'

update_test_genesis '.app_state["tokenfactory"]["params"]["denom_creation_fee"]=[]'
update_test_genesis '.app_state["tokenfactory"]["params"]["denom_creation_gas_consume"]=100000'

BASE_GENESIS_ALLOCATIONS="10000000000000000000000000$DENOM"
"$OLD_BIN" genesis add-genesis-account $KEY  $BASE_GENESIS_ALLOCATIONS --keyring-backend $KEYRING --home "$HOME_DIR" --append
"$OLD_BIN" genesis add-genesis-account $KEY2 $BASE_GENESIS_ALLOCATIONS --keyring-backend $KEYRING --home "$HOME_DIR" --append
"$OLD_BIN" genesis add-genesis-account $KEY3 $BASE_GENESIS_ALLOCATIONS --keyring-backend $KEYRING --home "$HOME_DIR" --append

# $KEY is the sole validator, so it alone clears quorum when it votes below.
# --min-self-delegation: v2.0.4 introduced the app/hooks floor of 500 TSC and
# rejects the gentx at InitChain without it; v2.0.3 accepts the flag but does
# not require it, so passing it keeps every old version bootable.
"$OLD_BIN" genesis gentx $KEY 500000000000000000000$DENOM --min-self-delegation 500000000000000000000 --gas-prices 0${DENOM} --keyring-backend $KEYRING --chain-id "$CHAIN_ID" --home "$HOME_DIR"
"$OLD_BIN" genesis collect-gentxs --home "$HOME_DIR"
"$OLD_BIN" genesis validate-genesis --home "$HOME_DIR" || die "failed to validate genesis"

# Move every listener onto this test's ports and speed the chain up.
sed -i -e 's/laddr = "tcp:\/\/127.0.0.1:26657"/laddr = "tcp:\/\/127.0.0.1:'$RPC'"/g' "$HOME_DIR/config/config.toml"
sed -i -e 's/laddr = "tcp:\/\/0.0.0.0:26656"/laddr = "tcp:\/\/0.0.0.0:'$P2P'"/g' "$HOME_DIR/config/config.toml"
sed -i -e 's/pprof_laddr = "localhost:6060"/pprof_laddr = "localhost:'$PROFF'"/g' "$HOME_DIR/config/config.toml"
sed -i -e 's/timeout_commit = "5s"/timeout_commit = "'$BLOCK_TIME'"/g' "$HOME_DIR/config/config.toml"
sed -i -e 's/address = "localhost:9090"/address = "localhost:'$GRPC'"/g' "$HOME_DIR/config/app.toml"
sed -i -e 's/address = "localhost:9091"/address = "localhost:'$GRPC_WEB'"/g' "$HOME_DIR/config/app.toml"
sed -i -e 's/address = "tcp:\/\/localhost:1317"/address = "tcp:\/\/localhost:'$REST'"/g' "$HOME_DIR/config/app.toml"
sed -i -e 's/address = ":8080"/address = ":'$ROSETTA'"/g' "$HOME_DIR/config/app.toml"
sed -i -e 's/^address = "127.0.0.1:8545"/address = "127.0.0.1:'$JSONRPC'"/g' "$HOME_DIR/config/app.toml"
sed -i -e 's/^ws-address = "127.0.0.1:8546"/ws-address = "127.0.0.1:'$JSONRPC_WS'"/g' "$HOME_DIR/config/app.toml"

# ---------------------------------------------------------------------------
step "Starting the $OLD_VERSION node"

"$OLD_BIN" start --home "$HOME_DIR" --chain-id "$CHAIN_ID" \
  --pruning=nothing --minimum-gas-prices=0$DENOM \
  --rpc.laddr "tcp://127.0.0.1:$RPC" \
  > "$OLD_LOG" 2>&1 &
NODE_PID=$!
echo "pid $NODE_PID, log $OLD_LOG"

# 120s covers the slowest legitimate startup: the old binary running under
# qemu binfmt emulation in the dockerized variant of this test.
wait_for_height 3 120 > /dev/null
CURRENT_HEIGHT=$(rpc_height)
UPGRADE_HEIGHT=$((CURRENT_HEIGHT + UPGRADE_DELTA))
echo "chain is producing blocks (height $CURRENT_HEIGHT); upgrade height will be $UPGRADE_HEIGHT"

# ---------------------------------------------------------------------------
step "Submitting software-upgrade proposal \"$UPGRADE_NAME\" for height $UPGRADE_HEIGHT"

# --no-validate skips the cosmovisor binary-checksum validation of
# --upgrade-info; this test swaps binaries by hand, so there is nothing there
# to validate.
TXHASH=$("$OLD_BIN" tx upgrade software-upgrade "$UPGRADE_NAME" \
  --title "$UPGRADE_NAME" \
  --summary "Upgrade to $UPGRADE_NAME" \
  --upgrade-height "$UPGRADE_HEIGHT" \
  --upgrade-info "test upgrade to $UPGRADE_NAME" \
  --no-validate \
  --deposit ${DEPOSIT}$DENOM \
  --from $KEY --keyring-backend $KEYRING --home "$HOME_DIR" \
  --chain-id "$CHAIN_ID" --node "$NODE" \
  --gas 500000 --gas-prices ${GAS_PRICES}$DENOM \
  -y -o json | jq -r '.txhash')

TX_JSON=$(wait_for_tx "$OLD_BIN" "$TXHASH" "submit-proposal")
PROP_ID=$(echo "$TX_JSON" | jq -r '[.events[] | select(.type=="submit_proposal") | .attributes[] | select(.key=="proposal_id") | .value][0] // empty')
if [ -z "$PROP_ID" ]; then
  PROP_ID=$("$OLD_BIN" q gov proposals --node "$NODE" -o json | jq -r '.proposals[-1].id // .proposals[-1].proposal_id')
fi
[ -n "$PROP_ID" ] && [ "$PROP_ID" != "null" ] || die "could not determine the proposal id"
echo "proposal $PROP_ID submitted in tx $TXHASH"

# ---------------------------------------------------------------------------
step "Voting yes on proposal $PROP_ID"

TXHASH=$("$OLD_BIN" tx gov vote "$PROP_ID" yes \
  --from $KEY --keyring-backend $KEYRING --home "$HOME_DIR" \
  --chain-id "$CHAIN_ID" --node "$NODE" \
  --gas 500000 --gas-prices ${GAS_PRICES}$DENOM \
  -y -o json | jq -r '.txhash')
wait_for_tx "$OLD_BIN" "$TXHASH" "vote" > /dev/null
echo "vote in tx $TXHASH; waiting out the $VOTING_PERIOD voting period..."

waited=0
while :; do
  STATUS=$("$OLD_BIN" q gov proposal "$PROP_ID" --node "$NODE" -o json | jq -r '.proposal.status // .status')
  case "$STATUS" in
    *PASSED|3) echo "proposal $PROP_ID PASSED"; break ;;
    *REJECTED|4) die "proposal $PROP_ID was REJECTED" ;;
    # FAILED here almost always means the upgrade height was already in the
    # past when the vote closed — raise UPGRADE_DELTA.
    *FAILED|5) die "proposal $PROP_ID FAILED at execution" ;;
  esac
  [ $waited -ge 180 ] && die "proposal $PROP_ID still $STATUS after 180s"
  sleep 2; waited=$((waited+2))
done

# ---------------------------------------------------------------------------
step "Waiting for the $OLD_VERSION node to halt at height $UPGRADE_HEIGHT"

# x/upgrade's BeginBlocker panics at the plan height because the v2 binary has
# no "v3" handler registered — that panic IS the correct behavior under test.
# Do not wait for the process to die, though: cometbft catches the panic
# (CONSENSUS FAILURE), stops consensus one block short of the plan height, and
# keeps the process alive serving RPC. Watch the log for the upgrade message,
# then stop the node ourselves — the same judgment call cosmovisor automates.
waited=0
while :; do
  grep -qF "UPGRADE \"$UPGRADE_NAME\" NEEDED" "$OLD_LOG" && break
  kill -0 "$NODE_PID" 2>/dev/null || break
  [ $waited -ge 300 ] && die "v2 node is still running well past the upgrade height — check $OLD_LOG"
  sleep 2; waited=$((waited+2))
done
if kill -0 "$NODE_PID" 2>/dev/null; then
  kill "$NODE_PID" 2>/dev/null || true
fi
wait "$NODE_PID" 2>/dev/null || true
NODE_PID=""

grep -qF "UPGRADE \"$UPGRADE_NAME\" NEEDED" "$OLD_LOG" \
  || die "the v2 node stopped, but not for the upgrade — check $OLD_LOG"
PLAN_NAME=$(jq -r '.name' "$HOME_DIR/data/upgrade-info.json" 2>/dev/null || echo "")
[ "$PLAN_NAME" = "$UPGRADE_NAME" ] || die "upgrade-info.json names \"$PLAN_NAME\", expected \"$UPGRADE_NAME\""
echo "v2 node halted with UPGRADE \"$UPGRADE_NAME\" NEEDED, upgrade-info.json written"

# ---------------------------------------------------------------------------
step "Restarting on the $UPGRADE_NAME binary"

# --evm.evm-chain-id is passed explicitly: this app.toml was written by the v2
# binary, and start must not depend on what an older build put there.
"$NEW_BIN" start --home "$HOME_DIR" --chain-id "$CHAIN_ID" \
  --pruning=nothing --minimum-gas-prices=0$DENOM \
  --rpc.laddr "tcp://127.0.0.1:$RPC" \
  --evm.evm-chain-id "$EVM_CHAIN_ID" \
  > "$NEW_LOG" 2>&1 &
NODE_PID=$!
echo "pid $NODE_PID, log $NEW_LOG"

# Two blocks past the upgrade height proves the handler ran AND consensus kept
# going afterwards.
wait_for_height $((UPGRADE_HEIGHT + 2)) 120 > /dev/null
echo "chain resumed and is producing blocks past the upgrade height"

# ---------------------------------------------------------------------------
step "Verifying the upgrade"

APPLIED=$("$NEW_BIN" q upgrade applied "$UPGRADE_NAME" --node "$NODE" 2>/dev/null) \
  || die "q upgrade applied $UPGRADE_NAME says the upgrade was never applied"
echo "x/upgrade reports \"$UPGRADE_NAME\" applied:"
echo "$APPLIED" | head -3

grep -q "applying upgrade" "$NEW_LOG" && echo "handler log: $(grep -m1 'applying upgrade' "$NEW_LOG" | head -1)"

# The v3 handler's observable output: the three new modules answer queries and
# carry the seeded params. Informational — command shapes may drift — but all
# three failing after "applied" succeeded would be worth a look.
for m in license network attestation; do
  echo ""
  echo "q $m params:"
  "$NEW_BIN" q $m params --node "$NODE" -o json 2>/dev/null | jq . || echo "  (query failed — check the $m module CLI)"
done

# ---------------------------------------------------------------------------
step "SUCCESS: $OLD_VERSION -> $UPGRADE_NAME upgrade path verified"

echo "proposal:       $PROP_ID"
echo "upgrade height: $UPGRADE_HEIGHT"
echo "chain height:   $(rpc_height)"
echo "home:           $HOME_DIR"

if [ "$KEEP_RUNNING" != "false" ]; then
  echo ""
  echo "Leaving the upgraded node running (pid $NODE_PID, rpc $NODE, log $NEW_LOG)."
  echo "Stop it with: kill $NODE_PID"
  NODE_PID=""
else
  echo ""
  echo "Stopping the node. Restart the upgraded chain any time with:"
  echo "  $NEW_BIN start --home $HOME_DIR --chain-id $CHAIN_ID --evm.evm-chain-id $EVM_CHAIN_ID --rpc.laddr tcp://127.0.0.1:$RPC --minimum-gas-prices=0$DENOM"
fi
