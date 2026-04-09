#!/usr/bin/env bash

set -eo pipefail

mkdir -p ./tmp-swagger-gen

cd proto

# generate swagger files for custom modules (distro, lockup)
proto_dirs=$(find . -name '*.proto' -not -path './licenses/*' -print0 | xargs -0 -n1 dirname | sort | uniq)
for dir in $proto_dirs; do
  for proto_file in $(find "${dir}" -maxdepth 1 \( -name 'query.proto' -o -name 'service.proto' -o -name 'tx.proto' \)); do
    buf generate --template buf.gen.swagger.yaml "$proto_file"
  done
done

cd ..

# generate swagger for external licenses module
# copy protos into the local proto dir so buf can access them within its context
LICENSES_PROTO_DIR=$(go list -m -f '{{.Dir}}' github.com/webstack-sdk/webstack)/proto
if [[ -d "$LICENSES_PROTO_DIR/licenses" ]]; then
  cp -r "$LICENSES_PROTO_DIR/licenses" proto/licenses
  chmod -R u+w proto/licenses
  cd proto
  buf generate --template buf.gen.swagger.yaml --path licenses/v1/query.proto
  buf generate --template buf.gen.swagger.yaml --path licenses/v1/tx.proto
  cd ..
  rm -rf proto/licenses
fi

# combine swagger files
# uses nodejs package `swagger-combine`.
# all the individual swagger files need to be configured in `config.json` for merging
swagger-combine ./client/docs/config.json -o ./client/docs/swagger-ui/swagger.yaml -f yaml --continueOnConflictingPaths true --includeDefinitions true

# clean swagger files
rm -rf ./tmp-swagger-gen