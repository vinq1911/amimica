package config

import (
	"errors"
	"fmt"
)

// validNormLevels lists all accepted normalization_level values.
var validNormLevels = map[string]bool{
	"raw":      true,
	"light":    true,
	"strong":   true,
	"semantic": true,
}

// validOutputFormats lists all accepted output format values.
var validOutputFormats = map[string]bool{
	"text":     true,
	"json":     true,
	"sarif":    true,
	"markdown": true,
}

// validSortBy lists all accepted sort_by values.
var validSortBy = map[string]bool{
	"score":   true,
	"lines":   true,
	"regions": true,
	"file":    true,
}

// Validate checks the Config for invalid or inconsistent values.
// It collects all errors and returns them joined. Returns nil if the config is valid.
func Validate(cfg *Config) error {
	var errs []error

	// Analysis
	if cfg.Analysis.MinStatements < 1 {
		errs = append(errs, fmt.Errorf("analysis.min_statements must be >= 1, got %d", cfg.Analysis.MinStatements))
	}
	if cfg.Analysis.MinLines < 1 {
		errs = append(errs, fmt.Errorf("analysis.min_lines must be >= 1, got %d", cfg.Analysis.MinLines))
	}
	if cfg.Analysis.WindowSize < 1 {
		errs = append(errs, fmt.Errorf("analysis.window_size must be >= 1, got %d", cfg.Analysis.WindowSize))
	}
	if !validNormLevels[cfg.Analysis.NormalizationLevel] {
		errs = append(errs, fmt.Errorf("analysis.normalization_level must be one of raw/light/strong/semantic, got %q", cfg.Analysis.NormalizationLevel))
	}
	if cfg.Analysis.ShingleSize < 1 {
		errs = append(errs, fmt.Errorf("analysis.shingle_size must be >= 1, got %d", cfg.Analysis.ShingleSize))
	}
	if cfg.Analysis.MinHashFunctions < 1 {
		errs = append(errs, fmt.Errorf("analysis.minhash_functions must be >= 1, got %d", cfg.Analysis.MinHashFunctions))
	}

	// Scoring
	if cfg.Scoring.MinScore < 0.0 || cfg.Scoring.MinScore > 1.0 {
		errs = append(errs, fmt.Errorf("scoring.min_score must be in [0.0, 1.0], got %f", cfg.Scoring.MinScore))
	}
	if cfg.Scoring.MaxFindings < 0 {
		errs = append(errs, fmt.Errorf("scoring.max_findings must be >= 0 (0 = no limit), got %d", cfg.Scoring.MaxFindings))
	}

	// Validate scoring weights individually
	weights := cfg.Scoring.Weights
	for name, w := range map[string]float64{
		"similarity":      weights.Similarity,
		"impact":          weights.Impact,
		"refactorability": weights.Refactorability,
		"repetition":      weights.Repetition,
		"confidence":      weights.Confidence,
	} {
		if w < 0.0 || w > 1.0 {
			errs = append(errs, fmt.Errorf("scoring.weights.%s must be in [0.0, 1.0], got %f", name, w))
		}
	}

	// Paths
	if len(cfg.Paths.Include) == 0 {
		errs = append(errs, errors.New("paths.include must have at least one pattern"))
	}

	// Output
	if !validOutputFormats[cfg.Output.Format] {
		errs = append(errs, fmt.Errorf("output.format must be one of text/json/sarif/markdown, got %q", cfg.Output.Format))
	}
	if cfg.Output.SortBy != "" && !validSortBy[cfg.Output.SortBy] {
		errs = append(errs, fmt.Errorf("output.sort_by must be one of score/lines/regions/file, got %q", cfg.Output.SortBy))
	}

	// Performance
	if cfg.Performance.MaxFileSize <= 0 {
		errs = append(errs, fmt.Errorf("performance.max_file_size must be > 0, got %d", cfg.Performance.MaxFileSize))
	}
	if cfg.Performance.Jobs < 0 {
		errs = append(errs, fmt.Errorf("performance.jobs must be >= 0, got %d", cfg.Performance.Jobs))
	}
	if cfg.Performance.MaxUnitsPerFile < 1 {
		errs = append(errs, fmt.Errorf("performance.max_units_per_file must be >= 1, got %d", cfg.Performance.MaxUnitsPerFile))
	}

	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
