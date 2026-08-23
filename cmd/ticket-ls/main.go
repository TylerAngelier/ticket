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
	"os/exec"
	"path/filepath"
	"strings"

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
// can be installed as ticket-ls, ticket-list, ticket-ready, ticket-blocked,
// ticket-dep, ticket-closed, and ticket-show.
func defaultCmd() string {
	base := filepath.Base(os.Args[0])
	switch base {
	case "ticket-ready", "tk-ready":
		return "ready"
	case "ticket-blocked", "tk-blocked":
		return "blocked"
	case "ticket-list", "tk-list":
		return "list"
	case "ticket-closed", "tk-closed":
		return "closed"
	case "ticket-show", "tk-show":
		return "show"
	case "ticket-dep", "tk-dep":
		return "dep"
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
		case "ready", "blocked", "ls", "list", "closed", "show", "dep", "tree", "cycle":
			if args[0] == "tree" || args[0] == "cycle" {
				// Keep args intact: runDep needs the subcommand.
				cmd = "dep"
				break
			}
			if cmd == "dep" && args[0] != "dep" {
				break // invoked as ticket-dep <tree|cycle>: keep args
			}
			cmd = args[0]
			args = args[1:]
		case "-h", "--help", "help":
			fmt.Print(usage)
			return 0
		}
	}

	if cmd == "show" {
		dir := ticketsDir()
		all, rc := loadTickets(dir)
		if rc != 0 {
			return rc
		}
		return runShow(all, dir, args)
	}

	if cmd == "dep" {
		// Subcommand stays in args; writes are delegated inside runDep.
		dir := ticketsDir()
		all, rc := loadTickets(dir)
		if rc != 0 {
			return rc
		}
		return runDep(all, args)
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

	dir := ticketsDir()
	all, rc := loadTickets(dir)
	if rc != 0 {
		return rc
	}

	if f.limit == 0 {
		f.limit = defaultClosedLimit
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
	case "closed":
		render.Closed(os.Stdout, all, filt, f.limit)
	}
	return 0
}

// runShow implements the show command (positional ID, no flags).
func runShow(all []tickets.Ticket, dir string, args []string) int {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ticket show <id>")
		return 1
	}
	g := graph.Build(all)
	target, err := g.ResolveID(args[0])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := render.Show(os.Stdout, dir, target, g); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

// runDep implements `dep tree`, `dep cycle`; anything else (the add path,
// a write) is delegated to the bash built-in via $TK_SCRIPT super dep.
func runDep(all []tickets.Ticket, args []string) int {
	g := graph.Build(all)
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Usage: ticket dep <id> <dependency-id>")
		fmt.Fprintln(os.Stderr, "       ticket dep tree [--full] <id>  - show dependency tree")
		fmt.Fprintln(os.Stderr, "       ticket dep cycle               - find dependency cycles")
		return 1
	}
	switch args[0] {
	case "tree":
		rest := args[1:]
		full := false
		if len(rest) > 0 && rest[0] == "--full" {
			full = true
			rest = rest[1:]
		}
		if len(rest) != 1 {
			fmt.Fprintln(os.Stderr, "Usage: ticket dep tree [--full] <id>")
			return 1
		}
		root, err := g.ResolveID(rest[0])
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		for _, line := range g.DepTree(root, full) {
			fmt.Println(line)
		}
		return 0
	case "cycle":
		cycles := g.OpenCycles()
		if len(cycles) == 0 {
			fmt.Println("No dependency cycles found")
			return 0
		}
		for i, c := range cycles {
			if i > 0 {
				fmt.Println()
			}
			fmt.Printf("Cycle %d: %s\n", i+1, strings.Join(c.Path, " -> "))
			for _, m := range c.Members {
				t := g.Tickets[m]
				status := t.Status
				if status == "" {
					status = "open"
				}
				fmt.Printf("  %-8s [%s] %s\n", m, status, t.Title)
			}
		}
		return 0
	default:
		// Write operation: delegate to the bash built-in.
		tkScript := os.Getenv("TK_SCRIPT")
		if tkScript == "" {
			fmt.Fprintf(os.Stderr, "Error: TK_SCRIPT not set; cannot delegate 'dep %s'\n", args[0])
			return 1
		}
		cmd := exec.Command(tkScript, append([]string{"super", "dep"}, args...)...)
		cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err := cmd.Run(); err != nil {
			if ee, ok := err.(*exec.ExitError); ok {
				return ee.ExitCode()
			}
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
		return 0
	}
}

// ticketsDir resolves the tickets directory per the plugin contract.
func ticketsDir() string {
	if d := os.Getenv("TICKETS_DIR"); d != "" {
		return d
	}
	return findTicketsDir()
}

func loadTickets(dir string) ([]tickets.Ticket, int) {
	if dir == "" {
		fmt.Fprintln(os.Stderr, "Error: no .tickets directory found (searched parent directories)")
		fmt.Fprintln(os.Stderr, "Run 'tk create' to initialize, or set TICKETS_DIR env var")
		return nil, 1
	}
	all, err := tickets.LoadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		return nil, 1
	}
	return all, 0
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
