// Package render turns graphs into terminal output.
package render

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/TylerAngelier/ticket/internal/graph"
	"github.com/TylerAngelier/ticket/internal/tickets"
)

// ANSI dim, used only when color is enabled (TTY output).
const (
	ansiDim   = "\x1b[2m"
	ansiReset = "\x1b[0m"
)

// TreeOptions controls tree rendering.
type TreeOptions struct {
	Color bool // emit ANSI dim for collapsed/closed and context lines
}

type treeRenderer struct {
	g     *graph.Graph
	f     graph.Filter
	opts  TreeOptions
	out   io.Writer
	shown map[string]bool
	inCyc map[string]bool
	path  map[string]bool // ancestors currently being rendered
	vis   map[string]bool // memoized filter visibility (cycle-safe)
}

// Tree renders the hierarchical ticket view.
//
//   - Roots (tickets nobody depends on) in priority, then ID order
//   - Children nested under the first parent that claims them
//   - Fully-closed subtrees collapse to one dimmed line: ▸ id ... · N closed
//   - Cross/missing dependencies render on a dimmed second line
//   - Cycle members are marked ⟲ and reported once on stderr by the caller
func Tree(w io.Writer, g *graph.Graph, f graph.Filter, opts TreeOptions) {
	r := &treeRenderer{
		g: g, f: f, opts: opts, out: w,
		shown: make(map[string]bool, len(g.Tickets)),
		inCyc: make(map[string]bool),
		path:  make(map[string]bool),
		vis:   make(map[string]bool),
	}
	for _, cyc := range g.Cycles {
		for _, id := range cyc {
			r.inCyc[id] = true
		}
	}
	if hasFilter(f) {
		for id := range g.Tickets {
			r.computeVisible(id, make(map[string]bool))
		}
	}
	for _, root := range g.Roots() {
		r.node(root, "", true, false)
	}
}

func (r *treeRenderer) dim(s string) string {
	if !r.opts.Color {
		return s
	}
	return ansiDim + s + ansiReset
}

// computeVisible memoizes whether id matches the filter or is an ancestor of
// something that matches. The on-stack set makes it safe on dependency cycles.
func (r *treeRenderer) computeVisible(id string, onStack map[string]bool) bool {
	if v, ok := r.vis[id]; ok {
		return v
	}
	if onStack[id] {
		return false // back-edge inside a cycle; other paths decide
	}
	onStack[id] = true
	defer delete(onStack, id)
	v := r.f.Matches(r.g.Tickets[id])
	if !v {
		for _, c := range r.g.Children[id] {
			if r.computeVisible(c, onStack) {
				v = true
				break
			}
		}
	}
	r.vis[id] = v
	return v
}

// visible reports the memoized visibility computed by computeVisible.
func (r *treeRenderer) visible(id string) bool { return r.vis[id] }

func (r *treeRenderer) node(id, prefix string, last, isRoot bool) {
	if r.shown[id] {
		return // already rendered under another parent (multi-parent ticket)
	}
	r.shown[id] = true
	r.path[id] = true
	defer delete(r.path, id)
	t := r.g.Tickets[id]

	// Skip subtrees that contain nothing visible under an active filter.
	if hasFilter(r.f) && !r.visible(id) {
		return
	}

	connector := ""
	if !isRoot {
		connector = "├── "
		if last {
			connector = "└── "
		}
	}

	// Collapse fully-closed subtrees to a single line (only when there is
	// an actual subtree to hide; lone closed leaves render normally).
	if t.Status == "closed" && r.subtreeSize(id) > 1 && r.subtreeAllClosed(id) {
		n := r.subtreeSize(id)
		extra := ""
		if n > 1 {
			extra = fmt.Sprintf(" · %d closed", n)
		}
		marker := ""
		if r.inCyc[id] {
			marker = " ⟲"
		}
		fmt.Fprintf(r.out, "%s%s▸ %s [P%d][closed] %s%s%s\n",
			prefix, connector, t.ID, t.Priority, t.Title, marker, r.dim(extra))
		return
	}

	marker := ""
	if r.inCyc[id] {
		marker = " ⟲"
	}
	context := ""
	if hasFilter(r.f) && !r.f.Matches(t) {
		context = r.dim(" (context)")
	}
	line := fmt.Sprintf("%s%s%s [P%d][%s] %s%s%s",
		prefix, connector, t.ID, t.Priority, t.Status, t.Title, marker, context)
	if t.Status == "closed" {
		line = r.dim(line)
	}
	fmt.Fprintln(r.out, line)

	// Second line: dependencies that are not rendered as this node's parent.
	if depLine := r.depLine(id); depLine != "" {
		fmt.Fprintf(r.out, "%s%s\n", childPrefix(prefix, connector, last), r.dim(depLine))
	}

	kids := r.visibleChildren(id)
	for i, c := range kids {
		cl := i == len(kids)-1
		r.node(c, childPrefix(prefix, connector, last), cl, false)
	}
}

// depLine lists dependencies not shown structurally above this ticket:
// cross-links to tickets outside this subtree, extra parents, and missing refs.
func (r *treeRenderer) depLine(id string) string {
	t := r.g.Tickets[id]
	var parts []string
	for _, dep := range t.Deps {
		if r.path[dep] {
			continue // already visible structurally in the current path
		}
		dt, ok := r.g.Tickets[dep]
		if !ok {
			parts = append(parts, dep+" (missing)")
			continue
		}
		// Structural parent (this ticket renders under it): skip.
		if len(r.g.Children[dep]) > 0 {
			isStructural := false
			for _, c := range r.g.Children[dep] {
				if c == id {
					isStructural = true
					break
				}
			}
			if isStructural {
				continue
			}
		}
		status := ""
		if dt.Status != "closed" {
			status = " ⛔"
		}
		parts = append(parts, dep+status)
	}
	if len(parts) == 0 {
		return ""
	}
	return "┆ deps: " + strings.Join(parts, ", ")
}

