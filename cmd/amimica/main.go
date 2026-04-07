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

	"github.com/user/amimica/internal/logging"
)

// version is the current build version. It can be overridden at link time via:
//
//	go build -ldflags "-X main.version=1.2.3" ./cmd/amimica
var version = "0.1.0-dev"

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

	log := logging.Setup("info", "text")
	_ = log

	switch args[0] {
	case "version":
		fmt.Printf("amimica version %s\n", version)
	case "scan", "report", "explain", "diff", "serve-mcp":
		fmt.Fprintf(os.Stderr, "amimica: command %q not implemented\n", args[0])
		os.Exit(1)
	default:
		fmt.Fprintf(os.Stderr, "amimica: unknown command %q\n", args[0])
		flag.Usage()
		os.Exit(2)
	}
}
