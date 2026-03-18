#!/usr/bin/env bash

set -eo pipefail

mkdir -p ./tmp-swagger-gen

# Generate swagger files from SDK proto definitions
SDK_VERSION=$(go list -m -f '{{.Version}}' github.com/cosmos/cosmos-sdk)
SDK_PROTO="$(go env GOMODCACHE)/github.com/cosmos/cosmos-sdk@${SDK_VERSION}/proto"

cd proto

# Generate swagger for SDK modules
proto_dirs=$(find "$SDK_PROTO/cosmos" -path -prune -o -name '*.proto' -print0 | xargs -0 -n1 dirname | sort | uniq)
for dir in $proto_dirs; do
  query_file=$(find "${dir}" -maxdepth 1 \( -name 'query.proto' -o -name 'service.proto' \))
  if [[ ! -z "$query_file" ]]; then
    buf generate --template buf.gen.swagger.yaml $query_file
  fi
done

# Generate swagger for custom modules
proto_dirs=$(find . -path -prune -o -name '*.proto' -print0 | xargs -0 -n1 dirname | sort | uniq)
for dir in $proto_dirs; do
  for proto_file in $(find "${dir}" -maxdepth 1 \( -name 'query.proto' -o -name 'tx.proto' \)); do
    buf generate --template buf.gen.swagger.yaml "$proto_file"
  done
done

cd ..

# Combine swagger files using config.json
# Uses nodejs package `swagger-combine`.
# All individual swagger files need to be configured in `config.json` for merging.
swagger-combine ./client/docs/config.json -o ./client/docs/swagger-ui/swagger.yaml -f yaml --continueOnConflictingPaths true --includeDefinitions true

# Clean swagger files
rm -rf ./tmp-swagger-gen
