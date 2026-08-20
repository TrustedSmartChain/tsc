#!/bin/bash
# Run scripts/test_upgrade.sh inside a Linux container, so the old binary is
# the actual tscd-linux asset from the GitHub release rather than a
# from-source rebuild — the exact artifact validators would be running.
#
# Examples:
#   sh scripts/test_upgrade_docker.sh
#   UPGRADE_DELTA=80 BLOCK_TIME=2000ms sh scripts/test_upgrade_docker.sh
#
# Two images are involved:
#   - tscd-build (the release Dockerfile, via make build-linux) compiles the
#     upgraded binary from this working tree as a static binary, built for the
#     docker server's NATIVE arch so the Go compile never runs emulated.
#   - upgrade-test-env.Dockerfile is the runtime the test executes in: always
#     linux/amd64, because the release asset is a dynamically linked amd64
#     glibc binary (see that file for why ubuntu:24.04 + libwasmvm).
#
# The kernel is shared across images, so both binaries run in the one amd64
# container: the static native-arch upgraded binary executes natively, and on
# a non-amd64 host the release binary executes through binfmt emulation —
# that costs a few extra seconds of node runtime, versus tens of minutes if
# the compile itself were emulated.
#
# The release binary is downloaded inside the container by test_upgrade.sh
# (OLD_FROM_RELEASE=true) and cached on the host under
# build/upgrade-test-docker along with both node logs. KEEP_RUNNING is not
# passed through on purpose: the container exits with the script, so there is
# nothing to keep running.

set -eu

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(dirname "$SCRIPT_DIR")

command -v docker > /dev/null 2>&1 || { echo >&2 "docker not found — required for the containerized upgrade test"; exit 1; }

SERVER_ARCH=$(docker version --format '{{.Server.Arch}}')
# The arch the upgraded binary is compiled for. Native by default; override
# with TARGET_PLATFORM=linux/amd64 to test the all-amd64 combination.
TARGET_PLATFORM=${TARGET_PLATFORM:-"linux/$SERVER_ARCH"}
# The wasmvm shared library the OLD release binary links against — must match
# the old version's go.mod, not this tree's.
WASMVM_VERSION=${WASMVM_VERSION:-"v2.3.1"}
# Defaults mirror test_upgrade.sh — they are needed here too for the
# host-side pre-fetch below.
OLD_VERSION=${OLD_VERSION:-"v2.0.4"}
GITHUB_REPO=${GITHUB_REPO:-"TrustedSmartChain/tsc"}
TEST_ENV_IMAGE=${TEST_ENV_IMAGE:-"tsc-upgrade-test-env"}
HOST_WORK_DIR=${HOST_WORK_DIR:-"$REPO_ROOT/build/upgrade-test-docker"}

# The amd64 release binary has to be executable somewhere in this setup:
# natively on an amd64 server, via binfmt otherwise. Probe before spending
# minutes on the image build, and fail with the fix rather than with
# "exec format error" mid-test.
if [ "$SERVER_ARCH" != "amd64" ]; then
  if ! docker run --rm --platform linux/amd64 ubuntu:24.04 true > /dev/null 2>&1; then
    echo >&2 "This docker server ($SERVER_ARCH) cannot execute amd64 binaries, and the"
    echo >&2 "release asset is amd64-only. Enable emulation with either:"
    echo >&2 "  - Docker Desktop: Settings -> General -> 'Use Rosetta for x86_64/amd64"
    echo >&2 "    emulation on Apple Silicon' (fastest), or"
    echo >&2 "  - QEMU binfmt: docker run --privileged --rm tonistiigi/binfmt --install amd64"
    echo >&2 "then re-run this script."
    exit 1
  fi
fi

echo "=== Building the $TARGET_PLATFORM upgraded binary (make build-linux) ==="
make -C "$REPO_ROOT" build-linux TARGET_PLATFORM="$TARGET_PLATFORM"

echo "=== Building the amd64 test environment image ==="
docker build --platform linux/amd64 \
  --build-arg WASMVM_VERSION="$WASMVM_VERSION" \
  -f "$SCRIPT_DIR/upgrade-test-env.Dockerfile" \
  -t "$TEST_ENV_IMAGE" "$SCRIPT_DIR"

mkdir -p "$HOST_WORK_DIR"

# Pre-fetch the release asset with the host's curl and CA store, landing it
# where test_upgrade.sh's cache check will find it. The script can download it
# itself (OLD_FROM_RELEASE=true below), but doing it here keeps the
# container's network needs at zero.
if [ ! -x "$HOST_WORK_DIR/tscd-$OLD_VERSION" ]; then
  echo "=== Pre-fetching the $OLD_VERSION release asset ==="
  curl -fL "https://github.com/$GITHUB_REPO/releases/download/$OLD_VERSION/tscd-linux" \
    -o "$HOST_WORK_DIR/tscd-$OLD_VERSION"
  chmod +x "$HOST_WORK_DIR/tscd-$OLD_VERSION"
fi

# Pass the test's tuning knobs through only when set on the host, so the
# script's own defaults stay authoritative.
ENV_ARGS=()
for v in OLD_VERSION UPGRADE_NAME GITHUB_REPO CHAIN_ID EVM_CHAIN_ID DENOM \
         BLOCK_TIME VOTING_PERIOD EXPEDITED_VOTING_PERIOD UPGRADE_DELTA \
         DEPOSIT GAS_PRICES; do
  if [ -n "${!v:-}" ]; then
    ENV_ARGS+=(-e "$v=${!v}")
  fi
done

echo "=== Running the upgrade test in the container ==="
docker run --rm --platform linux/amd64 \
  -v "$SCRIPT_DIR/test_upgrade.sh:/opt/scripts/test_upgrade.sh:ro" \
  -v "$REPO_ROOT/build/tscd-linux:/usr/bin/tscd:ro" \
  -v "$HOST_WORK_DIR:/work" \
  -e WORK_DIR=/work \
  -e NEW_BINARY=/usr/bin/tscd \
  -e OLD_FROM_RELEASE=true \
  -e HOME_DIR=/root/.tsc-upgrade-test \
  ${ENV_ARGS[@]+"${ENV_ARGS[@]}"} \
  "$TEST_ENV_IMAGE" bash /opt/scripts/test_upgrade.sh
