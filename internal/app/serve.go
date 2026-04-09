package app

import (
	"flag"
	"fmt"
	"os"

	"github.com/user/amimica/internal/logging"
	"github.com/user/amimica/internal/mcp"
)

// RunServeMCP implements the "serve-mcp" subcommand.
func RunServeMCP(args []string) int {
	fs := flag.NewFlagSet("serve-mcp", flag.ContinueOnError)
	configPath := fs.String("config", "", "Config file path")
	logFile := fs.String("log-file", "", "Log to file (default: stderr)")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "amimica serve-mcp: %v\n", err)
		return 2
	}

	cfg, err := loadConfig(*configPath)
	if err != nil {
		return exitError(err, 2)
	}

	// MCP uses stdio for JSON-RPC, so logs must go to a file or be suppressed.
	if *logFile == "" {
		cfg.Logging.Level = "error"
	}
	log := logging.Setup(cfg.Logging.Level, cfg.Logging.Format)

	server := mcp.NewServer(cfg, log)
	if err := server.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "amimica: MCP server error: %v\n", err)
		return 3
	}
	return 0
}
