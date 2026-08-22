// Package graph builds the parent/child and dependency graphs over tickets.
//
// Semantics (matching `tk dep tree`): a ticket T that depends on P renders
// as a CHILD of P. In other words parents depend on their children: an epic
// is not ready until its stories are closed.
package graph

import (
	"sort"

	"github.com/TylerAngelier/ticket/internal/tickets"
)

// Graph is the derived structure over a set of tickets.
type Graph struct {
	Tickets  map[string]*tickets.Ticket
	Children map[string][]string // parent -> child IDs (sorted by priority, then ID)
	Parents  map[string][]string // child -> existing parent IDs
	Missing  map[string][]string // ticket ID -> referenced but nonexistent dep IDs
	Cycles   [][]string          // each detected dependency cycle, in path order

	roots []string // sorted root IDs
}

// Filter narrows which tickets are shown. Empty fields match everything.
type Filter struct {
	Status   string
	Assignee string
	Tag      string
}

// Matches reports whether t satisfies f.
func (f Filter) Matches(t *tickets.Ticket) bool {
	if f.Status != "" && t.Status != f.Status {
		return false
	}
	if f.Assignee != "" && t.Assignee != f.Assignee {
		return false
	}
	if f.Tag != "" && !hasTag(t.Tags, f.Tag) {
		return false
	}
	return true
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

// Build derives the graphs. Tickets with duplicate IDs keep the first seen.
func Build(all []tickets.Ticket) *Graph {
	g := &Graph{
		Tickets:  make(map[string]*tickets.Ticket, len(all)),
		Children: make(map[string][]string),
		Parents:  make(map[string][]string),
		Missing:  make(map[string][]string),
	}

	ids := make([]string, 0, len(all))
	for i := range all {
		t := &all[i]
		if _, dup := g.Tickets[t.ID]; dup {
			continue
		}
		g.Tickets[t.ID] = t
		ids = append(ids, t.ID)
	}

	isChild := make(map[string]bool)
	for _, id := range ids {
		t := g.Tickets[id]
		for _, dep := range t.Deps {
			if _, ok := g.Tickets[dep]; !ok {
				g.Missing[id] = append(g.Missing[id], dep)
				continue // missing deps are independent work, never structural
			}
			g.Parents[id] = append(g.Parents[id], dep)
			g.Children[dep] = append(g.Children[dep], id)
			isChild[id] = true
		}
	}

	g.detectCycles(ids)

	// Sort children everywhere by priority then ID.
	for p := range g.Children {
		sort.Slice(g.Children[p], func(i, j int) bool {
			a, b := g.Tickets[g.Children[p][i]], g.Tickets[g.Children[p][j]]
			if a.Priority != b.Priority {
				return a.Priority < b.Priority
			}
			return a.ID < b.ID
		})
	}

	// Roots: everyone who is nobody's child, plus cycle members (a pure
	// cycle has no root; promote members so they still render).
	inCycle := make(map[string]bool)
	for _, cyc := range g.Cycles {
		for _, id := range cyc {
			inCycle[id] = true
		}
	}
	for _, id := range ids {
		if !isChild[id] || inCycle[id] {
			g.roots = append(g.roots, id)
		}
	}
	sort.Slice(g.roots, func(i, j int) bool {
		a, b := g.Tickets[g.roots[i]], g.Tickets[g.roots[j]]
		if a.Priority != b.Priority {
			return a.Priority < b.Priority
		}
		return a.ID < b.ID
	})
	return g
}

// Roots returns the top-level ticket IDs in display order.
func (g *Graph) Roots() []string { return g.roots }

// detectCycles finds every distinct dependency cycle using DFS with colors.
// Each cycle is reported once, as a path like [a, b, c, a].
func (g *Graph) detectCycles(ids []string) {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := make(map[string]int, len(ids))
	var stack []string
	reported := make(map[string]bool) // canonical cycle key -> reported

	var dfs func(id string)
	dfs = func(id string) {
		color[id] = gray
		stack = append(stack, id)
		for _, dep := range g.parentRefs(id) {
			switch color[dep] {
			case white:
				dfs(dep)
			case gray:
				// Found a cycle: slice the stack from dep to top.
				start := len(stack) - 1
				for start >= 0 && stack[start] != dep {
					start--
				}
				cyc := append(append([]string{}, stack[start:]...), dep)
				key := cycleKey(cyc)
				if !reported[key] {
					reported[key] = true
					g.Cycles = append(g.Cycles, cyc)
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[id] = black
	}
	for _, id := range ids {
		if color[id] == white {
			dfs(id)
		}
	}
}

// parentRefs returns the existing tickets this one depends on (its parents).
func (g *Graph) parentRefs(id string) []string { return g.Parents[id] }

func cycleKey(cyc []string) string {
	// Rotate so the smallest ID is first, making cycles comparable.
	min, mi := cyc[0], 0
	for i, id := range cyc {
		if id < min {
			min, mi = id, i
		}
	}
	n := len(cyc)
	out := make([]string, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, cyc[(mi+i)%n])
	}
	sort.Strings(out)
	key := ""
	for _, s := range out {
		key += s + "\x00"
	}
	return key
}
