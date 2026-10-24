#!/bin/sh
# Local no-credentials proof for the sandbox conformance battery: verify the
# committed baseline inside an Alpine (busybox/musl) container through
# shim.sh, driven by smokepod's process-mode JSON-RPC framing.
#
# --cpuset-cpus=0 emulates the provider-allocated CPU-count divergence that
# sandboxes document (see docs/sandboxes.md); it needs a multi-core host so
# the cpu-count section genuinely xfails rather than xpassing.
set -eu

cd "$(dirname "$0")"

IMAGE="${SMOKEPOD_SANDBOX_IMAGE:-alpine:latest}"
CTR=smokepod-sandbox-conformance
CPUS="${SMOKEPOD_SANDBOX_CPUS:-0}"

command -v docker >/dev/null 2>&1 || { echo "docker is required" >&2; exit 1; }
command -v smokepod >/dev/null 2>&1 || {
    echo "smokepod must be on PATH (build it with: go build ./cmd/smokepod)" >&2
    exit 1
}

docker rm -f "$CTR" >/dev/null 2>&1 || true
docker run -d --name "$CTR" --cpuset-cpus="$CPUS" "$IMAGE" tail -f /dev/null >/dev/null
docker cp shim.sh "$CTR:/shim.sh"
trap 'docker rm -f "$CTR" >/dev/null 2>&1 || true' EXIT

smokepod verify \
    --tests . \
    --fixtures fixtures \
    --mode process \
    --target docker \
    --target-arg=exec \
    --target-arg=-i \
    --target-arg="$CTR" \
    --target-arg=sh \
    --target-arg=/shim.sh
