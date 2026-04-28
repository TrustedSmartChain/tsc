FROM golang:1.24-alpine3.21 AS build-env

SHELL ["/bin/sh", "-ecuxo", "pipefail"]

RUN set -eux; apk add --no-cache \
    ca-certificates \
    build-base \
    git \
    linux-headers \
    bash \
    binutils-gold

WORKDIR /code

ADD go.mod go.sum ./
RUN set -eux; \
    go mod download; \
    ARCH=$(uname -m); \
    WASM_VERSION=$(go list -m all | grep github.com/CosmWasm/wasmvm || true); \
    if [ ! -z "${WASM_VERSION}" ]; then \
      WASMVM_REPO=$(echo $WASM_VERSION | awk '{print $1}');\
      WASMVM_VERS=$(echo $WASM_VERSION | awk '{print $2}');\
      WASMVM_RELEASE_REPO=$(echo $WASMVM_REPO | sed 's#/v[0-9]\+$##');\
      WASMVM_DIR=$(go list -m -f '{{.Dir}}' ${WASMVM_REPO});\
      chmod -R u+w "${WASMVM_DIR}/internal/api";\
      wget -O "${WASMVM_DIR}/internal/api/libwasmvm_muslc.${ARCH}.a" https://${WASMVM_RELEASE_REPO}/releases/download/${WASMVM_VERS}/libwasmvm_muslc.${ARCH}.a;\
    fi;

# Copy over code
COPY . /code

ARG VERSION=""

# force it to use static lib (from above) not standard libgo_cosmwasm.so file
# then log output of file /code/bin/tscd
# then ensure static linking
RUN LEDGER_ENABLED=false BUILD_TAGS=muslc LINK_STATICALLY=true make build VERSION="${VERSION}" \
  && file /code/build/tscd \
  && echo "Ensuring binary is statically linked ..." \
  && (file /code/build/tscd | grep "statically linked")

# --------------------------------------------------------
FROM alpine:3.21

COPY --from=build-env /code/build/tscd /usr/bin/tscd

RUN apk add --no-cache ca-certificates curl make bash jq sed

WORKDIR /opt

# rest server, tendermint p2p, tendermint rpc
EXPOSE 1317 26656 26657 8545 8546

CMD ["/usr/bin/tscd", "version"]
