package app

import (
	"flag"
	"fmt"
	"os"

	"github.com/user/amimica/internal/config"
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

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "amimica: config error: %v\n", err)
		return 2
	}
	config.ApplyEnv(cfg)

	// MCP uses stdio for JSON-RPC, so logs must go to a file or be suppressed.
	if *logFile == "" {
		cfg.Logging.Level = "error" // suppress info logs on stderr during MCP
	}
	log := logging.Setup(cfg.Logging.Level, cfg.Logging.Format)

	if *logFile != "" {
		// Redirect slog to file — but for now just use stderr with reduced level.
		log = logging.Setup(cfg.Logging.Level, cfg.Logging.Format)
	}

	server := mcp.NewServer(cfg, log)
	if err := server.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "amimica: MCP server error: %v\n", err)
		return 3
	}
	return 0
}
