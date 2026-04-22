#!/bin/bash
# Run a local 2-validator testnet.
#
# Usage:
#   CHAIN_ID="tsc_8878788-1" BLOCK_TIME="1000ms" CLEAN=true sh scripts/test_2node.sh

set -eu

export CHAIN_ID=${CHAIN_ID:-"tsc_8878788-1"}
export KEYALGO="eth_secp256k1"
export KEYRING=${KEYRING:-"test"}
export BINARY=${BINARY:-tscd}
export DENOM=${DENOM:-aTSC}
export BLOCK_TIME=${BLOCK_TIME:-"5s"}
export CLEAN=${CLEAN:-"false"}

# Node 1 config
NODE1_HOME=$(eval echo "~/.tsc-node1")
NODE1_MONIKER="validator1"
NODE1_RPC=26657
NODE1_P2P=26656
NODE1_REST=1317
NODE1_GRPC=9090
NODE1_GRPC_WEB=9091
NODE1_PROFF=6060
NODE1_ROSETTA=8080
NODE1_JSON_RPC=8545
NODE1_JSON_RPC_WS=8546

# Node 2 config
NODE2_HOME=$(eval echo "~/.tsc-node2")
NODE2_MONIKER="validator2"
NODE2_RPC=36657
NODE2_P2P=36656
NODE2_REST=2317
NODE2_GRPC=9190
NODE2_GRPC_WEB=9191
NODE2_PROFF=6160
NODE2_ROSETTA=8180
NODE2_JSON_RPC=8645
NODE2_JSON_RPC_WS=8646

# Keys
KEY1="val1"
KEY2="val2"
KEY_EXTRA="acc0"

# Mnemonics (deterministic for reproducibility)
MNEMONIC1="decorate bright ozone fork gallery riot bus exhaust worth way bone indoor calm squirrel merry zero scheme cotton until shop any excess stage laundry"
MNEMONIC2="wealth flavor believe regret funny network recall kiss grape useless pepper cram hint member few certain unveil rather brick bargain curious require crowd raise"
MNEMONIC_EXTRA="tilt steel wet bottom afraid return thrive wrestle camera bitter tape pretty"

if [ -z "$(which $BINARY)" ]; then
  make install
  if [ -z "$(which $BINARY)" ]; then
    echo "Ensure $BINARY is installed and in your PATH"
    exit 1
  fi
fi

command -v jq > /dev/null 2>&1 || { echo >&2 "jq not installed."; exit 1; }

cleanup() {
  echo ""
  echo "Shutting down validators..."
  kill $PID1 $PID2 2>/dev/null || true
  wait $PID1 $PID2 2>/dev/null || true
  echo "Done."
}
trap cleanup EXIT INT TERM

