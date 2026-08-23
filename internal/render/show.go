package render

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/TylerAngelier/ticket/internal/graph"
	"github.com/TylerAngelier/ticket/internal/tickets"
)

// Closed prints recently closed tickets, most recently modified first,
// matching the bash built-in (`ls -t`, statuses closed/done, limit).
func Closed(w io.Writer, all []tickets.Ticket, f graph.Filter, limit int) {
	rows := make([]tickets.Ticket, 0, len(all))
	for i := range all {
		t := &all[i]
		if t.Status != "closed" && t.Status != "done" {
			continue
		}
		if !f.Matches(t) {
			continue
		}
		rows = append(rows, *t)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].ModTime.After(rows[j].ModTime)
	})
	if limit > 0 && len(rows) > limit {
		rows = rows[:limit]
	}
	for _, t := range rows {
		fmt.Fprintf(w, "%-8s [%s] - %s\n", t.ID, t.Status, t.Title)
	}
}

// Show reproduces bash `tk show`: the raw ticket file with the parent line
// annotated by title, followed by Blockers / Blocking / Children / Linked.
func Show(w io.Writer, dir string, target string, g *graph.Graph) error {
	t := g.Tickets[target]
	raw, err := os.ReadFile(filepath.Join(dir, target+".md"))
	if err != nil {
		return err
	}

	// Missing referenced tickets render with empty status/title, like awk.
	status := func(id string) string {
		if t, ok := g.Tickets[id]; ok {
			return t.Status
		}
		return ""
	}
	title := func(id string) string {
		if t, ok := g.Tickets[id]; ok {
			return t.Title
		}
		return ""
	}

	inFront := false
	lines := strings.Split(strings.TrimSuffix(string(raw), "\n"), "\n")
	var out []string
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			inFront = !inFront
			out = append(out, line)
			continue
		}
		if inFront && strings.HasPrefix(line, "parent:") {
			p := strings.TrimSpace(strings.TrimPrefix(line, "parent:"))
			if pt, ok := g.Tickets[p]; ok && pt.Title != "" {
				out = append(out, fmt.Sprintf("%s  # %s", line, pt.Title))
				continue
			}
		}
		out = append(out, line)
	}
	fmt.Fprintln(w, strings.TrimRight(strings.Join(out, "\n"), "\n"))

	emit := func(header string, ids []string) {
		if len(ids) == 0 {
			return
		}
		fmt.Fprintf(w, "\n%s\n\n", header)
		for _, id := range ids {
			fmt.Fprintf(w, "- %s [%s] %s\n", id, status(id), title(id))
		}
	}

	// Blockers: unclosed deps of target (in declared order)
	var blockers []string
	for _, d := range t.Deps {
		if dt, ok := g.Tickets[d]; !ok || dt.Status != "closed" {
			blockers = append(blockers, d) // missing or unclosed, matching bash
		}
	}
	emit("## Blockers", blockers)

	// Blocking: open tickets whose deps include target (self-deps included,
	// matching bash)
	var blocking []string
	for _, id := range sortedIDs(g) {
		if g.Tickets[id].Status == "closed" {
			continue
		}
		for _, d := range g.Tickets[id].Deps {
			if d == target {
				blocking = append(blocking, id) // once per reference, like bash
			}
		}
	}
	emit("## Blocking", blocking)

	// Children: tickets with parent == target
	var children []string
	for _, id := range sortedIDs(g) {
		if g.Tickets[id].Parent == target {
			children = append(children, id)
		}
	}
	emit("## Children", children)

	// Linked
	if len(t.Links) > 0 {
		fmt.Fprintf(w, "\n%s\n\n", "## Linked")
		for _, l := range t.Links {
			fmt.Fprintf(w, "- %s [%s] %s\n", l, status(l), title(l))
		}
	}
	return nil
}

func sortedIDs(g *graph.Graph) []string {
	ids := make([]string, 0, len(g.Tickets))
	for id := range g.Tickets {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
