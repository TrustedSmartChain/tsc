#!/bin/bash
# Run this script to quickly install, setup, and run the current version of the network without docker.
#
# Examples:
# CHAIN_ID="tsc_8878788-1" HOME_DIR="~/.tsc" BLOCK_TIME="1000ms" CLEAN=true sh scripts/test_node.sh
# CHAIN_ID="localchain_9000-2" HOME_DIR="~/.tsc" CLEAN=true RPC=36657 REST=2317 PROFF=6061 P2P=36656 GRPC=8090 GRPC_WEB=8091 ROSETTA=8081 JSONRPC=8555 JSONRPC_WS=8556 BLOCK_TIME="500ms" sh scripts/test_node.sh
#
# On a host with only the tscd binary and no source checkout, the build step is
# skipped automatically; the rest (keys, genesis, config, start) works as-is.
# Only jq and tscd are required. Piping over the network works too — note that
# env vars go BEFORE bash, they are not arguments to the script:
# CLEAN=true bash <(curl -sSL https://host/install-scripts/test_node.sh)
#
# CLEAN=true wipes HOME_DIR and rebuilds genesis. Without it, an existing chain
# in HOME_DIR is started as-is, and an empty HOME_DIR is set up from scratch.

set -eu

export KEY="acc0"
export KEY2="acc1"
# KEY3 is the validator operator (gentx below), the license/network namespace
# owner in genesis, and x/distro's DefaultMintingAddress — hence the label.
export KEY3="minting"

# Must match app.ChainID in app/app.go. The EIP-155 suffix has to equal
# app.EVMChainID: the EVM keeper and the tx encoder are compiled with that
# constant (app/app.go), so a chain id whose suffix says anything else is
# lying to wallets and tooling.
export CHAIN_ID=${CHAIN_ID:-"tsc_8878788-1"}
# EVM (EIP-155) chain id served by the JSON-RPC. app.toml is written with
# app.EVMChainID at init time, but an app.toml from an older build keeps its
# old value, so pass it explicitly on start — if the JSON-RPC reports a
# different id than the state machine signs with, every EVM tx is rejected.
export EVM_CHAIN_ID=${EVM_CHAIN_ID:-"8878788"}
export MONIKER="localvalidator"
export KEYALGO="eth_secp256k1"
export KEYRING=${KEYRING:-"test"}
export HOME_DIR=$(eval echo "${HOME_DIR:-"~/.tsc"}")
export BINARY=${BINARY:-tscd}
export DENOM=${DENOM:-aTSC}

export CLEAN=${CLEAN:-"false"}
export RPC=${RPC:-"26657"}
export REST=${REST:-"1317"}
export PROFF=${PROFF:-"6060"}
export P2P=${P2P:-"26656"}
export GRPC=${GRPC:-"9090"}
export GRPC_WEB=${GRPC_WEB:-"9091"}
export ROSETTA=${ROSETTA:-"8080"}
export JSONRPC=${JSONRPC:-"8545"}
export JSONRPC_WS=${JSONRPC_WS:-"8546"}
export BLOCK_TIME=${BLOCK_TIME:-"5s"}

# Rebuilding needs the repo. On a host that only carries the binary — a VM, a
# release artifact — there is no source tree and no Go toolchain, so use the
# $BINARY already on PATH and set genesis up from scratch with that. Everything
# below this point is binary-only. SKIP_BUILD=true forces that in a checkout too.
# $0 is not a real path when the script is piped or run through process
# substitution (bash <(curl ...)), so only look for a source tree when it is.
if [ -f "$0" ]; then
  SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
  REPO_ROOT=${REPO_ROOT:-$(dirname "$SCRIPT_DIR")}
else
  REPO_ROOT=${REPO_ROOT:-""}
fi
export SKIP_BUILD=${SKIP_BUILD:-"false"}

