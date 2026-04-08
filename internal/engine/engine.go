// Package engine orchestrates the clone detection pipeline:
// discovery → parsing → normalization → extraction → fingerprinting → matching → scoring.
// Both the CLI and MCP server call engine.Analyze() as their primary entrypoint.
package engine

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/user/amimica/internal/config"
	"github.com/user/amimica/internal/discovery"
	"github.com/user/amimica/internal/extract"
	"github.com/user/amimica/internal/match"
	"github.com/user/amimica/internal/model"
	"github.com/user/amimica/internal/parser"
	"github.com/user/amimica/internal/report"
	"github.com/user/amimica/internal/score"
)

// Analyze runs the full clone detection pipeline on the given roots.
func Analyze(roots []string, cfg *config.Config, log *slog.Logger) (*report.Result, error) {
	start := time.Now()

	// 1. Discovery: find Go files.
	log.Info("discovering files", "roots", roots)
	files, err := discovery.Walk(roots, cfg, log)
	if err != nil {
		return nil, fmt.Errorf("engine: discovery: %w", err)
	}
	log.Info("files discovered", "count", len(files))

	if len(files) == 0 {
		return &report.Result{
			Version:      "0.1.0-dev",
			Timestamp:    time.Now(),
			FilesScanned: 0,
			Duration:     time.Since(start),
		}, nil
	}

	// 2. Parsing: parse all files.
	log.Info("parsing files")
	parsed := parser.ParseFiles(files, log)
	log.Info("files parsed", "count", len(parsed))

	// 3. Normalization + Extraction: extract units at the configured level.
	normLevel := parseNormLevel(cfg.Analysis.NormalizationLevel)
	log.Info("extracting units", "norm_level", normLevel)

	var allUnits []model.NormalizedUnit
	funcCount := 0

	for _, pf := range parsed {
		units := extract.Extract(pf, cfg, normLevel)
		allUnits = append(allUnits, units...)
		// Count functions for reporting.
		for _, u := range units {
			if u.Kind == model.UnitFunction {
				funcCount++
			}
		}
	}

	log.Info("units extracted", "total", len(allUnits), "functions", funcCount)

	if len(allUnits) == 0 {
		return &report.Result{
			Version:       "0.1.0-dev",
			ScanRoot:      roots[0],
			Timestamp:     time.Now(),
			FilesScanned:  len(files),
			FuncsAnalyzed: 0,
			UnitsAnalyzed: 0,
			Duration:      time.Since(start),
		}, nil
	}

	// 4. Matching: find clone classes.
	log.Info("matching clones")
	classes := match.FindClones(allUnits, cfg, log)
	log.Info("clone classes found", "count", len(classes))

	// 5. Scoring: score and rank findings.
	log.Info("scoring findings")
	findings := score.ScoreFindings(classes, allUnits, files, cfg)
	log.Info("findings scored", "total", len(findings))

	result := &report.Result{
		Version:       "0.1.0-dev",
		ScanRoot:      roots[0],
		Timestamp:     time.Now(),
		FilesScanned:  len(files),
		FuncsAnalyzed: funcCount,
		UnitsAnalyzed: len(allUnits),
		Duration:      time.Since(start),
		Findings:      findings,
	}

	return result, nil
}

func parseNormLevel(s string) model.NormalizationLevel {
	switch s {
	case "raw":
		return model.NormRaw
	case "light":
		return model.NormLight
	case "strong":
		return model.NormStrong
	case "semantic":
		return model.NormSemantic
	default:
		return model.NormStrong
	}
}
