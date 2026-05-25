#!/usr/bin/env bash
# build-pgo.sh — three-step Profile-Guided Optimization build of sakura.
#
#   1. build a non-PGO sakura
#   2. run it under `sakura profile` against a representative workload
#      to collect a CPU profile
#   3. rebuild with `go build -pgo=<profile> -o sakura ./cmd`
#
# The resulting binary feeds the collected CPU hotspots back into the Go
# compiler's inlining / devirtualisation heuristics. Typical wins on a
# tight interpreter loop are 5-15%.
#
# Usage:
#   scripts/build-pgo.sh                    # profiles examples/02_functions.sakura
#   scripts/build-pgo.sh path/to/script.sk  # profiles your own workload
#
# Pre-reqs: bash, go ≥ 1.21 (for -pgo), the sakura source tree.
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

WORKLOAD="${1:-examples/02_functions.sakura}"
CPU_PROFILE="${REPO_ROOT}/cpu.pgo"
STAGE1_BIN="${REPO_ROOT}/sakura-stage1"
FINAL_BIN="${REPO_ROOT}/sakura"

if [[ ! -f "$WORKLOAD" ]]; then
    echo "build-pgo: workload not found: $WORKLOAD" >&2
    exit 1
fi

echo "==> stage 1: building non-PGO interpreter"
go build -o "$STAGE1_BIN" ./cmd

echo "==> stage 2: profiling against $WORKLOAD"
# -count 50 keeps the profile sample count high enough that the resulting
# `default.pgo`-style data is statistically useful. Bump it for long
# workloads, drop it for quick ones.
"$STAGE1_BIN" profile -cpu "$CPU_PROFILE" -count 50 "$WORKLOAD"

if [[ ! -s "$CPU_PROFILE" ]]; then
    echo "build-pgo: profile is empty — workload may be too short to sample" >&2
    exit 1
fi

echo "==> stage 3: PGO-guided rebuild"
go build -pgo="$CPU_PROFILE" -o "$FINAL_BIN" ./cmd

echo
echo "PGO build complete:"
echo "  profile: $CPU_PROFILE"
echo "  binary:  $FINAL_BIN"
echo
echo "Compare against the non-PGO build:"
echo "  $STAGE1_BIN <script>   # baseline"
echo "  $FINAL_BIN  <script>   # PGO"