build_binary() {
  if [ "$SKIP_BUILD" != "false" ]; then
    echo "SKIP_BUILD=$SKIP_BUILD — using the $BINARY already on PATH"
  elif [ -n "$REPO_ROOT" ] && [ -f "$REPO_ROOT/Makefile" ] && [ -f "$REPO_ROOT/go.mod" ] && command -v make > /dev/null 2>&1; then
    make -C "$REPO_ROOT" install
  else
    echo "No source tree — using the $BINARY already on PATH"
  fi
}

# if the binary does not exist, try to build it from source
command -v $BINARY > /dev/null 2>&1 || build_binary

command -v $BINARY > /dev/null 2>&1 || { echo >&2 "$BINARY command not found. Install it into your PATH, or run this from a source checkout so 'make install' can build it."; exit 1; }
command -v jq > /dev/null 2>&1 || { echo >&2 "jq not installed. More info: https://stedolan.github.io/jq/download/"; exit 1; }

set_config() {
  $BINARY config set client chain-id $CHAIN_ID
  $BINARY config set client keyring-backend $KEYRING
}
set_config


from_scratch () {
  # Fresh install on current branch — no-op when there is no source tree
  build_binary

  # remove existing daemon files.
  if [ ${#HOME_DIR} -le 2 ]; then
      echo "HOME_DIR must be more than 2 characters long"
      return
  fi
  rm -rf $HOME_DIR && echo "Removed $HOME_DIR"

  # reset values if not set already after whipe
  set_config

  add_key() {
    key=$1
    mnemonic=$2
    echo $mnemonic | $BINARY keys add $key --keyring-backend $KEYRING --algo $KEYALGO --home $HOME_DIR --recover
  }

  # tsc15mfhza23rt35panamkyzr989rme34yke3k0tjf
  add_key $KEY "virus dinner recipe bid ripple amateur zebra frown flip walk acquire leopard poverty picture diamond pitch fresh talent color taste series faculty employ crew"
  # tsc1n2tvn6cqs0xesfe6s706y8r2sarezypwajsctp
  add_key $KEY2 "effort shift garlic pledge tiny where theme advice palm lift elephant giant erase critic off naive neutral person bone silly fall coconut ask boost"
  # tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4
  add_key $KEY3 "color drastic bachelor local mansion twenty grace camera circle hover ensure civil finger walnut yellow myth sort pottery sustain midnight punch card village clever"

  $BINARY init $MONIKER --chain-id $CHAIN_ID --default-denom $DENOM --home $HOME_DIR

  update_test_genesis () {
    cat $HOME_DIR/config/genesis.json | jq "$1" > $HOME_DIR/config/tmp_genesis.json && mv $HOME_DIR/config/tmp_genesis.json $HOME_DIR/config/genesis.json
  }

  # === CORE MODULES ===

  # Block
  update_test_genesis '.consensus_params["block"]["max_gas"]="100000000"'

  # Gov
  update_test_genesis `printf '.app_state["gov"]["params"]["min_deposit"]=[{"denom":"%s","amount":"1000000"}]' $DENOM`
  update_test_genesis '.app_state["gov"]["params"]["voting_period"]="30s"'
  update_test_genesis '.app_state["gov"]["params"]["expedited_voting_period"]="15s"'

  # Bank - register denom metadata for EVM (required by cosmos-evm v0.5.1)
  DENOM_METADATA="{\"description\":\"The native staking token of Trusted Smart Chain\",\"denom_units\":[{\"denom\":\"$DENOM\",\"exponent\":0,\"aliases\":[\"atsc\"]},{\"denom\":\"TSC\",\"exponent\":18}],\"base\":\"$DENOM\",\"display\":\"TSC\",\"name\":\"Trusted Smart Chain\",\"symbol\":\"TSC\"}"
  cat $HOME_DIR/config/genesis.json | jq ".app_state.bank.denom_metadata = [$DENOM_METADATA]" > $HOME_DIR/config/tmp_genesis.json && mv $HOME_DIR/config/tmp_genesis.json $HOME_DIR/config/genesis.json

  update_test_genesis `printf '.app_state["evm"]["params"]["evm_denom"]="%s"' $DENOM`
  update_test_genesis '.app_state["evm"]["params"]["active_static_precompiles"]=["0x0000000000000000000000000000000000000100","0x0000000000000000000000000000000000000400","0x0000000000000000000000000000000000000800","0x0000000000000000000000000000000000000801","0x0000000000000000000000000000000000000802","0x0000000000000000000000000000000000000803","0x0000000000000000000000000000000000000804","0x0000000000000000000000000000000000000805","0x0000000000000000000000000000000000000900","0x776562737461636B000000000000000000000001"]'
  # update_test_genesis '.app_state["erc20"]["params"]["native_precompiles"]=["0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE"]' # https://eips.ethereum.org/EIPS/eip-7528
  # update_test_genesis `printf '.app_state["erc20"]["token_pairs"]=[{contract_owner:1,erc20_address:"0xEeeeeEeeeEeEeeEeEeEeeEEEeeeeEeeeeeeeEEeE",denom:"%s",enabled:true}]' $DENOM`
  update_test_genesis '.app_state["feemarket"]["params"]["no_base_fee"]=true'
  update_test_genesis '.app_state["feemarket"]["params"]["base_fee"]="0.000000000000000000"'

  # staking
  update_test_genesis `printf '.app_state["staking"]["params"]["bond_denom"]="%s"' $DENOM`
  update_test_genesis '.app_state["staking"]["params"]["min_commission_rate"]="0.050000000000000000"'

  # mint
  update_test_genesis `printf '.app_state["mint"]["params"]["mint_denom"]="%s"' $DENOM`

  # crisis
  update_test_genesis `printf '.app_state["crisis"]["constant_fee"]={"denom":"%s","amount":"1000"}' $DENOM`

  ## abci
  update_test_genesis '.consensus["params"]["abci"]["vote_extensions_enable_height"]="1"'

  # === CUSTOM MODULES ===
  # tokenfactory
  update_test_genesis '.app_state["tokenfactory"]["params"]["denom_creation_fee"]=[]'
  update_test_genesis '.app_state["tokenfactory"]["params"]["denom_creation_gas_consume"]=100000'

  # license / permission / network / attestation.
  #
  # This is a dev chain, so it seeds a ready-to-use catalog. The v3 upgrade
  # handler deliberately does NOT: on a real network it seeds namespace
  # ownership and nothing else — no grants, no license types, no node types.
  # Everything below is done there by tx once the upgrade lands, in this order:
  # the owner grants itself license type.create and network nodetype.create,
  # creates the license types, grants itself the per-type issue/revoke, then
  # registers the node types. What follows is that runbook pre-applied, not a
  # mirror of the handler.
  #
  # Namespace owner for both the license and network namespaces is KEY3.
  update_test_genesis '.app_state["permission"]["namespaces"]=[{"module":"license","owner":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"},{"module":"network","owner":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"}]'
  # The node license types. These ids are license SKUs and are deliberately NOT
  # the node type strings below; the node_types registry binds one to the other.
  # max_supply caps licenses outstanding (active_count), so revoking one frees a
  # slot. creator is load-bearing rather than informational: x/network only lets
  # a node type bind to a license type its signer created, so a wrong creator
  # here makes the binding below unauthorized.
  update_test_genesis '.app_state["license"]["license_types"]=[{"id":"node.trust","transferrable":false,"max_supply":"240000","issued_count":"0","active_count":"0","revoked_count":"0","creator":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"},{"id":"node.nano","transferrable":false,"max_supply":"200000","issued_count":"0","active_count":"0","revoked_count":"0","creator":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"}]'
  # The node type registry, replacing the old allowed_node_types param. Each
  # entry binds one node type to one license type, one-to-one in both
  # directions, and that binding is what x/network counts and what x/attestation
  # checks a node against. An empty registry fail-closes activation.
  update_test_genesis '.app_state["network"]["node_types"]=[{"id":"trust","creator":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","license_type_id":"node.trust"},{"id":"nano","creator":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","license_type_id":"node.nano"}]'
  # KEY3 may create license types and node types, and issue and revoke node
  # licenses. Required, not merely convenient: both modules check the grant
  # table with no owner bypass. type.create and nodetype.create are module-wide,
  # so their scope is empty — the only form x/permission stores an unscoped
  # grant under.
  update_test_genesis '.app_state["permission"]["grants"]=[{"module":"license","grantee":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","permission":"type.create","scope":""},{"module":"license","grantee":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","permission":"issue","scope":"node.trust"},{"module":"license","grantee":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","permission":"revoke","scope":"node.trust"},{"module":"license","grantee":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","permission":"issue","scope":"node.nano"},{"module":"license","grantee":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","permission":"revoke","scope":"node.nano"},{"module":"network","grantee":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","permission":"nodetype.create","scope":""}]'
  # Network params carry only knobs now — the counted license SKUs and the node
  # types an activation may declare both moved into the node_types registry
  # above. The deauthorize fee (0.01 TSC) is the one value here the v3 handler
  # also seeds.
  update_test_genesis `printf '.app_state["network"]["params"]["deauthorize_fee"]=[{"denom":"%s","amount":"10000000000000000"}]' $DENOM`


  BASE_GENESIS_ALLOCATIONS="10000000000000000000000000$DENOM"

  # Allocate genesis accounts
  $BINARY genesis add-genesis-account $KEY $BASE_GENESIS_ALLOCATIONS --keyring-backend $KEYRING --home $HOME_DIR --append
  $BINARY genesis add-genesis-account $KEY2 $BASE_GENESIS_ALLOCATIONS --keyring-backend $KEYRING --home $HOME_DIR --append
  $BINARY genesis add-genesis-account $KEY3 $BASE_GENESIS_ALLOCATIONS --keyring-backend $KEYRING --home $HOME_DIR --append

  # Sign genesis transaction
  # min-self-delegation must be >= 500 TSC (500e18 aTSC) to satisfy the
  # min_self_delegation floor enforced by app/hooks.
  $BINARY genesis gentx $KEY3 500000000000000000000$DENOM --min-self-delegation 500000000000000000000 --gas-prices 0${DENOM} --keyring-backend $KEYRING --chain-id $CHAIN_ID --home $HOME_DIR

  $BINARY genesis collect-gentxs --home $HOME_DIR

  $BINARY genesis validate-genesis --home $HOME_DIR
  err=$?
  if [ $err -ne 0 ]; then
    echo "Failed to validate genesis"
    return
  fi
}

# check if CLEAN is not set to false
if [ "$CLEAN" != "false" ]; then
  echo "Starting from a clean state"
  from_scratch
elif [ ! -f "$HOME_DIR/config/genesis.json" ]; then
  # Nothing to start. A home that was never initialized is safe to build out;
  # one that has a config but no genesis belongs to some other setup (its
  # priv_validator_key.json and keyring are not ours to delete), so say so
  # instead of rm -rf'ing it.
  if [ -d "$HOME_DIR/config" ]; then
    echo >&2 "$HOME_DIR/config exists but has no genesis.json, so there is nothing to start."
    echo >&2 "It was initialized by something else (this script would use moniker '$MONIKER')."
    echo >&2 "Re-run with CLEAN=true to erase $HOME_DIR and build a fresh single-node chain,"
    echo >&2 "or point HOME_DIR somewhere else: CLEAN=true HOME_DIR=~/.tsc-local <this script>"
    exit 1
  fi
  echo "No chain at $HOME_DIR — setting up from scratch"
  from_scratch
fi

echo "Starting node..."

# Opens the RPC endpoint to outside connections
sed -i -e 's/laddr = "tcp:\/\/127.0.0.1:26657"/laddr = "tcp:\/\/0.0.0.0:'$RPC'"/g' $HOME_DIR/config/config.toml
sed -i -e 's/cors_allowed_origins = \[\]/cors_allowed_origins = \["\*"\]/g' $HOME_DIR/config/config.toml

# REST endpoint
sed -i -e 's/address = "tcp:\/\/localhost:1317"/address = "tcp:\/\/0.0.0.0:'$REST'"/g' $HOME_DIR/config/app.toml
sed -i -e 's/enable = false/enable = true/g' $HOME_DIR/config/app.toml
sed -i -e 's/enabled-unsafe-cors = false/enabled-unsafe-cors = true/g' $HOME_DIR/config/app.toml
sed -i -e 's/swagger = false/swagger = true/g' $HOME_DIR/config/app.toml

# peer exchange
sed -i -e 's/pprof_laddr = "localhost:6060"/pprof_laddr = "localhost:'$PROFF'"/g' $HOME_DIR/config/config.toml
sed -i -e 's/laddr = "tcp:\/\/0.0.0.0:26656"/laddr = "tcp:\/\/0.0.0.0:'$P2P'"/g' $HOME_DIR/config/config.toml

# GRPC
sed -i -e 's/address = "localhost:9090"/address = "0.0.0.0:'$GRPC'"/g' $HOME_DIR/config/app.toml
# NOTE: no-op on cosmos-sdk v0.53 — gRPC-web has no address of its own, it is
# served by the API server above. $GRPC_WEB is kept for callers that set it.
sed -i -e 's/address = "localhost:9091"/address = "0.0.0.0:'$GRPC_WEB'"/g' $HOME_DIR/config/app.toml

# EVM JSON-RPC. cosmos/evm binds 127.0.0.1:8545 / 127.0.0.1:8546 by default
# (127.0.0.1, not localhost — the obvious sed patterns silently match nothing),
# and evm-chain-id is whatever app.EVMChainID was when this app.toml was first
# written, so an app.toml carried over from an older build keeps the old id.
# The blanket "enable = false" -> true above already flips [json-rpc] enable;
# start passes --json-rpc.enable so it does not depend on that.
sed -i -e 's/^address = "127.0.0.1:8545"/address = "0.0.0.0:'$JSONRPC'"/g' $HOME_DIR/config/app.toml
sed -i -e 's/^ws-address = .*/ws-address = "0.0.0.0:'$JSONRPC_WS'"/g' $HOME_DIR/config/app.toml
sed -i -e 's/^evm-chain-id = .*/evm-chain-id = '$EVM_CHAIN_ID'/g' $HOME_DIR/config/app.toml
# ws-origins is deliberately NOT set here: the registered --json-rpc.ws-origins
# flag beats app.toml even when unset, so the ws handshake would keep 403ing
# browser clients no matter what the file says. It is passed on start instead.

# Rosetta Api
sed -i -e 's/address = ":8080"/address = "0.0.0.0:'$ROSETTA'"/g' $HOME_DIR/config/app.toml

# Faster blocks
sed -i -e 's/timeout_commit = "5s"/timeout_commit = "'$BLOCK_TIME'"/g' $HOME_DIR/config/config.toml

$BINARY start --pruning=nothing  --minimum-gas-prices=0$DENOM --rpc.laddr="tcp://0.0.0.0:$RPC" --home $HOME_DIR --api.enabled-unsafe-cors --json-rpc.enable --json-rpc.api=eth,txpool,personal,net,debug,web3 --json-rpc.address="0.0.0.0:$JSONRPC" --json-rpc.ws-address="0.0.0.0:$JSONRPC_WS" --json-rpc.ws-origins="*" --evm.evm-chain-id="$EVM_CHAIN_ID" --chain-id="$CHAIN_ID"
