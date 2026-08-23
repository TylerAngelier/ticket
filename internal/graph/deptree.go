package graph

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveID resolves a full or partial ticket ID the same way bash
// `ticket_path` does: exact match first, then unique substring match.
// Returns an error message identical to the bash implementation on failure.
func (g *Graph) ResolveID(partial string) (string, error) {
	if _, ok := g.Tickets[partial]; ok {
		return partial, nil
	}
	var matches []string
	for id := range g.Tickets {
		if strings.Contains(id, partial) {
			matches = append(matches, id)
		}
	}
	sort.Strings(matches)
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", fmt.Errorf("Error: ticket '%s' not found", partial)
	default:
		return "", fmt.Errorf("Error: ambiguous ID '%s' matches multiple tickets", partial)
	}
}

// DepTree renders the dependency tree rooted at root, matching the bash
// `tk dep tree` output: box-drawing connectors, children sorted by subtree
// depth then ID, deduplicated at each node's deepest occurrence unless full.
func (g *Graph) DepTree(root string, full bool) []string {
	maxDepth := g.maxDepths(root)
	subtree := g.subtreeDepths(root, maxDepth)
	status := func(id string) string {
		if s := g.Tickets[id].Status; s != "" {
			return s
		}
		return "open"
	}

	lines := []string{fmt.Sprintf("%s [%s] %s", root, status(root), g.Tickets[root].Title)}
	printed := map[string]bool{root: true}

	var walk func(id, prefix, connector string)
	walk = func(id, prefix, connector string) {
		kids := g.treeChildren(id, maxDepth, subtree, printed, full)
		// Empirically, the bash implementation always indents descendants
		// by four spaces per level (it never emits vertical-bar
		// continuations), so we mirror that exactly.
		childPrefix := prefix + "    "
		for i, c := range kids {
			// Re-check at print time: an earlier sibling's subtree may have
			// rendered this node already (matches bash's pop-time guard).
			if !full && printed[c] && c != id {
				continue
			}
			last := i == len(kids)-1
			conn := "├── "
			if last {
				conn = "└── "
			}
			lines = append(lines, fmt.Sprintf("%s%s [%s] %s",
				prefix+conn, c, status(c), g.Tickets[c].Title))
			firstTime := !printed[c]
			printed[c] = true
			if firstTime || full {
				walk(c, childPrefix, conn)
			}
		}
	}
	walk(root, "", "")
	return lines
}

// treeChildren returns printable children of id in display order:
// ascending subtree depth, then ascending ID — matching bash insertion sort.
func (g *Graph) treeChildren(id string, maxDepth, subtree map[string]int, printed map[string]bool, full bool) []string {
	var kids []string
	for _, c := range g.Parents[id] {
		if !full && printed[c] {
			continue
		}
		md, ok := maxDepth[c]
		if !ok {
			continue // unreachable from root
		}
		if !full && maxDepth[id]+1 != md {
			continue
		}
		kids = append(kids, c)
	}
	sort.SliceStable(kids, func(i, j int) bool {
		si, sj := subtree[kids[i]], subtree[kids[j]]
		if si != sj {
			return si < sj
		}
		return kids[i] < kids[j]
	})
	return kids
}

// maxDepths computes the maximum depth at which each reachable ticket occurs
// below root (cycle-safe via path tracking).
func (g *Graph) maxDepths(root string) map[string]int {
	depths := map[string]int{}
	var visit func(id string, depth int, path map[string]bool)
	visit = func(id string, depth int, path map[string]bool) {
		if path[id] {
			return
		}
		if d, seen := depths[id]; seen && d >= depth {
			return // already found at equal or greater depth
		}
		depths[id] = depth
		path[id] = true
		for _, c := range g.Parents[id] {
			visit(c, depth+1, path)
		}
		delete(path, id)
	}
	visit(root, 0, map[string]bool{})
	return depths
}

// subtreeDepths computes each node's maximum subtree depth below root,
// mirroring bash's post-order computation.
func (g *Graph) subtreeDepths(root string, maxDepth map[string]int) map[string]int {
	subtree := make(map[string]int, len(maxDepth))
	visiting := map[string]bool{}

	var compute func(id string) int
	compute = func(id string) int {
		if d, ok := subtree[id]; ok {
			return d
		}
		if visiting[id] {
			return maxDepth[id] // cycle back-edge
		}
		visiting[id] = true
		best := maxDepth[id]
		for _, c := range g.Parents[id] {
			if _, reachable := maxDepth[c]; reachable {
				if d := compute(c); d > best {
					best = d
				}
			}
		}
		visiting[id] = false
		subtree[id] = best
		return best
	}
	for id := range maxDepth {
		compute(id)
	}
	return subtree
}

// Cycle is one dependency cycle among open tickets.
type Cycle struct {
	Path    []string // e.g. a -> b -> a
	Members []string // normalized member list (rotation starting at smallest ID)
}

// OpenCycles finds dependency cycles considering only non-closed tickets,
// matching bash `tk dep cycle`.
func (g *Graph) OpenCycles() []Cycle {
	open := map[string]bool{}
	for id, t := range g.Tickets {
		if t.Status != "closed" {
			open[id] = true
		}
	}
	const (
		white = 0
		gray  = 1
		black = 2
	)
	state := map[string]int{}
	var stack []string
	seen := map[string]bool{}
	var out []Cycle

	var dfs func(id string)
	dfs = func(id string) {
		state[id] = gray
		stack = append(stack, id)
		for _, dep := range g.Parents[id] {
			if !open[dep] {
				continue // closed tickets are invisible to `dep cycle`, like bash
			}
			switch state[dep] {
			case white:
				dfs(dep)
			case gray:
				start := len(stack) - 1
				for start >= 0 && stack[start] != dep {
					start--
				}
				if start < 0 {
					continue
				}
				cyc := append(append([]string{}, stack[start:]...), dep)
				key := cycleKey(cyc[:len(cyc)-1])
				if !seen[key] {
					seen[key] = true
					out = append(out, Cycle{
						Path:    cyc,
						Members: normalizeMembers(cyc),
					})
				}
			}
		}
		stack = stack[:len(stack)-1]
		state[id] = black
	}

	ids := make([]string, 0, len(g.Tickets))
	for id := range g.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		if state[id] == white {
			dfs(id)
		}
	}
	return out
}

func normalizeMembers(cyc []string) []string {
	// cyc is closed (last == first); drop duplicate last, rotate to smallest.
	members := cyc[:len(cyc)-1]
	min, mi := members[0], 0
	for i, id := range members {
		if id < min {
			min, mi = id, i
		}
	}
	out := make([]string, 0, len(members))
	for i := 0; i < len(members); i++ {
		out = append(out, members[(mi+i)%len(members)])
	}
	return out
}