func (r *treeRenderer) visibleChildren(id string) []string {
	var out []string
	for _, c := range r.g.Children[id] {
		if r.shown[c] {
			continue
		}
		if hasFilter(r.f) && !r.visible(c) {
			continue
		}
		out = append(out, c)
	}
	return out
}

func (r *treeRenderer) subtreeAllClosed(id string) bool {
	return r.subtreeAllClosedVisit(id, make(map[string]bool))
}

func (r *treeRenderer) subtreeAllClosedVisit(id string, seen map[string]bool) bool {
	if seen[id] {
		return true // already verified on this path; breaks dependency cycles
	}
	seen[id] = true
	t := r.g.Tickets[id]
	if t.Status != "closed" {
		return false
	}
	for _, c := range r.g.Children[id] {
		if !r.subtreeAllClosedVisit(c, seen) {
			return false
		}
	}
	return true
}

func (r *treeRenderer) subtreeSize(id string) int {
	return r.subtreeSizeVisit(id, make(map[string]bool))
}

func (r *treeRenderer) subtreeSizeVisit(id string, seen map[string]bool) int {
	if seen[id] {
		return 0 // cycle back-edge; already counted via the first path
	}
	seen[id] = true
	n := 1
	for _, c := range r.g.Children[id] {
		n += r.subtreeSizeVisit(c, seen)
	}
	return n
}

func childPrefix(prefix, connector string, last bool) string {
	if isRootConnector(connector) {
		return prefix
	}
	if last {
		return prefix + "    "
	}
	return prefix + "│   "
}

func isRootConnector(c string) bool { return c == "" }

func hasFilter(f graph.Filter) bool {
	return f.Status != "" || f.Assignee != "" || f.Tag != ""
}

// Flat prints one line per ticket: "id [Pn][status] title", sorted by
// priority then ID. Used by --flat for scripting.
func Flat(w io.Writer, all []tickets.Ticket, f graph.Filter) {
	sorted := make([]tickets.Ticket, len(all))
	copy(sorted, all)
	sortTickets(sorted)
	for i := range sorted {
		t := &sorted[i]
		if !f.Matches(t) {
			continue
		}
		fmt.Fprintf(w, "%-8s [P%d][%s] %s\n", t.ID, t.Priority, t.Status, t.Title)
	}
}

// Ready prints tickets that are open/in_progress with every dependency
// closed, matching the bash built-in output format byte-for-byte
// (numeric priority sort is the one deliberate improvement).
func Ready(w io.Writer, all []tickets.Ticket, f graph.Filter) {
	rows := make([]tickets.Ticket, 0, len(all))
	byID := make(map[string]*tickets.Ticket, len(all))
	for i := range all {
		byID[all[i].ID] = &all[i]
	}
	for i := range all {
		t := &all[i]
		if t.Status != "open" && t.Status != "in_progress" {
			continue
		}
		if !f.Matches(t) {
			continue
		}
		blocked := false
		for _, dep := range t.Deps {
			if dt, ok := byID[dep]; !ok || dt.Status != "closed" {
				blocked = true // missing or unclosed dep blocks, matching bash
				break
			}
		}
		if !blocked {
			rows = append(rows, *t)
		}
	}
	sortTickets(rows)
	for i := range rows {
		t := &rows[i]
		fmt.Fprintf(w, "%-8s [P%d][%s] - %s\n", t.ID, t.Priority, t.Status, t.Title)
	}
}

// Blocked prints active tickets with at least one unclosed dependency,
// including the open blockers, matching the bash built-in output format.
func Blocked(w io.Writer, all []tickets.Ticket, f graph.Filter) {
	byID := make(map[string]*tickets.Ticket, len(all))
	for i := range all {
		byID[all[i].ID] = &all[i]
	}
	type row struct {
		t        tickets.Ticket
		blockers []string
	}
	var rows []row
	for i := range all {
		t := &all[i]
		if t.Status != "open" && t.Status != "in_progress" {
			continue
		}
		if !f.Matches(t) {
			continue
		}
		var blockers []string
		for _, dep := range t.Deps {
			dt, ok := byID[dep]
			if !ok || dt.Status != "closed" {
				blockers = append(blockers, dep) // missing or unclosed, matching bash
			}
		}
		if len(blockers) > 0 {
			rows = append(rows, row{*t, blockers})
		}
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].t.Priority != rows[j].t.Priority {
			return rows[i].t.Priority < rows[j].t.Priority
		}
		return rows[i].t.ID < rows[j].t.ID
	})
	for _, r := range rows {
		fmt.Fprintf(w, "%-8s [P%d][%s] - %s <- [%s]\n",
			r.t.ID, r.t.Priority, r.t.Status, r.t.Title, strings.Join(r.blockers, ", "))
	}
}

func sortTickets(ts []tickets.Ticket) {
	sort.SliceStable(ts, func(i, j int) bool {
		if ts[i].Priority != ts[j].Priority {
			return ts[i].Priority < ts[j].Priority
		}
		return ts[i].ID < ts[j].ID
	})
}
