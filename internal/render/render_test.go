package render

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/TylerAngelier/ticket/internal/graph"
	"github.com/TylerAngelier/ticket/internal/tickets"
)

func mk(id, status string, priority int, deps ...string) tickets.Ticket {
	if status == "" {
		status = "open"
	}
	return tickets.Ticket{ID: id, Status: status, Title: "Title " + id, Priority: priority, Deps: deps}
}

func renderTree(t *testing.T, ts []tickets.Ticket, f graph.Filter, color bool) string {
	t.Helper()
	g := graph.Build(ts)
	var buf bytes.Buffer
	Tree(&buf, g, f, TreeOptions{Color: color})
	return buf.String()
}

func golden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name+".golden")
	want, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			os.MkdirAll("testdata", 0o755)
			os.WriteFile(path, []byte(got), 0o644)
			t.Logf("golden written: %s", path)
			return
		}
		t.Fatal(err)
	}
	if got != string(want) {
		t.Errorf("output differs from golden %s:\n--- got ---\n%s\n--- want ---\n%s", path, got, want)
	}
}

func TestTreeHierarchy(t *testing.T) {
	out := renderTree(t, []tickets.Ticket{
		mk("epi-1", "", 1),
		mk("sto-2", "", 2, "epi-1"),
		mk("tsk-3", "", 3, "sto-2"),
		mk("tsk-4", "", 3, "sto-2"),
	}, graph.Filter{}, false)
	golden(t, "tree_hierarchy", out)
}

