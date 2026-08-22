#!/usr/bin/env bash
# Performance regression gate.
# The bash implementation took ~29s for `tk ready` at 5k tickets (quadratic
# awk sort). The Go implementation must stay well under BUDGET seconds.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
N="${1:-5000}"
BUDGET="${2:-2}"

FIXTURE=$(mktemp -d /tmp/tk-bench.XXXXXX)
trap 'rm -rf "$FIXTURE"' EXIT
mkdir -p "$FIXTURE/.tickets"

python3 - "$FIXTURE/.tickets" "$N" <<'EOF'
import random, sys, os
d, n = sys.argv[1], int(sys.argv[2])
for i in range(1, n + 1):
    id = f'bx-{i:05d}'
    deps = f'[bx-{random.randint(1, i-1):05d}]' if i > 1 and random.random() < 0.3 else '[]'
    status = random.choice(['open'] * 6 + ['in_progress', 'closed'] * 2)
    with open(os.path.join(d, id + '.md'), 'w') as f:
        f.write(f"---\nid: {id}\nstatus: {status}\npriority: {random.randint(0,3)}\ndeps: {deps}\n---\n# Bench {i}\n")
EOF

echo "Benchmarking $N tickets:"
for cmd in ls ready blocked; do
    start=$(date +%s.%N)
    (cd "$FIXTURE" && "$REPO_ROOT/build/ticket-ls" "$cmd" >/dev/null 2>&1)
    end=$(date +%s.%N)
    elapsed=$(echo "$end $start" | awk '{printf "%.2f", $1 - $2}')
    verdict="OK"
    awk -v e="$elapsed" -v b="$BUDGET" 'BEGIN { exit !(e <= b) }' || { verdict="OVER BUDGET"; FAIL=1; }
    echo "  $cmd: ${elapsed}s ($verdict)"
done

if [[ "${FAIL:-0}" == "1" ]]; then
    echo "PERFORMANCE REGRESSION detected (budget ${BUDGET}s per command)"
    exit 1
fi
echo "Performance check passed."
