// Package tickets loads and parses ticket markdown files.
package tickets

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Ticket is a parsed ticket file.
type Ticket struct {
	ID       string
	Status   string
	Title    string
	Priority int // default 2 when unset
	Assignee string
	Tags     []string
	Deps     []string
}

// LoadDir reads every *.md file in dir and returns the parsed tickets,
// ordered by filename (deterministic).
func LoadDir(dir string) ([]Ticket, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read tickets dir: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)

	var out []Ticket
	for _, name := range names {
		t, err := ParseFile(filepath.Join(dir, name))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if t.ID == "" {
			continue // not a ticket file
		}
		out = append(out, t)
	}
	return out, nil
}

// ParseFile parses a single ticket markdown file.
func ParseFile(path string) (Ticket, error) {
	f, err := os.Open(path)
	if err != nil {
		return Ticket{}, err
	}
	defer f.Close()

	var t Ticket
	t.Priority = 2 // bash behavior: missing priority defaults to 2

	sc := bufio.NewScanner(f)
	inFront := false
	sawFront := false
	titleDone := false
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.TrimSpace(line) == "---":
			if !sawFront && !inFront {
				inFront = true
				sawFront = true
				continue
			}
			if inFront {
				inFront = false
				continue
			}
		case inFront:
			parseField(&t, line)
		case !titleDone:
			if strings.HasPrefix(line, "# ") {
				t.Title = strings.TrimSpace(strings.TrimPrefix(line, "# "))
				titleDone = true
			}
		}
	}
	if err := sc.Err(); err != nil {
		return Ticket{}, err
	}
	return t, nil
}

func parseField(t *Ticket, line string) {
	key, val, ok := strings.Cut(line, ":")
	if !ok {
		return
	}
	val = strings.TrimSpace(val)
	// Strip surrounding quotes.
	if len(val) >= 2 && (val[0] == '"' || val[0] == '\'') && val[len(val)-1] == val[0] {
		val = val[1 : len(val)-1]
	}
	switch strings.TrimSpace(key) {
	case "id":
		t.ID = val
	case "status":
		t.Status = val
	case "priority":
		t.Priority = atoiDefault(val, 2)
	case "assignee":
		t.Assignee = val
	case "tags":
		t.Tags = parseList(val)
	case "deps":
		t.Deps = parseList(val)
	}
}

// parseList splits "[a, b]" or "a,b" into ["a", "b"].
func parseList(s string) []string {
	s = strings.TrimPrefix(s, "[")
	s = strings.TrimSuffix(s, "]")
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func atoiDefault(s string, def int) int {
	s = strings.TrimSpace(s)
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}
