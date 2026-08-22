package graph

import (
	"reflect"
	"testing"

	"github.com/TylerAngelier/ticket/internal/tickets"
)

func mk(id, status string, priority int, deps ...string) tickets.Ticket {
	if status == "" {
		status = "open"
	}
	return tickets.Ticket{ID: id, Status: status, Priority: priority, Deps: deps}
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
