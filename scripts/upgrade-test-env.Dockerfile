# Runtime environment for scripts/test_upgrade_docker.sh — NOT a build image.
#
# The tscd-linux release asset is a dynamically linked amd64 binary (the
# v2-era CI ran a plain `go build` on ubuntu-latest rather than the static
# Dockerfile build), so it has two runtime needs the release does not ship:
#   - glibc 2.38+ — this image is ubuntu:24.04-based, matching the runner
#     that built it; alpine (musl) and debian bookworm (glibc 2.36) both
#     fail to load it
#   - libwasmvm.x86_64.so — fetched here from the wasmvm release matching the
#     old version's go.mod (v2.0.3 and v2.0.4 both pin wasmvm v2.3.1)
#
# No apt-get anywhere, deliberately: on an arm64 host this image builds and
# runs under qemu emulation, where apt's gpgv reliably fails with "invalid
# signature" errors. Everything is either already in the base image
# (buildpack-deps:noble-curl = ubuntu:24.04 + curl + CA certs) or ADDed as a
# static binary — buildkit fetches ADD URLs natively on the daemon side, so
# emulation never touches them.
#
# The upgraded binary needs none of this — it is static — but it is mounted
# into this same container, so the whole upgrade runs in one place.
FROM buildpack-deps:noble-curl

ARG WASMVM_VERSION=v2.3.1
ARG JQ_VERSION=1.7.1

ADD --chmod=755 https://github.com/jqlang/jq/releases/download/jq-${JQ_VERSION}/jq-linux-amd64 /usr/bin/jq
ADD --chmod=644 https://github.com/CosmWasm/wasmvm/releases/download/${WASMVM_VERSION}/libwasmvm.x86_64.so /usr/lib/libwasmvm.x86_64.so

WORKDIR /opt
