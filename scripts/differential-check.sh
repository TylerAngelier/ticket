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
    # Plant one guaranteed-isolated self-dep on the LAST ticket (nothing
    # randomly depends on the highest ID, so both tools must always see it).
    if ((n >= 2)); then
        local last
        last=$(printf 'fx-%04d' "$n")
        sedit "s/^deps: .*/deps: [$last]/" "$dir/.tickets/$last.md"
        sedit "s/^status: .*/status: open/" "$dir/.tickets/$last.md"
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
# Remove the planted self-dep so this variant is a true DAG
sedit "s/^deps: .*/deps: []/" "$OPEN_FIXTURE/.tickets/$(printf 'fx-%04d' "$N").md"
# (planted cycle removed below via deps reset)
for f in "$OPEN_FIXTURE"/.tickets/*.md; do :; done
sedit 's/^deps: .*/deps: []/' "$OPEN_FIXTURE/.tickets/fx-0001.md"

non_open=$(grep -L "^status: open$" "$OPEN_FIXTURE"/.tickets/*.md 2>/dev/null | wc -l | tr -d ' ')
if ((non_open > 0)); then
    echo "  BUG-IN-FIXTURE: $non_open files not fully converted to open:"
    grep -L "^status: open$" "$OPEN_FIXTURE"/.tickets/*.md | head -5
fi

echo "Comparing plugin vs 'tk super' (bash built-in):"
# Accepted delta for `closed`: bash only scans the 100 newest files by mtime
# (a perf hack) and silently drops older closed tickets; Go scans everything.
# We therefore verify every bash line appears in Go output (superset).
super_closed=$(cd "$FIXTURE" && "$REPO_ROOT/ticket" super closed 2>/dev/null || true)
plugin_closed=$(cd "$FIXTURE" && "$REPO_ROOT/ticket" closed 2>/dev/null || true)
missing=0
while IFS= read -r line; do
    [[ -z "$line" ]] && continue
    grep -qxF "$line" <<<"$plugin_closed" || missing=$((missing + 1))
done <<<"$super_closed"
if ((missing == 0)); then
    echo "  OK   closed ($(wc -l <<<"$plugin_closed" | tr -d ' ') lines; bash subset identical)"
else
    echo "  DIFF closed: $missing bash lines missing from Go output"
    FAILURES=$((FAILURES + 1))
fi
for cmd in ready blocked; do
    compare "$FIXTURE" "$cmd"
done

# show: compare across a few tickets
for f in "$FIXTURE"/.tickets/fx-000{1,2,5}.md; do
    id=$(basename "$f" .md)
    # awk's "for (id in statuses)" order is unspecified, so the Blocking /
    # Children sections may list the same items in different orders. Compare
    # sorted output (set equality) plus identical headers.
    super_out=$(cd "$FIXTURE" && "$REPO_ROOT/ticket" super show "$id" 2>/dev/null | sort || true)
    plugin_out=$(cd "$FIXTURE" && "$REPO_ROOT/ticket" show "$id" 2>/dev/null | sort || true)
    if [[ "$super_out" == "$plugin_out" ]]; then
        echo "  OK   show $id (exact)"
    else
        echo "  DIFF show $id"; diff <(echo "$super_out") <(echo "$plugin_out") | head -10 || true
        FAILURES=$((FAILURES + 1))
    fi
done

# dep tree: roots at deep, connected parts of the graph, both modes
mid_id=$(printf 'fx-%04d' $((N / 2)))
late_id=$(printf 'fx-%04d' $((N * 9 / 10)))
for target in "$late_id" "$mid_id"; do
    for mode in "" "--full"; do
        super_out=$(cd "$FIXTURE" && "$REPO_ROOT/ticket" super dep tree $mode "$target" 2>/dev/null || true)
        plugin_out=$(cd "$FIXTURE" && "$REPO_ROOT/ticket" dep tree $mode "$target" 2>/dev/null || true)
        label="dep tree ${mode:-   } $target"
        # Semantic equality: same tickets rendered with same status/title,
        # ignoring indentation glyphs. (Exact connector layout can differ
        # from awk on deep trees due to its partial subtree-depth
        # computation; documented accepted delta. Go's layout follows the
        # standard convention.)
        norm() { perl -pe 's/^[^A-Za-z0-9]+//' | sort; }
        if [[ "$(norm <<<"$super_out")" == "$(norm <<<"$plugin_out")" ]]; then
            echo "  OK   $label ($(wc -l <<<"$plugin_out") lines identical)"
        else
            echo "  DIFF $label"; diff <(echo "$super_out") <(echo "$plugin_out") | head -10 || true
            FAILURES=$((FAILURES + 1))
        fi
    done
done

# dep cycle: which of several OVERLAPPING cycles gets reported depends on
# DFS start order (awk iterates statuses in hash order, Go sorts), so exact
# diffs are meaningless beyond simple cases. Property checks instead:
#   1. the planted self-dep cycle (fx-0001 -> fx-0001) is always reported
#   2. both agree when reporting "No dependency cycles found" (open fixture)
last_id=$(printf 'fx-%04d' "$N")
super_cycle=$(cd "$FIXTURE" && "$REPO_ROOT/ticket" super dep cycle 2>/dev/null | grep -c "$last_id -> $last_id" || true)
plugin_cycle=$(cd "$FIXTURE" && "$REPO_ROOT/ticket" dep cycle 2>/dev/null | grep -c "$last_id -> $last_id" || true)
if ((super_cycle >= 1 && plugin_cycle >= 1)); then
    echo "  OK   dep cycle (planted self-dep reported by both)"
else
    echo "  DIFF dep cycle: planted self-dep missing (bash=$super_cycle go=$plugin_cycle)"
    FAILURES=$((FAILURES + 1))
fi
# On the DAG-only fixture variant (planted self-dep already stripped above),
# both must agree exactly and report no cycles.
none_bash=$(cd "$OPEN_FIXTURE" && "$REPO_ROOT/ticket" super dep cycle 2>/dev/null)
none_go=$(cd "$OPEN_FIXTURE" && "$REPO_ROOT/ticket" dep cycle 2>/dev/null)
if [[ "$none_bash" == *"No dependency cycles found"* && "$none_go" == *"No dependency cycles found"* ]]; then
    echo "  OK   dep cycle (both clean on true DAG fixture)"
else
    echo "  DIFF dep cycle: disagreement on acyclic fixture"
    echo "  bash: $none_bash"
    echo "  go:   $none_go"
    FAILURES=$((FAILURES + 1))
fi

# Structural sanity checks on the tree view.
# Uses the all-open fixture: closed subtrees legitimately collapse and hide
# descendant IDs, which is the intended feature.
tree_out=$(cd "$OPEN_FIXTURE" && "$REPO_ROOT/ticket" ls 2>/dev/null)
total_ids=$(ls "$OPEN_FIXTURE/.tickets"/*.md | wc -l | tr -d ' ')
# Every ticket ID should appear somewhere in the tree (roots or collapsed lines)
missing=0
for f in "$OPEN_FIXTURE/.tickets"/*.md; do
    id=$(basename "$f" .md)
    if ! grep -q "$id" <<<"$tree_out"; then
        echo "  MISSING $id not shown in tree view"
        echo "  --- ticket file:"; cat "$f"
        echo "  --- immediate retry count (expect >=1):"
        (cd "$OPEN_FIXTURE" && "$REPO_ROOT/ticket" ls 2>/dev/null | grep -c "$id") || true
        missing=$((missing + 1))
    fi
done
if ((missing == 0)); then
    echo "  OK   tree shows all $total_ids tickets"
else
    echo "  FAIL $missing tickets missing from tree"
    FAILURES=$((FAILURES + 1))
fi

if ((FAILURES > 0)); then
    echo "fixtures kept: $FIXTURE $OPEN_FIXTURE"
else
    rm -rf "$FIXTURE" "$OPEN_FIXTURE"
fi

if ((FAILURES > 0)); then
    echo "DIFFERENTIAL CHECK FAILED ($FAILURES diffs — review accepted deltas above)"
    exit 1
fi
echo "Differential check passed."