from_scratch() {
  make install

  # Remove old state
  for dir in "$NODE1_HOME" "$NODE2_HOME"; do
    if [ ${#dir} -le 2 ]; then
      echo "HOME_DIR must be more than 2 characters long"
      return
    fi
    rm -rf "$dir" && echo "Removed $dir"
  done

  add_key() {
    local home=$1
    local key=$2
    local mnemonic=$3
    echo "$mnemonic" | $BINARY keys add "$key" --keyring-backend $KEYRING --algo $KEYALGO --home "$home" --recover
  }

  # --- Init both nodes ---
  $BINARY init $NODE1_MONIKER --chain-id $CHAIN_ID --default-denom $DENOM --home $NODE1_HOME
  $BINARY init $NODE2_MONIKER --chain-id $CHAIN_ID --default-denom $DENOM --home $NODE2_HOME

  # --- Add keys to both nodes ---
  add_key $NODE1_HOME $KEY1 "$MNEMONIC1"
  add_key $NODE1_HOME $KEY_EXTRA "$MNEMONIC_EXTRA"
  add_key $NODE2_HOME $KEY2 "$MNEMONIC2"

  # Add key2 to node1's keyring too (needed for genesis account creation)
  add_key $NODE1_HOME $KEY2 "$MNEMONIC2"

  # --- Configure shared genesis on node1 ---
  update_genesis() {
    cat $NODE1_HOME/config/genesis.json | jq "$1" > $NODE1_HOME/config/tmp_genesis.json && mv $NODE1_HOME/config/tmp_genesis.json $NODE1_HOME/config/genesis.json
  }

  # Block
  update_genesis '.consensus_params["block"]["max_gas"]="100000000"'

  # Gov
  update_genesis "$(printf '.app_state["gov"]["params"]["min_deposit"]=[{"denom":"%s","amount":"1000000"}]' $DENOM)"
  update_genesis '.app_state["gov"]["params"]["voting_period"]="30s"'
  update_genesis '.app_state["gov"]["params"]["expedited_voting_period"]="15s"'

  # Bank denom metadata
  DENOM_METADATA="{\"description\":\"The native staking token of Trusted Smart Chain\",\"denom_units\":[{\"denom\":\"$DENOM\",\"exponent\":0,\"aliases\":[\"atsc\"]},{\"denom\":\"TSC\",\"exponent\":18}],\"base\":\"$DENOM\",\"display\":\"TSC\",\"name\":\"Trusted Smart Chain\",\"symbol\":\"TSC\"}"
  cat $NODE1_HOME/config/genesis.json | jq ".app_state.bank.denom_metadata = [$DENOM_METADATA]" > $NODE1_HOME/config/tmp_genesis.json && mv $NODE1_HOME/config/tmp_genesis.json $NODE1_HOME/config/genesis.json

  # EVM
  update_genesis "$(printf '.app_state["evm"]["params"]["evm_denom"]="%s"' $DENOM)"
  update_genesis '.app_state["evm"]["params"]["active_static_precompiles"]=["0x0000000000000000000000000000000000000100","0x0000000000000000000000000000000000000400","0x0000000000000000000000000000000000000800","0x0000000000000000000000000000000000000801","0x0000000000000000000000000000000000000802","0x0000000000000000000000000000000000000803","0x0000000000000000000000000000000000000804","0x0000000000000000000000000000000000000805","0x0000000000000000000000000000000000000900"]'
  update_genesis '.app_state["feemarket"]["params"]["no_base_fee"]=true'
  update_genesis '.app_state["feemarket"]["params"]["base_fee"]="0.000000000000000000"'

  # Staking
  update_genesis "$(printf '.app_state["staking"]["params"]["bond_denom"]="%s"' $DENOM)"
  update_genesis '.app_state["staking"]["params"]["min_commission_rate"]="0.050000000000000000"'

  # Mint
  update_genesis "$(printf '.app_state["mint"]["params"]["mint_denom"]="%s"' $DENOM)"

  # Crisis
  update_genesis "$(printf '.app_state["crisis"]["constant_fee"]={"denom":"%s","amount":"1000"}' $DENOM)"

  # ABCI vote extensions
  update_genesis '.consensus["params"]["abci"]["vote_extensions_enable_height"]="1"'

  # Tokenfactory
  update_genesis '.app_state["tokenfactory"]["params"]["denom_creation_fee"]=[]'
  update_genesis '.app_state["tokenfactory"]["params"]["denom_creation_gas_consume"]=100000'

  # --- Genesis accounts ---
  BASE_ALLOC="1000000000000000000000$DENOM"
  $BINARY genesis add-genesis-account $KEY1 $BASE_ALLOC --keyring-backend $KEYRING --home $NODE1_HOME --append
  $BINARY genesis add-genesis-account $KEY2 $BASE_ALLOC --keyring-backend $KEYRING --home $NODE1_HOME --append
  $BINARY genesis add-genesis-account $KEY_EXTRA $BASE_ALLOC --keyring-backend $KEYRING --home $NODE1_HOME --append

  # --- Gentx from both validators ---
  STAKE="500000000000000000000$DENOM"

  # Node1 gentx
  $BINARY genesis gentx $KEY1 $STAKE --gas-prices 0${DENOM} --keyring-backend $KEYRING --chain-id $CHAIN_ID --home $NODE1_HOME

  # Node2 gentx: copy genesis to node2, generate gentx there, copy back
  cp $NODE1_HOME/config/genesis.json $NODE2_HOME/config/genesis.json
  $BINARY genesis gentx $KEY2 $STAKE --gas-prices 0${DENOM} --keyring-backend $KEYRING --chain-id $CHAIN_ID --home $NODE2_HOME
  cp $NODE2_HOME/config/gentx/*.json $NODE1_HOME/config/gentx/

  # Collect all gentxs on node1
  $BINARY genesis collect-gentxs --home $NODE1_HOME

  # Validate
  $BINARY genesis validate-genesis --home $NODE1_HOME
  err=$?
  if [ $err -ne 0 ]; then
    echo "Failed to validate genesis"
    return
  fi

  # Copy final genesis to node2
  cp $NODE1_HOME/config/genesis.json $NODE2_HOME/config/genesis.json

  echo ""
  echo "Genesis created with 2 validators."
}

configure_node() {
  local home=$1
  local rpc=$2
  local p2p=$3
  local rest=$4
  local grpc=$5
  local grpc_web=$6
  local proff=$7
  local rosetta=$8
  local json_rpc=$9
  local json_rpc_ws=${10}

  # RPC
  sed -i '' "s|laddr = \"tcp://127.0.0.1:26657\"|laddr = \"tcp://0.0.0.0:${rpc}\"|g" $home/config/config.toml
  sed -i '' "s|cors_allowed_origins = \[\]|cors_allowed_origins = [\"*\"]|g" $home/config/config.toml

  # REST
  sed -i '' "s|address = \"tcp://localhost:1317\"|address = \"tcp://0.0.0.0:${rest}\"|g" $home/config/app.toml
  sed -i '' "s|enable = false|enable = true|g" $home/config/app.toml
  sed -i '' "s|enabled-unsafe-cors = false|enabled-unsafe-cors = true|g" $home/config/app.toml

  # P2P & pprof
  sed -i '' "s|pprof_laddr = \"localhost:6060\"|pprof_laddr = \"localhost:${proff}\"|g" $home/config/config.toml
  sed -i '' "s|laddr = \"tcp://0.0.0.0:26656\"|laddr = \"tcp://0.0.0.0:${p2p}\"|g" $home/config/config.toml

  # GRPC
  sed -i '' "s|address = \"localhost:9090\"|address = \"0.0.0.0:${grpc}\"|g" $home/config/app.toml
  sed -i '' "s|address = \"localhost:9091\"|address = \"0.0.0.0:${grpc_web}\"|g" $home/config/app.toml

  # Rosetta
  sed -i '' "s|address = \":8080\"|address = \"0.0.0.0:${rosetta}\"|g" $home/config/app.toml

  # Block time
  sed -i '' "s|timeout_commit = \"5s\"|timeout_commit = \"${BLOCK_TIME}\"|g" $home/config/config.toml

  # JSON-RPC
  sed -i '' "s|address = \"127.0.0.1:8545\"|address = \"0.0.0.0:${json_rpc}\"|g" $home/config/app.toml
  sed -i '' "s|ws-address = \"127.0.0.1:8546\"|ws-address = \"0.0.0.0:${json_rpc_ws}\"|g" $home/config/app.toml
}

setup_peers() {
  NODE1_ID=$($BINARY comet show-node-id --home $NODE1_HOME)
  NODE2_ID=$($BINARY comet show-node-id --home $NODE2_HOME)

  sed -i '' "s|persistent_peers = \"\"|persistent_peers = \"${NODE2_ID}@127.0.0.1:${NODE2_P2P}\"|g" $NODE1_HOME/config/config.toml
  sed -i '' "s|persistent_peers = \"\"|persistent_peers = \"${NODE1_ID}@127.0.0.1:${NODE1_P2P}\"|g" $NODE2_HOME/config/config.toml
}

# --- Main ---

if [ "$CLEAN" != "false" ]; then
  echo "Starting from a clean state"
  from_scratch
fi

echo ""
echo "Configuring nodes..."

configure_node $NODE1_HOME $NODE1_RPC $NODE1_P2P $NODE1_REST $NODE1_GRPC $NODE1_GRPC_WEB $NODE1_PROFF $NODE1_ROSETTA $NODE1_JSON_RPC $NODE1_JSON_RPC_WS
configure_node $NODE2_HOME $NODE2_RPC $NODE2_P2P $NODE2_REST $NODE2_GRPC $NODE2_GRPC_WEB $NODE2_PROFF $NODE2_ROSETTA $NODE2_JSON_RPC $NODE2_JSON_RPC_WS

setup_peers

echo "Starting validator 1 (RPC :$NODE1_RPC, P2P :$NODE1_P2P, JSON-RPC :$NODE1_JSON_RPC)..."
$BINARY start --pruning=nothing --minimum-gas-prices=0$DENOM --rpc.laddr="tcp://0.0.0.0:$NODE1_RPC" --home $NODE1_HOME --json-rpc.api=eth,txpool,personal,net,debug,web3 --chain-id="$CHAIN_ID" > /tmp/tsc-node1.log 2>&1 &
PID1=$!

echo "Starting validator 2 (RPC :$NODE2_RPC, P2P :$NODE2_P2P, JSON-RPC :$NODE2_JSON_RPC)..."
$BINARY start --pruning=nothing --minimum-gas-prices=0$DENOM --rpc.laddr="tcp://0.0.0.0:$NODE2_RPC" --home $NODE2_HOME --json-rpc.api=eth,txpool,personal,net,debug,web3 --chain-id="$CHAIN_ID" > /tmp/tsc-node2.log 2>&1 &
PID2=$!

echo ""
echo "======================================"
echo "  2-Validator Testnet Running"
echo "======================================"
echo ""
echo "  Validator 1:"
echo "    Home:     $NODE1_HOME"
echo "    RPC:      http://localhost:$NODE1_RPC"
echo "    REST:     http://localhost:$NODE1_REST"
echo "    gRPC:     localhost:$NODE1_GRPC"
echo "    JSON-RPC: http://localhost:$NODE1_JSON_RPC"
echo "    Log:      /tmp/tsc-node1.log"
echo ""
echo "  Validator 2:"
echo "    Home:     $NODE2_HOME"
echo "    RPC:      http://localhost:$NODE2_RPC"
echo "    REST:     http://localhost:$NODE2_REST"
echo "    gRPC:     localhost:$NODE2_GRPC"
echo "    JSON-RPC: http://localhost:$NODE2_JSON_RPC"
echo "    Log:      /tmp/tsc-node2.log"
echo ""
echo "  Press Ctrl+C to stop both validators."
echo "  Use 'tail -f /tmp/tsc-node1.log' or '/tmp/tsc-node2.log' to watch logs."
echo "======================================"
echo ""

# Wait for both processes
wait $PID1 $PID2
