// Package app implements the CLI command handlers. Each command is a thin
// wrapper that parses flags, loads configuration, and calls into the engine.
package app

import (
	"flag"
	"fmt"
	"log/slog"
	"os"

	"github.com/user/amimica/internal/config"
	"github.com/user/amimica/internal/engine"
	"github.com/user/amimica/internal/logging"
	"github.com/user/amimica/internal/report"
)

// RunScan implements the "scan" subcommand.
func RunScan(args []string) int {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)

	configPath := fs.String("config", "", "Config file path")
	outputFmt := fs.String("output", "", "Output format: text, json (default: text)")
	outputFile := fs.String("output-file", "", "Write output to file")
	minScore := fs.Float64("min-score", -1, "Minimum composite score")
	minLines := fs.Int("min-lines", -1, "Minimum lines per region")
	minStmts := fs.Int("min-statements", -1, "Minimum statements per unit")
	normLevel := fs.String("norm-level", "", "Normalization level: raw, light, strong, semantic")
	excludeTests := fs.Bool("exclude-tests", false, "Exclude test files entirely")
	thorough := fs.Bool("thorough", false, "Enable thorough scanning")
	maxFindings := fs.Int("n", 0, "Limit output to N findings (0 = no limit)")
	debug := fs.Bool("debug", false, "Enable debug logging")
	noCache := fs.Bool("no-cache", false, "Disable caching")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "amimica scan: %v\n", err)
		return 2
	}

	// Load config.
	cfg, err := loadConfig(*configPath)
	if err != nil {
		return exitError(err, 2)
	}

	// Apply flag overrides.
	if *outputFmt != "" {
		cfg.Output.Format = *outputFmt
	}
	if *outputFile != "" {
		cfg.Output.File = *outputFile
	}
	if *minScore >= 0 {
		cfg.Scoring.MinScore = *minScore
	}
	if *minLines >= 0 {
		cfg.Analysis.MinLines = *minLines
	}
	if *minStmts >= 0 {
		cfg.Analysis.MinStatements = *minStmts
	}
	if *normLevel != "" {
		cfg.Analysis.NormalizationLevel = *normLevel
	}
	if *excludeTests {
		cfg.Paths.IncludeTests = false
	}
	if *thorough {
		cfg.Analysis.Thorough = true
	}
	if *maxFindings > 0 {
		cfg.Scoring.MaxFindings = *maxFindings
	}
	if *noCache {
		cfg.Cache.Enabled = false
	}
	if *debug {
		cfg.Logging.Level = "debug"
	}

	if err := config.Validate(cfg); err != nil {
		return exitError(fmt.Errorf("invalid config: %w", err), 2)
	}

	log := logging.Setup(cfg.Logging.Level, cfg.Logging.Format)

	// Determine scan roots.
	roots := fs.Args()
	if len(roots) == 0 {
		roots = []string{"."}
	}

	// Run analysis.
	result, err := engine.Analyze(roots, cfg, log)
	if err != nil {
		log.Error("analysis failed", "error", err)
		fmt.Fprintf(os.Stderr, "amimica: analysis error: %v\n", err)
		return 3
	}

	// Write output.
	var w *os.File
	if cfg.Output.File != "" {
		w, err = os.Create(cfg.Output.File)
		if err != nil {
			fmt.Fprintf(os.Stderr, "amimica: cannot create output file: %v\n", err)
			return 3
		}
		defer w.Close()
	} else {
		w = os.Stdout
	}

	switch cfg.Output.Format {
	case "json":
		if err := report.WriteJSON(w, result); err != nil {
			log.Error("write JSON failed", slog.String("error", err.Error()))
			return 3
		}
	default:
		if err := report.WriteText(w, result); err != nil {
			log.Error("write text failed", slog.String("error", err.Error()))
			return 3
		}
	}

	// Exit code: 1 if findings above threshold, 0 otherwise.
	for _, f := range result.Findings {
		if !f.Suppressed {
			return 1
		}
	}
	return 0
}
