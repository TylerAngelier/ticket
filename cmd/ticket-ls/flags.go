package main

import (
	"fmt"
	"os"
	"strings"
)

type flags struct {
	status   string
	assignee string
	tag      string
	flat     bool
}

func parseFlags(args []string) (f flags, flat bool, rest []string, err error) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--flat":
			flat = true
		case a == "--status=open" || strings.HasPrefix(a, "--status="):
			f.status = strings.TrimPrefix(a, "--status=")
		case a == "-a":
			if i+1 >= len(args) {
				return f, flat, rest, fmt.Errorf("-a requires an argument")
			}
			i++
			f.assignee = args[i]
		case strings.HasPrefix(a, "--assignee="):
			f.assignee = strings.TrimPrefix(a, "--assignee=")
		case a == "-T":
			if i+1 >= len(args) {
				return f, flat, rest, fmt.Errorf("-T requires an argument")
			}
			i++
			f.tag = args[i]
		case strings.HasPrefix(a, "--tag="):
			f.tag = strings.TrimPrefix(a, "--tag=")
		default:
			rest = append(rest, a)
		}
	}
	return f, flat, rest, nil
}

// isTTY reports whether the file is a terminal.
func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
