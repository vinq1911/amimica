// Command amimica is a deterministic, offline code clone detection tool for Go codebases.
// It finds repetitive code patterns — from exact copy-paste clones to structurally
// similar but renamed fragments — and reports them with enough evidence for a human
// to decide whether to refactor.
//
// Usage:
//
//	amimica <command> [flags]
//
// Commands:
//
//	scan          Run clone detection analysis
//	report        Re-format a previous scan's results
//	explain       Show detailed explanation of a specific finding
//	diff          Show diff between two code regions
//	serve-mcp     Start MCP server for editor/agent integration
//	version       Print version and build info
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/user/amimica/internal/app"
)

// Build-time variables injected via ldflags. See Makefile.
var (
	version   = "0.1.0-dev"
	commit    = "unknown"
	buildDate = "unknown"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: amimica <command> [flags]\n\n")
		fmt.Fprintf(os.Stderr, "Commands:\n")
		fmt.Fprintf(os.Stderr, "  scan          Run clone detection analysis\n")
		fmt.Fprintf(os.Stderr, "  report        Re-format a previous scan's results\n")
		fmt.Fprintf(os.Stderr, "  explain       Show detailed explanation of a specific finding\n")
		fmt.Fprintf(os.Stderr, "  diff          Show diff between two code regions\n")
		fmt.Fprintf(os.Stderr, "  serve-mcp     Start MCP server for editor/agent integration\n")
		fmt.Fprintf(os.Stderr, "  version       Print version and build info\n")
	}

	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	switch args[0] {
	case "version":
		fmt.Printf("amimica version %s (commit %s, built %s)\n", version, commit, buildDate)
	case "scan":
		os.Exit(app.RunScan(args[1:]))
	case "report", "explain", "diff", "serve-mcp":
		fmt.Fprintf(os.Stderr, "amimica: command %q not implemented yet\n", args[0])
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "amimica: unknown command %q\n", args[0])
		flag.Usage()
		os.Exit(2)
	}
}
