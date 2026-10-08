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
RUN go mod download

# Copy over code
COPY . /code

ARG VERSION=""

# Build, then make sure the binary is statically linked.
RUN LEDGER_ENABLED=false LINK_STATICALLY=true make build VERSION="${VERSION}" \
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
