// Package engine orchestrates the clone detection pipeline:
// discovery -> parsing/normalization/extraction -> fingerprinting -> matching -> scoring.
// Both the CLI and MCP server call engine.Analyze() as their primary entrypoint.
package engine

import (
	"fmt"
	"log/slog"
	"time"

	"github.com/vinq1911/amimica/internal/config"
	"github.com/vinq1911/amimica/internal/discovery"
	"github.com/vinq1911/amimica/internal/lang"
	"github.com/vinq1911/amimica/internal/lang/csharp"
	golanguage "github.com/vinq1911/amimica/internal/lang/golang"
	"github.com/vinq1911/amimica/internal/lang/javascript"
	"github.com/vinq1911/amimica/internal/lang/objc"
	"github.com/vinq1911/amimica/internal/lang/ruby"
	"github.com/vinq1911/amimica/internal/match"
	"github.com/vinq1911/amimica/internal/model"
	"github.com/vinq1911/amimica/internal/report"
	"github.com/vinq1911/amimica/internal/score"
)

// DefaultRegistry returns a registry with all built-in languages.
func DefaultRegistry() *lang.Registry {
	r := lang.NewRegistry()
	r.Register(golanguage.New())
	r.Register(javascript.New())
	r.Register(ruby.New())
	r.Register(objc.New())
	r.Register(csharp.New())
	return r
}

// Analyze runs the full clone detection pipeline on the given roots.
func Analyze(roots []string, cfg *config.Config, log *slog.Logger) (*report.Result, error) {
	return AnalyzeWith(roots, cfg, DefaultRegistry(), log)
}

// AnalyzeWith runs analysis with a specific language registry.
func AnalyzeWith(roots []string, cfg *config.Config, registry *lang.Registry, log *slog.Logger) (*report.Result, error) {
	start := time.Now()

	// 1. Discovery.
	log.Info("discovering files", "roots", roots)
	files, err := discovery.Walk(roots, cfg, registry, log)
	if err != nil {
		return nil, fmt.Errorf("engine: discovery: %w", err)
	}

	// Log language breakdown.
	langCounts := make(map[string]int)
	for _, f := range files {
		langCounts[f.Language]++
	}
	log.Info("files discovered", "count", len(files), "languages", langCounts)

	if len(files) == 0 {
		return &report.Result{
			Version:      "0.1.0-dev",
			Timestamp:    time.Now(),
			FilesScanned: 0,
			Duration:     time.Since(start),
		}, nil
	}

	// 2. Parse + Normalize + Extract per language.
	normLevel := parseNormLevel(cfg.Analysis.NormalizationLevel)
	log.Info("extracting units", "norm_level", normLevel)

	var allUnits []model.NormalizedUnit
	funcCount := 0
	parseErrors := 0

	for i := range files {
		sf := files[i]
		language := registry.ForFile(sf.Path)
		if language == nil {
			continue
		}

		units, err := language.ParseAndExtract(sf, cfg, normLevel, log)
		if err != nil {
			log.Debug("parse error", "path", sf.RelPath, "lang", sf.Language, "error", err)
			parseErrors++
			continue
		}

		allUnits = append(allUnits, units...)
		for _, u := range units {
			if u.Kind == model.UnitFunction {
				funcCount++
			}
		}
	}

	log.Info("units extracted", "total", len(allUnits), "functions", funcCount, "parse_errors", parseErrors)

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

	// 3. Matching (language-agnostic from here).
	log.Info("matching clones")
	classes := match.FindClones(allUnits, cfg, log)
	log.Info("clone classes found", "count", len(classes))

	// 4. Scoring.
	log.Info("scoring findings")
	findings := score.ScoreFindings(classes, allUnits, files, cfg)
	log.Info("findings scored", "total", len(findings))

	result := &report.Result{
		Version:           "0.1.0-dev",
		ScanRoot:          roots[0],
		Timestamp:         time.Now(),
		FilesScanned:      len(files),
		FuncsAnalyzed:     funcCount,
		UnitsAnalyzed:     len(allUnits),
		CloneClassesTotal: len(classes),
		Duration:          time.Since(start),
		Findings:          findings,
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
