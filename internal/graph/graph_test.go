package graph

import (
	"reflect"
	"strings"
	"testing"

	"github.com/TylerAngelier/ticket/internal/tickets"
)

func mk(id, status string, priority int, deps ...string) tickets.Ticket {
	if status == "" {
		status = "open"
	}
	return tickets.Ticket{ID: id, Status: status, Priority: priority, Deps: deps, Title: "Title " + id}
}

func build(ts ...tickets.Ticket) *Graph { return Build(ts) }

func TestParentChildDirection(t *testing.T) {
	// epic depends on story => story is a child of epic.
	g := build(mk("epic-1", "", 1), mk("sto-1", "", 2, "epic-1"))
	if roots := g.Roots(); len(roots) != 1 || roots[0] != "epic-1" {
		t.Fatalf("roots = %v, want [epic-1]", roots)
	}
	if kids := g.Children["epic-1"]; !reflect.DeepEqual(kids, []string{"sto-1"}) {
		t.Errorf("children = %v", kids)
	}
}

func TestRootsSortedPriorityThenID(t *testing.T) {
	g := build(
		mk("b-2", "closed", 1),
		mk("b-1", "", 1),
		mk("c-1", "", 3),
		mk("a-9", "", 1),
	)
	want := []string{"a-9", "b-1", "b-2", "c-1"}
	if got := g.Roots(); !reflect.DeepEqual(got, want) {
		t.Fatalf("roots = %v, want %v", got, want)
	}
}

func TestMissingDepsAreNotStructural(t *testing.T) {
	g := build(mk("a-1", "", 1, "ghost-99"))
	if roots := g.Roots(); len(roots) != 1 || roots[0] != "a-1" {
		t.Fatalf("missing dep must not change structure, roots = %v", roots)
	}
	if g.Missing["a-1"] == nil || g.Missing["a-1"][0] != "ghost-99" {
		t.Errorf("Missing = %v", g.Missing)
	}
}

func TestCycleDetection(t *testing.T) {
	g := build(
		mk("a-1", "", 1, "b-1"),
		mk("b-1", "", 1, "c-1"),
		mk("c-1", "", 1, "a-1"),
		mk("x-1", "", 2),
	)
	if len(g.Cycles) != 1 {
		t.Fatalf("cycles = %v, want 1", g.Cycles)
	}
	cyc := g.Cycles[0]
	// Path order: a-1 -> b-1 -> c-1 -> a-1
	if cyc[0] != cyc[len(cyc)-1] {
		t.Errorf("cycle not closed: %v", cyc)
	}
	if len(cyc) != 4 {
		t.Errorf("cycle = %v, want a,b,c,a", cyc)
	}
	// Cycle members are promoted to roots so they still render.
	found := map[string]bool{}
	for _, r := range g.Roots() {
		found[r] = true
	}
	if !found["a-1"] || !found["b-1"] || !found["c-1"] || !found["x-1"] {
		t.Errorf("roots = %v; cycle members and x-1 expected", g.Roots())
	}
}

func TestSelfCycle(t *testing.T) {
	g := build(mk("a-1", "", 1, "a-1"))
	if len(g.Cycles) != 1 || len(g.Cycles[0]) != 2 {
		t.Fatalf("cycles = %v", g.Cycles)
	}
}

func TestDuplicateIDKeepsFirst(t *testing.T) {
	g := build(mk("a-1", "closed", 1), mk("a-1", "open", 3))
	if g.Tickets["a-1"].Status != "closed" {
		t.Errorf("duplicate handling broken")
	}
}

func TestFilterMatching(t *testing.T) {
	f := Filter{Assignee: "tyler", Tag: "api"}
	ok := mk("a-1", "open", 1)
	ok.Assignee = "tyler"
	ok.Tags = []string{"api", "web"}
	no := mk("b-1", "open", 1)
	no.Assignee = "other"
	if !f.Matches(&ok) || f.Matches(&no) {
		t.Error("filter matching broken")
	}
	if !(Filter{}).Matches(&no) {
		t.Error("empty filter should match everything")
	}
}

func TestChildrenSortedPriorityThenID(t *testing.T) {
	g := build(
		mk("p-1", "", 1),
		mk("z-1", "", 3, "p-1"),
		mk("a-2", "", 3, "p-1"),
		mk("m-9", "", 2, "p-1"),
	)
	want := []string{"m-9", "a-2", "z-1"} // P2 first, then P3 sorted by id
	if got := g.Children["p-1"]; !reflect.DeepEqual(got, want) {
		t.Fatalf("children = %v, want %v", got, want)
	}
}

func TestDepTreeDirectionAndOrder(t *testing.T) {
	// a-1 depends on b-1 and c-1; b-1 depends on d-1 (deep).
	g := build(
		mk("a-1", "", 1, "b-1", "c-1"),
		mk("b-1", "", 2, "d-1"),
		mk("c-1", "", 3),
		mk("d-1", "", 4),
	)
	lines := g.DepTree("a-1", false)
	want := []string{
		"a-1 [open] Title a-1",
		"├── c-1 [open] Title c-1", // shallow branch first (bash subtree-depth sort)
		"└── b-1 [open] Title b-1",
		"    └── d-1 [open] Title d-1",
	}
	if !reflect.DeepEqual(lines, want) {
		t.Errorf("got:\n%s\nwant:\n%s", strings.Join(lines, "\n"), strings.Join(want, "\n"))
	}
}

func TestDepTreeShallowBeforeDeep(t *testing.T) {
	// Children sorted by subtree depth ascending then ID.
	g := build(
		mk("r-1", "", 1, "z-9", "a-9", "m-5"),
		mk("z-9", "", 2),
		mk("a-9", "", 2),
		mk("m-5", "", 2, "deep-1"),
		mk("deep-1", "", 3),
	)
	lines := g.DepTree("r-1", false)
	var order []string
	for _, l := range lines[1:] {
		order = append(order, strings.Fields(strings.TrimLeft(l, "├└─ "))[0])
	}
	want := []string{"a-9", "z-9", "m-5", "deep-1"} // shallow leaves first, deep branch last
	if !reflect.DeepEqual(order, want) {
		t.Errorf("order = %v, want %v\n%s", order, want, strings.Join(lines, "\n"))
	}
}

func TestResolveID(t *testing.T) {
	g := build(mk("abc-1234", "open", 1), mk("abd-5678", "open", 1))
	if id, err := g.ResolveID("abc-1234"); err != nil || id != "abc-1234" {
		t.Errorf("exact: %v %v", id, err)
	}
	if id, err := g.ResolveID("1234"); err != nil || id != "abc-1234" {
		t.Errorf("partial: %v %v", id, err)
	}
	if _, err := g.ResolveID("zzzz"); err == nil || !strings.Contains(err.Error(), "not found") {
		t.Errorf("missing: %v", err)
	}
	if _, err := g.ResolveID("ab"); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("ambiguous: %v", err)
	}
}

func TestOpenCyclesExcludesClosed(t *testing.T) {
	g := build(
		mk("a-1", "closed", 1, "b-1"), // closed: cycle invisible to dep cycle
		mk("b-1", "open", 1, "a-1"),
		mk("c-1", "open", 1, "d-1"),
		mk("d-1", "open", 1, "c-1"),
	)
	cycles := g.OpenCycles()
	if len(cycles) != 1 {
		t.Fatalf("cycles = %d, want 1 (only the open one)", len(cycles))
	}
	if cycles[0].Members[0] != "c-1" {
		t.Errorf("members = %v, want rotation from c-1", cycles[0].Members)
	}
}
