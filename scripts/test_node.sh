#!/bin/bash
# Run this script to quickly install, setup, and run the current version of the network without docker.
#
# Examples:
# CHAIN_ID="tsc_8878788-1" HOME_DIR="~/.trustedsmartchain" BLOCK_TIME="1000ms" CLEAN=true sh scripts/test_node.sh
# CHAIN_ID="localchain_9000-2" HOME_DIR="~/.trustedsmartchain" CLEAN=true RPC=36657 REST=2317 PROFF=6061 P2P=36656 GRPC=8090 GRPC_WEB=8091 ROSETTA=8081 BLOCK_TIME="500ms" sh scripts/test_node.sh

set -eu

export KEY="acc0"
export KEY2="acc1"
export KEY3="minting"

export CHAIN_ID=${CHAIN_ID:-"tsc_8878788-1"}
export MONIKER="localvalidator"
export KEYALGO="eth_secp256k1"
export KEYRING=${KEYRING:-"test"}
export HOME_DIR=$(eval echo "${HOME_DIR:-"~/.trustedsmartchain"}")
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

# if which binary does not exist, install it
if [ -z `which $BINARY` ]; then
  make install

  if [ -z `which $BINARY` ]; then
    echo "Ensure $BINARY is installed and in your PATH"
    exit 1
  fi
fi

command -v $BINARY > /dev/null 2>&1 || { echo >&2 "$BINARY command not found. Ensure this is setup / properly installed in your GOPATH (make install)."; exit 1; }
command -v jq > /dev/null 2>&1 || { echo >&2 "jq not installed. More info: https://stedolan.github.io/jq/download/"; exit 1; }

set_config() {
  $BINARY config set client chain-id $CHAIN_ID
  $BINARY config set client keyring-backend $KEYRING
}
set_config


from_scratch () {
  # Fresh install on current branch
  make install

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

  # tsc140fehngcrxvhdt84x729p3f0qmkmea8nynd44p
  add_key $KEY "decorate bright ozone fork gallery riot bus exhaust worth way bone indoor calm squirrel merry zero scheme cotton until shop any excess stage laundry"
  # tsc1r6yue0vuyj9m7xw78npspt9drq2tmtvggwt5x2
  add_key $KEY2 "wealth flavor believe regret funny network recall kiss grape useless pepper cram hint member few certain unveil rather brick bargain curious require crowd raise"
  # tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4
  add_key $KEY3 "tilt steel wet bottom afraid return thrive wrestle camera bitter tape pretty"

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

  # license / permission / network / attestation — mirror what the v3
  # upgrade handler seeds on real networks (the module defaults fail closed:
  # with an empty license_types no node could ever activate).
  # Namespace owner for both the license and network namespaces is KEY3.
  update_test_genesis '.app_state["permission"]["namespaces"]=[{"module":"license","owner":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"},{"module":"network","owner":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4"}]'
  # The counted node license types, pre-created so grants can be seeded.
  # These ids are license SKUs and are deliberately NOT the node type strings
  # below; x/attestation maps one to the other.
  # max_supply caps lifetime issuance (issued_count), which revocation does not
  # decrement — matching v3NodeLicenseSupply in app/upgrades_v3.go.
  update_test_genesis '.app_state["license"]["license_types"]=[{"id":"node.trust","transferrable":false,"max_supply":"240000","issued_count":"0","active_count":"0","revoked_count":"0"},{"id":"node.nano","transferrable":false,"max_supply":"200000","issued_count":"0","active_count":"0","revoked_count":"0"}]'
  # KEY3 may issue and revoke node licenses. Required, not merely convenient:
  # x/license checks issue/revoke against the grant table with no owner bypass.
  update_test_genesis '.app_state["permission"]["grants"]=[{"module":"license","grantee":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","permission":"issue","scope":"node.trust"},{"module":"license","grantee":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","permission":"revoke","scope":"node.trust"},{"module":"license","grantee":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","permission":"issue","scope":"node.nano"},{"module":"license","grantee":"tsc1cd3de90g8ktz20qtyc945chwg8pg8xn9trwpz4","permission":"revoke","scope":"node.nano"}]'
  # Network params: counted license SKU ids, the node types an activation may
  # declare, and the deauthorize fee (0.01 TSC), matching app/upgrades_v3.go.
  update_test_genesis `printf '.app_state["network"]["params"]["license_types"]=["node.trust","node.nano"]'`
  update_test_genesis `printf '.app_state["network"]["params"]["allowed_node_types"]=["trust","nano"]'`
  update_test_genesis `printf '.app_state["network"]["params"]["deauthorize_fee"]=[{"denom":"%s","amount":"10000000000000000"}]' $DENOM`


  BASE_GENESIS_ALLOCATIONS="1000000000000000000000$DENOM"

  # Allocate genesis accounts
  $BINARY genesis add-genesis-account $KEY $BASE_GENESIS_ALLOCATIONS --keyring-backend $KEYRING --home $HOME_DIR --append
  $BINARY genesis add-genesis-account $KEY2 $BASE_GENESIS_ALLOCATIONS --keyring-backend $KEYRING --home $HOME_DIR --append
  $BINARY genesis add-genesis-account $KEY3 $BASE_GENESIS_ALLOCATIONS --keyring-backend $KEYRING --home $HOME_DIR --append

  # Sign genesis transaction
  # min-self-delegation must be >= 500 TSC (500e18 aTSC) to satisfy the
  # min_self_delegation floor enforced by app/hooks.
  $BINARY genesis gentx $KEY 500000000000000000000$DENOM --min-self-delegation 500000000000000000000 --gas-prices 0${DENOM} --keyring-backend $KEYRING --chain-id $CHAIN_ID --home $HOME_DIR

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
fi

echo "Starting node..."

# Opens the RPC endpoint to outside connections
sed -i -e 's/laddr = "tcp:\/\/127.0.0.1:26657"/c\laddr = "tcp:\/\/0.0.0.0:'$RPC'"/g' $HOME_DIR/config/config.toml
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
sed -i -e 's/address = "localhost:9091"/address = "0.0.0.0:'$GRPC_WEB'"/g' $HOME_DIR/config/app.toml

# Rosetta Api
sed -i -e 's/address = ":8080"/address = "0.0.0.0:'$ROSETTA'"/g' $HOME_DIR/config/app.toml

# Faster blocks
sed -i -e 's/timeout_commit = "5s"/timeout_commit = "'$BLOCK_TIME'"/g' $HOME_DIR/config/config.toml

$BINARY start --pruning=nothing  --minimum-gas-prices=0$DENOM --rpc.laddr="tcp://0.0.0.0:$RPC" --home $HOME_DIR --json-rpc.api=eth,txpool,personal,net,debug,web3 --json-rpc.address="127.0.0.1:$JSONRPC" --json-rpc.ws-address="127.0.0.1:$JSONRPC_WS" --chain-id="$CHAIN_ID"
