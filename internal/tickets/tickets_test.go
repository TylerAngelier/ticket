package tickets

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const sample = `---
id: abc-1234
status: in_progress
priority: 1
assignee: "tyler: ops"
tags: [backend, api]
deps: [def-5678, ghi-9012]
created: 2025-01-01T00:00:00Z
---
# Fix the login flow

Some body text with # not a title.
`

func TestParseFile(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "abc-1234.md", sample)

	ts, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 {
		t.Fatalf("want 1 ticket, got %d", len(ts))
	}
	got := ts[0]
	if got.ID != "abc-1234" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.Status != "in_progress" {
		t.Errorf("Status = %q", got.Status)
	}
	if got.Title != "Fix the login flow" {
		t.Errorf("Title = %q", got.Title)
	}
	if got.Assignee != `tyler: ops` {
		t.Errorf("Assignee = %q (quoted colon value must survive)", got.Assignee)
	}
	if len(got.Tags) != 2 || got.Tags[0] != "backend" || got.Tags[1] != "api" {
		t.Errorf("Tags = %v", got.Tags)
	}
	if len(got.Deps) != 2 || got.Deps[0] != "def-5678" {
		t.Errorf("Deps = %v", got.Deps)
	}
}

func TestParseDefaults(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "x-0001.md", "---\nid: x-0001\n---\n# Bare\n")
	ts, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if ts[0].Priority != 2 {
		t.Errorf("default Priority = %d, want 2", ts[0].Priority)
	}
	if ts[0].Status != "" {
		t.Errorf("Status = %q, want empty", ts[0].Status)
	}
}

func TestLoadDirSkipsNonTicketsAndDirs(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "a-0001.md", "---\nid: a-0001\n---\n# A\n")
	write(t, dir, "notes.txt", "id: nope\n")
	write(t, dir, "b-0002.md", "# no frontmatter id\n")
	if err := os.MkdirAll(filepath.Join(dir, "sub.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	ts, err := LoadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ts) != 1 || ts[0].ID != "a-0001" {
		t.Errorf("got %+v", ts)
	}
}

func TestLoadDirSortedByName(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{"c-0003", "a-0001", "b-0002"} {
		write(t, dir, n+".md", "---\nid: "+n+"\n---\n# T\n")
	}
	ts, _ := LoadDir(dir)
	want := []string{"a-0001", "b-0002", "c-0003"}
	for i, w := range want {
		if ts[i].ID != w {
			t.Fatalf("order: got %v", ts)
		}
	}
}

func TestMissingDirErrors(t *testing.T) {
	if _, err := LoadDir(filepath.Join(t.TempDir(), "nope")); err == nil {
		t.Fatal("want error for missing dir")
	}
}
