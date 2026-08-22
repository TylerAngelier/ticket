#!/usr/bin/env bash
# Differential testing: compare Go plugin output against the bash built-ins
# (`tk super <cmd>`) across generated fixtures.
#
# Usage: scripts/differential-check.sh [num_tickets]
# Requires: build/ticket-ls (+ symlinks) built via `make build`.
#
# Known accepted deltas:
#   - Priority sorting is numeric in Go ("P10" sorts after "P9");
#     awk sorted priorities as strings.
#   - `ready`/`blocked` output is byte-identical otherwise.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BIN_DIR="$REPO_ROOT/build"
export PATH="$BIN_DIR:$PATH"
N="${1:-300}"
FAILURES=0

# Portable in-place sed (GNU needs no backup arg, BSD requires '')
sedit() {
    if sed --version >/dev/null 2>&1; then sed -i "$1" "$2"; else sed -i '' "$1" "$2"; fi
}

command -v go >/dev/null || { echo "go not found; run inside a machine with Go"; exit 1; }
[[ -x "$BIN_DIR/ticket-ls" ]] || { echo "run 'make build' first"; exit 1; }

# Generate a random fixture set
gen_fixture() {
    local dir="$1" n="$2"
    mkdir -p "$dir/.tickets"
    local ids=()
    for ((i = 1; i <= n; i++)); do
        local id="fx-$(printf '%04d' $i)"
        ids+=("$id")
        local status="open" prio=$((RANDOM % 4)) deps=()
        case $((RANDOM % 5)) in
            1) status=in_progress ;;
            2) status=closed ;;
        esac
        # Random deps on earlier tickets (keeps mostly acyclic)
        local ndeps=$((RANDOM % 3))
        for ((d = 0; d < ndeps; d++)); do
            j=$((RANDOM % i))
            deps+=("fx-$(printf '%04d' $j)")
        done
        local dep_str
        if ((${#deps[@]})); then
            dep_str="$(IFS=,; echo "${deps[*]}")"
            dep_str="[$dep_str]"
        else
            dep_str="[]"
        fi
        cat >"$dir/.tickets/$id.md" <<EOF
---
id: $id
status: $status
priority: $prio
deps: $dep_str
---
# Ticket $i
EOF
    done
    # Sprinkle a cycle occasionally
    if ((n >= 3)); then
        sedit 's/^status: .*/status: open/' "$dir/.tickets/fx-0002.md"
        sedit 's/^deps: .*/deps: [fx-0001]/' "$dir/.tickets/fx-0001.md"
        sedit 's/^deps: .*/deps: [fx-0002]/' "$dir/.tickets/fx-0003.md"
    fi
}

compare() {
    local fixture="$1" cmd="$2"
    local super_out plugin_out
    super_out=$(cd "$fixture" && "$REPO_ROOT/ticket" super $cmd 2>/dev/null || true)
    plugin_out=$(cd "$fixture" && "$REPO_ROOT/ticket" $cmd 2>/dev/null || true)

    if [[ "$super_out" == "$plugin_out" ]]; then
        echo "  OK   $cmd ($(wc -l <<<"$plugin_out") lines identical)"
    else
        echo "  DIFF $cmd"
        diff <(echo "$super_out") <(echo "$plugin_out") | head -10 || true
        FAILURES=$((FAILURES + 1))
    fi
}

echo "Generating fixtures: $N tickets"
FIXTURE=$(mktemp -d /tmp/tk-diff.XXXXXX)
gen_fixture "$FIXTURE" "$N"

# All-open variant for tree-completeness checking (nothing may collapse)
OPEN_FIXTURE=$(mktemp -d /tmp/tk-diff-open.XXXXXX)
gen_fixture "$OPEN_FIXTURE" "$N"
for f in "$OPEN_FIXTURE"/.tickets/*.md; do sedit 's/^status: .*/status: open/' "$f"; done

echo "Comparing plugin vs 'tk super' (bash built-in):"
for cmd in ready blocked; do
    compare "$FIXTURE" "$cmd"
done

# Structural sanity checks on the tree view.
# Uses the all-open fixture: closed subtrees legitimately collapse and hide
# descendant IDs, which is the intended feature.
tree_out=$(cd "$OPEN_FIXTURE" && "$REPO_ROOT/ticket" ls 2>/dev/null)
total_ids=$(ls "$OPEN_FIXTURE/.tickets"/*.md | wc -l | tr -d ' ')
# Every ticket ID should appear somewhere in the tree (roots or collapsed lines)
missing=0
for f in "$OPEN_FIXTURE/.tickets"/*.md; do
    id=$(basename "$f" .md)
    grep -q "$id" <<<"$tree_out" || { echo "  MISSING $id not shown in tree view"; missing=$((missing + 1)); }
done
if ((missing == 0)); then
    echo "  OK   tree shows all $total_ids tickets"
else
    echo "  FAIL $missing tickets missing from tree"
    FAILURES=$((FAILURES + 1))
fi

rm -rf "$FIXTURE" "$OPEN_FIXTURE"

if ((FAILURES > 0)); then
    echo "DIFFERENTIAL CHECK FAILED ($FAILURES diffs — review accepted deltas above)"
    exit 1
fi
echo "Differential check passed."
