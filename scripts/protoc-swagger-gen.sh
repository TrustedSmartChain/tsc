#!/usr/bin/env bash

set -eo pipefail

mkdir -p ./tmp-swagger-gen

# external modules vendored in from the nodelabs SDK that serve a query/tx API
SDK_MODULES="license network"
# SDK protos that are imports only. x/access defines the grant types the license
# and network services embed, but exposes no service of its own, so it has to be
# copied in for those two to compile and must not be generated for.
SDK_DEP_MODULES="access"

cd proto

# generate swagger files for custom modules (distro, lockup)
prune_paths=""
for mod in $SDK_MODULES $SDK_DEP_MODULES; do
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

# Copied protos are removed on the way out, including on failure — a leftover
# proto/license would be picked up as a local module by the loop above on the
# next run and generated twice.
copied=""
cleanup() {
  for mod in $copied; do
    rm -rf "proto/$mod"
  done
}
trap cleanup EXIT

# Dependencies first: the service protos below import them.
for mod in $SDK_DEP_MODULES; do
  if [[ -d "$SDK_PROTO_DIR/$mod" ]]; then
    rm -rf "proto/$mod"
    cp -r "$SDK_PROTO_DIR/$mod" "proto/$mod"
    chmod -R u+w "proto/$mod"
    copied="$copied $mod"
  fi
done

for mod in $SDK_MODULES; do
  if [[ -d "$SDK_PROTO_DIR/$mod" ]]; then
    rm -rf "proto/$mod"
    cp -r "$SDK_PROTO_DIR/$mod" "proto/$mod"
    chmod -R u+w "proto/$mod"
    copied="$copied $mod"
    cd proto
    buf generate --template buf.gen.swagger.yaml --path "$mod/v1/query.proto"
    buf generate --template buf.gen.swagger.yaml --path "$mod/v1/tx.proto"
    cd ..
  fi
done

cleanup
copied=""

# combine swagger files
# uses nodejs package `swagger-combine`.
# all the individual swagger files need to be configured in `config.json` for merging
swagger-combine ./client/docs/config.json -o ./client/docs/swagger-ui/swagger.yaml -f yaml --continueOnConflictingPaths true --includeDefinitions true

# clean swagger files
rm -rf ./tmp-swagger-gen