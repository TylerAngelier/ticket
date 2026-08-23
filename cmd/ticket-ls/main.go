// ticket-ls is a compiled tk plugin providing hierarchical listing and
// graph views: list (tree), ready, blocked.
//
// Dispatched by tk as plugins named ticket-ls / ticket-list / ticket-ready /
// ticket-blocked. Receives TICKETS_DIR in the environment per the plugin
// contract.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/TylerAngelier/ticket/internal/graph"
	"github.com/TylerAngelier/ticket/internal/render"
	"github.com/TylerAngelier/ticket/internal/tickets"
)

// version is reported via --tk-describe and used by release tooling.
const version = "1.0.0"

const usage = `Usage: ticket-ls [command] [options]

Commands:
  (default) | list | ls   Hierarchical tree of all tickets
  ready                   Tickets with all dependencies closed
  blocked                 Tickets with unclosed dependencies

Options:
  -a, --assignee=<id>     Filter by assignee
  -T, --tag=<tag>         Filter by tag
      --status=<status>   Filter by status (list only)
      --flat              One line per ticket, no tree structure
  -h, --help              Show this help
`

func main() {
	// Plugin metadata contract: binaries answer --tk-describe without
	// touching ticket storage.
	if len(os.Args) > 1 && os.Args[1] == "--tk-describe" {
		desc := map[string]string{
			"ticket-ready":   "List tickets whose dependencies are all closed",
			"ticket-blocked": "List tickets blocked by unclosed dependencies",
			"tk-ready":       "List tickets whose dependencies are all closed",
			"tk-blocked":     "List tickets blocked by unclosed dependencies",
		}[filepath.Base(os.Args[0])]
		if desc == "" {
			desc = "List tickets as a hierarchy tree (epics -> stories -> tasks)"
		}
		fmt.Printf("tk-plugin: %s\n", desc)
		fmt.Printf("tk-plugin-version: %s\n", version)
		return
	}
	os.Exit(run(os.Args[1:]))
}

// defaultCmd derives the command from the invoked name so the same binary
// can be installed as ticket-ls, ticket-list, ticket-ready, ticket-blocked.
func defaultCmd() string {
	base := filepath.Base(os.Args[0])
	switch base {
	case "ticket-ready", "tk-ready":
		return "ready"
	case "ticket-blocked", "tk-blocked":
		return "blocked"
	case "ticket-list", "tk-list":
		return "list"
	}
	return ""
}

func run(args []string) int {
	cmd := defaultCmd()
	if cmd == "" {
		cmd = "list"
	}
	if len(args) > 0 {
		switch args[0] {
		case "ready", "blocked", "ls", "list":
			cmd = args[0]
			args = args[1:]
		case "-h", "--help", "help":
			fmt.Print(usage)
			return 0
		}
	}

	f, flat, rest, err := parseFlags(args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ticket-ls: %v\n%s", err, usage)
		return 2
	}
	if len(rest) > 0 {
		fmt.Fprintf(os.Stderr, "ticket-ls: unexpected argument %q\n%s", rest[0], usage)
		return 2
	}
	if cmd != "list" && f.status != "" {
		fmt.Fprintln(os.Stderr, "ticket-ls: --status is only supported for list")
		return 2
	}

	dir := os.Getenv("TICKETS_DIR")
	if dir == "" {
		// Plugin contract: plugins handle their own TICKETS_DIR discovery.
		dir = findTicketsDir()
		if dir == "" {
			fmt.Fprintln(os.Stderr, "Error: no .tickets directory found (searched parent directories)")
			fmt.Fprintln(os.Stderr, "Run 'tk create' to initialize, or set TICKETS_DIR env var")
			return 1
		}
	}
	all, err := tickets.LoadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return 1
	}

	filt := graph.Filter{Status: f.status, Assignee: f.assignee, Tag: f.tag}

	switch cmd {
	case "ls", "list":
		if flat {
			render.Flat(os.Stdout, all, filt)
			return 0
		}
		g := graph.Build(all)
		warnCycles(g.Cycles)
		color := isTTY(os.Stdout)
		render.Tree(os.Stdout, g, filt, render.TreeOptions{Color: color})
	case "ready":
		render.Ready(os.Stdout, all, filt)
	case "blocked":
		render.Blocked(os.Stdout, all, filt)
	}
	return 0
}

// findTicketsDir walks parent directories looking for .tickets,
// mirroring the bash implementation.
func findTicketsDir() string {
	dir, err := os.Getwd()
	if err != nil {
		return ""
	}
	for {
		candidate := filepath.Join(dir, ".tickets")
		if fi, statErr := os.Stat(candidate); statErr == nil && fi.IsDir() {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return ""
}

func warnCycles(cycles [][]string) {
	for _, cyc := range cycles {
		fmt.Fprintf(os.Stderr, "Warning: dependency cycle detected: %s\n", joinPath(cyc))
	}
}

func joinPath(cyc []string) string {
	out := ""
	for i, id := range cyc {
		if i > 0 {
			out += " -> "
		}
		out += id
	}
	return out
}