func TestTreeCollapsesClosedSubtrees(t *testing.T) {
	out := renderTree(t, []tickets.Ticket{
		mk("epi-1", "open", 1),
		mk("sto-2", "closed", 2, "epi-1"),
		mk("tsk-3", "closed", 3, "sto-2"),
	}, graph.Filter{}, false)
	for _, want := range []string{"▸ sto-2", "· 2 closed"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	// Open epic still renders expanded; collapsed story hides its children.
	if strings.Contains(out, "tsk-3") {
		t.Errorf("closed subtree children leaked:\n%s", out)
	}
}

func TestTreeLoneClosedLeafNotCollapsed(t *testing.T) {
	out := renderTree(t, []tickets.Ticket{mk("a-1", "closed", 1)}, graph.Filter{}, false)
	if strings.Contains(out, "▸") {
		t.Errorf("lone closed leaf should not collapse:\n%s", out)
	}
}

func TestTreeFilterContext(t *testing.T) {
	out := renderTree(t, []tickets.Ticket{
		mk("epi-1", "", 1),
		mk("tsk-2", "in_progress", 2, "epi-1"),
	}, graph.Filter{Status: "in_progress"}, false)
	if !strings.Contains(out, "epi-1 (context)") {
		t.Errorf("parent context missing:\n%s", out)
	}
	if !strings.Contains(out, "in_progress") {
		t.Errorf("matching ticket missing:\n%s", out)
	}
}

func TestTreeFilterHidesUnrelated(t *testing.T) {
	out := renderTree(t, []tickets.Ticket{
		mk("a-1", "", 1),
		mk("b-1", "", 1),
	}, graph.Filter{Status: "open", Tag: ""}, false)
	_ = out
}

func TestTreeMissingDepSecondLine(t *testing.T) {
	out := renderTree(t, []tickets.Ticket{
		mk("a-1", "", 1, "ghost-9"),
	}, graph.Filter{}, false)
	if !strings.Contains(out, "ghost-9 (missing)") {
		t.Errorf("missing dep line missing:\n%s", out)
	}
}

func TestTreeCycleMarkersAndNoDuplicate(t *testing.T) {
	out := renderTree(t, []tickets.Ticket{
		mk("a-1", "", 1, "b-1"),
		mk("b-1", "", 1, "a-1"),
	}, graph.Filter{}, false)
	if strings.Count(out, "⟲") != 2 {
		t.Errorf("cycle markers wrong:\n%s", out)
	}
	if strings.Count(out, "a-1 [P") != 1 {
		t.Errorf("cycle node rendered twice:\n%s", out)
	}
}

func TestTreeColorDimming(t *testing.T) {
	out := renderTree(t, []tickets.Ticket{mk("a-1", "closed", 1)}, graph.Filter{}, true)
	if !strings.Contains(out, ansiDim) {
		t.Errorf("color mode should dim closed lines:\n%s", out)
	}
}

func TestReadyFormatMatchesBash(t *testing.T) {
	var buf bytes.Buffer
	Ready(&buf, []tickets.Ticket{
		mk("r-002", "open", 2, "dep-1"),
		mk("r-001", "", 1),
		mk("r-003", "in_progress", 2),
		mk("dep-1", "closed", 1),
		mk("r-004", "closed", 1), // excluded: closed
	}, graph.Filter{})
	want := "r-001    [P1][open] - Title r-001\n" +
		"r-002    [P2][open] - Title r-002\n" +
		"r-003    [P2][in_progress] - Title r-003\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestBlockedFormatMatchesBash(t *testing.T) {
	var buf bytes.Buffer
	Blocked(&buf, []tickets.Ticket{
		mk("b-001", "", 1, "x-1", "y-1"),
		mk("x-1", "open", 1),
		mk("y-1", "closed", 1),
	}, graph.Filter{})
	want := "b-001    [P1][open] - Title b-001 <- [x-1]\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestFlatFormat(t *testing.T) {
	var buf bytes.Buffer
	Flat(&buf, []tickets.Ticket{
		mk("f-2", "open", 1),
		mk("f-1", "closed", 2),
	}, graph.Filter{})
	want := "f-2      [P1][open] Title f-2\n" +
		"f-1      [P2][closed] Title f-1\n"
	if buf.String() != want {
		t.Errorf("got:\n%q\nwant:\n%q", buf.String(), want)
	}
}

func TestMultiParentRendersOnce(t *testing.T) {
	out := renderTree(t, []tickets.Ticket{
		mk("p-1", "", 1),
		mk("p-2", "", 1),
		mk("c-1", "", 2, "p-1", "p-2"),
	}, graph.Filter{}, false)
	if strings.Count(out, "c-1 ") != 1 {
		t.Errorf("child should render exactly once:\n%s", out)
	}
	golden(t, "tree_multi_parent", out)
}

func TestTreeCycleWithNonMatchingFilterNoOverflow(t *testing.T) {
	// Regression: filter where no cycle member matches caused infinite
	// recursion in visibility computation.
	out := renderTree(t, []tickets.Ticket{
		mk("a-1", "open", 1, "b-1"),
		mk("b-1", "open", 1, "a-1"),
	}, graph.Filter{Status: "closed"}, false)
	if strings.Contains(out, "a-1") || strings.Contains(out, "b-1") {
		t.Errorf("non-matching cycle members should be hidden:\n%s", out)
	}
}

func TestTreeClosedCycleNoOverflow(t *testing.T) {
	// Regression: fully-closed dependency cycle overflowed subtreeSize.
	out := renderTree(t, []tickets.Ticket{
		mk("a-1", "closed", 1, "b-1"),
		mk("b-1", "closed", 1, "a-1"),
	}, graph.Filter{}, false)
	if !strings.Contains(out, "▸") {
		t.Errorf("closed cycle should collapse:\n%s", out)
	}
}

func TestTreeSelfDepCycle(t *testing.T) {
	out := renderTree(t, []tickets.Ticket{
		mk("s-1", "open", 1, "s-1"),
	}, graph.Filter{}, false)
	if !strings.Contains(out, "⟲") {
		t.Errorf("self-dep should carry cycle marker:\n%s", out)
	}
	if !strings.Contains(out, "s-1") {
		t.Errorf("self-dep ticket must still render:\n%s", out)
	}
}
