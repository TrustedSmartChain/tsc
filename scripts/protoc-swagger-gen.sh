#!/usr/bin/env bash

set -eo pipefail

mkdir -p ./tmp-swagger-gen

# external modules vendored in from the nodelabs SDK
SDK_MODULES="license permission network"

cd proto

# generate swagger files for custom modules (distro, lockup)
prune_paths=""
for mod in $SDK_MODULES; do
  prune_paths="$prune_paths -not -path ./$mod/*"
done
proto_dirs=$(find . -name '*.proto' $prune_paths -print0 | xargs -0 -n1 dirname | sort | uniq)
for dir in $proto_dirs; do
  for proto_file in $(find "${dir}" -maxdepth 1 \( -name 'query.proto' -o -name 'service.proto' -o -name 'tx.proto' \)); do
    buf generate --template buf.gen.swagger.yaml "$proto_file"
  done
done

cd ..

# generate swagger for the external nodelabs SDK modules
# copy protos into the local proto dir so buf can access them within its context
SDK_PROTO_DIR=$(go list -m -f '{{.Dir}}' github.com/nodelabs-sdk/nodelabs)/proto
for mod in $SDK_MODULES; do
  if [[ -d "$SDK_PROTO_DIR/$mod" ]]; then
    rm -rf "proto/$mod"
    cp -r "$SDK_PROTO_DIR/$mod" "proto/$mod"
    chmod -R u+w "proto/$mod"
    cd proto
    buf generate --template buf.gen.swagger.yaml --path "$mod/v1/query.proto"
    buf generate --template buf.gen.swagger.yaml --path "$mod/v1/tx.proto"
    cd ..
    rm -rf "proto/$mod"
  fi
done

# combine swagger files
# uses nodejs package `swagger-combine`.
# all the individual swagger files need to be configured in `config.json` for merging
swagger-combine ./client/docs/config.json -o ./client/docs/swagger-ui/swagger.yaml -f yaml --continueOnConflictingPaths true --includeDefinitions true

# clean swagger files
rm -rf ./tmp-swagger-gen