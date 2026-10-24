#!/bin/sh
# Verify the sandbox conformance battery inside a Daytona sandbox over SSH,
# the research-pinned lowest-friction provider channel. The shim is uploaded
# once, then smokepod drives it through the SSH connection in process mode.
#
# All connection settings come from the environment; nothing is committed:
#
#   DAYTONA_SANDBOX   SSH destination for the sandbox (user@host, or an
#                     ssh-config Host alias). Required.
#   DAYTONA_PORT      SSH port, e.g. 2222 (optional)
#   DAYTONA_SSH_KEY   path to the sandbox SSH private key (optional)
#   DAYTONA_SSH_ARGS  extra ssh arguments, word-split, e.g. a ProxyCommand
#                     (optional)
#
# The battery runs against the committed fixtures, so record on a reference
# host first (see README.md). Create the sandbox fresh from its template:
# fresh-from-template is the comparison state (docs/sandboxes.md).
set -eu

cd "$(dirname "$0")"

: "${DAYTONA_SANDBOX:?set DAYTONA_SANDBOX to the sandbox SSH destination (user@host)}"

command -v ssh >/dev/null 2>&1 || { echo "ssh is required" >&2; exit 1; }
command -v smokepod >/dev/null 2>&1 || {
    echo "smokepod must be on PATH (build it with: go build ./cmd/smokepod)" >&2
    exit 1
}

SSH_ARGS="-o StrictHostKeyChecking=accept-new"
[ -n "${DAYTONA_PORT:-}" ] && SSH_ARGS="$SSH_ARGS -p $DAYTONA_PORT"
[ -n "${DAYTONA_SSH_KEY:-}" ] && SSH_ARGS="$SSH_ARGS -i $DAYTONA_SSH_KEY"
if [ -n "${DAYTONA_SSH_ARGS:-}" ]; then
    # shellcheck disable=SC2086
    SSH_ARGS="$SSH_ARGS $DAYTONA_SSH_ARGS"
fi

# Upload the shim, then point smokepod's process target at it over SSH.
# shellcheck disable=SC2086
ssh $SSH_ARGS "$DAYTONA_SANDBOX" \
    'cat > /tmp/smokepod-shim.sh && chmod +x /tmp/smokepod-shim.sh' < shim.sh

# Each ssh argument becomes one --target-arg value.
TARGET_ARGS="--target-arg=-o --target-arg=StrictHostKeyChecking=accept-new"
[ -n "${DAYTONA_PORT:-}" ] && TARGET_ARGS="$TARGET_ARGS --target-arg=-p --target-arg=$DAYTONA_PORT"
[ -n "${DAYTONA_SSH_KEY:-}" ] && TARGET_ARGS="$TARGET_ARGS --target-arg=-i --target-arg=$DAYTONA_SSH_KEY"
if [ -n "${DAYTONA_SSH_ARGS:-}" ]; then
    for a in $DAYTONA_SSH_ARGS; do
        TARGET_ARGS="$TARGET_ARGS --target-arg=$a"
    done
fi

# shellcheck disable=SC2086
smokepod verify \
    --tests . \
    --fixtures fixtures \
    --mode process \
    --target ssh $TARGET_ARGS \
    --target-arg="$DAYTONA_SANDBOX" \
    --target-arg=sh \
    --target-arg=/tmp/smokepod-shim.sh
