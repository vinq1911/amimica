package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/user/amimica/internal/config"
)

func TestDefaultReturnsPopulatedConfig(t *testing.T) {
	cfg := config.Default()
	if cfg == nil {
		t.Fatal("Default() returned nil")
	}

	// Analysis defaults
	if cfg.Analysis.MinStatements != 3 {
		t.Errorf("MinStatements: got %d, want 3", cfg.Analysis.MinStatements)
	}
	if cfg.Analysis.MinLines != 6 {
		t.Errorf("MinLines: got %d, want 6", cfg.Analysis.MinLines)
	}
	if cfg.Analysis.WindowSize != 5 {
		t.Errorf("WindowSize: got %d, want 5", cfg.Analysis.WindowSize)
	}
	if cfg.Analysis.NormalizationLevel != "strong" {
		t.Errorf("NormalizationLevel: got %q, want %q", cfg.Analysis.NormalizationLevel, "strong")
	}

	// Scoring defaults
	if cfg.Scoring.MinScore != 0.15 {
		t.Errorf("MinScore: got %f, want 0.15", cfg.Scoring.MinScore)
	}
	if cfg.Scoring.MaxFindings != 0 {
		t.Errorf("MaxFindings: got %d, want 0 (no limit)", cfg.Scoring.MaxFindings)
	}

	// Version
	if cfg.Version != 1 {
		t.Errorf("Version: got %d, want 1", cfg.Version)
	}

	// Paths defaults
	if len(cfg.Paths.Include) == 0 {
		t.Error("Paths.Include should have at least one pattern")
	}
	if len(cfg.Paths.Exclude) == 0 {
		t.Error("Paths.Exclude should have at least one pattern")
	}

	// Performance defaults
	if cfg.Performance.MaxFileSize != 1048576 {
		t.Errorf("MaxFileSize: got %d, want 1048576", cfg.Performance.MaxFileSize)
	}
}

func TestLoadNonExistentFileReturnsDefaults(t *testing.T) {
	cfg, err := config.Load("/nonexistent/path/to/config.yaml")
	if err != nil {
		t.Fatalf("Load non-existent file returned error: %v", err)
	}
	if cfg == nil {
		t.Fatal("Load returned nil config")
	}
	// Should return defaults
	defaults := config.Default()
	if cfg.Analysis.MinStatements != defaults.Analysis.MinStatements {
		t.Errorf("MinStatements: got %d, want %d", cfg.Analysis.MinStatements, defaults.Analysis.MinStatements)
	}
}

func TestLoadFromYAMLFile(t *testing.T) {
	// Write a temp YAML file
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "config.yaml")
	yamlContent := `version: 1
analysis:
  min_statements: 5
  normalization_level: semantic
scoring:
  min_score: 0.30
`
	if err := os.WriteFile(yamlPath, []byte(yamlContent), 0644); err != nil {
		t.Fatalf("Failed to write temp config: %v", err)
	}

	cfg, err := config.Load(yamlPath)
	if err != nil {
		t.Fatalf("Load returned error: %v", err)
	}
	if cfg.Analysis.MinStatements != 5 {
		t.Errorf("MinStatements: got %d, want 5", cfg.Analysis.MinStatements)
	}
	if cfg.Analysis.NormalizationLevel != "semantic" {
		t.Errorf("NormalizationLevel: got %q, want semantic", cfg.Analysis.NormalizationLevel)
	}
	if cfg.Scoring.MinScore != 0.30 {
		t.Errorf("MinScore: got %f, want 0.30", cfg.Scoring.MinScore)
	}
	// Unset fields should still be defaults
	if cfg.Analysis.WindowSize != 5 {
		t.Errorf("WindowSize should be default 5, got %d", cfg.Analysis.WindowSize)
	}
}

func TestValidateRejectsMinStatementsLessThan1(t *testing.T) {
	cfg := config.Default()
	cfg.Analysis.MinStatements = 0
	err := config.Validate(cfg)
	if err == nil {
		t.Error("Validate should reject min_statements < 1")
	}
}

func TestValidateRejectsMinScoreOutOfRange(t *testing.T) {
	cfg := config.Default()
	cfg.Scoring.MinScore = -0.1
	if err := config.Validate(cfg); err == nil {
		t.Error("Validate should reject min_score < 0")
	}

	cfg2 := config.Default()
	cfg2.Scoring.MinScore = 1.1
	if err := config.Validate(cfg2); err == nil {
		t.Error("Validate should reject min_score > 1")
	}
}

func TestValidateRejectsEmptyIncludePatterns(t *testing.T) {
	cfg := config.Default()
	cfg.Paths.Include = []string{}
	err := config.Validate(cfg)
	if err == nil {
		t.Error("Validate should reject empty include patterns")
	}
}

func TestValidateAcceptsDefaultConfig(t *testing.T) {
	cfg := config.Default()
	if err := config.Validate(cfg); err != nil {
		t.Errorf("Validate should accept default config, got error: %v", err)
	}
}

func TestValidateRejectsInvalidNormLevel(t *testing.T) {
	cfg := config.Default()
	cfg.Analysis.NormalizationLevel = "invalid"
	if err := config.Validate(cfg); err == nil {
		t.Error("Validate should reject invalid normalization_level")
	}
}

func TestValidateRejectsInvalidOutputFormat(t *testing.T) {
	cfg := config.Default()
	cfg.Output.Format = "xml"
	if err := config.Validate(cfg); err == nil {
		t.Error("Validate should reject invalid output format")
	}
}

func TestApplyEnvOverridesNormalizationLevel(t *testing.T) {
	t.Setenv("AMIMICA_ANALYSIS_NORMALIZATION_LEVEL", "semantic")
	cfg := config.Default()
	config.ApplyEnv(cfg)
	if cfg.Analysis.NormalizationLevel != "semantic" {
		t.Errorf("ApplyEnv: NormalizationLevel should be 'semantic', got %q", cfg.Analysis.NormalizationLevel)
	}
}

func TestValidateRejectsMinLinesLessThan1(t *testing.T) {
	cfg := config.Default()
	cfg.Analysis.MinLines = 0
	err := config.Validate(cfg)
	if err == nil {
		t.Error("Validate should reject min_lines < 1")
	}
}

func TestValidateRejectsMaxFileSizeZero(t *testing.T) {
	cfg := config.Default()
	cfg.Performance.MaxFileSize = 0
	err := config.Validate(cfg)
	if err == nil {
		t.Error("Validate should reject max_file_size <= 0")
	}
}
