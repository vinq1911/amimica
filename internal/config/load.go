package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Load reads a Config from the YAML file at path and merges it over Default().
//
// If path is empty, Load searches the following locations in order:
//  1. .amimica.yaml in the current directory
//  2. ~/.config/amimica/config.yaml
//
// If no file is found at the given or discovered path, Load returns Default()
// with a nil error. Parse errors are returned as errors.
func Load(path string) (*Config, error) {
	cfg := Default()

	effectivePath := path
	if effectivePath == "" {
		effectivePath = discoverConfigFile()
	}
	if effectivePath == "" {
		return cfg, nil // No config file found; use defaults
	}

	data, err := os.ReadFile(effectivePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil // File not found; use defaults
		}
		return nil, err
	}

	// Decode into the default config so that unset YAML keys retain their defaults.
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}

	return cfg, nil
}

// discoverConfigFile searches default locations for a config file.
// Returns the first location that exists, or empty string if none found.
func discoverConfigFile() string {
	candidates := []string{
		".amimica.yaml",
	}

	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".config", "amimica", "config.yaml"))
	}

	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

// ApplyEnv applies AMIMICA_* environment variable overrides to the config.
//
// The pattern is AMIMICA_<SECTION>_<KEY> with all uppercase and underscores.
// Examples:
//
//	AMIMICA_ANALYSIS_NORMALIZATION_LEVEL=semantic
//	AMIMICA_SCORING_MIN_SCORE=0.5
//	AMIMICA_PERFORMANCE_JOBS=4
//	AMIMICA_LOGGING_LEVEL=debug
//	AMIMICA_LOGGING_FORMAT=json
func ApplyEnv(cfg *Config) {
	if v := os.Getenv("AMIMICA_ANALYSIS_NORMALIZATION_LEVEL"); v != "" {
		cfg.Analysis.NormalizationLevel = v
	}
	if v := os.Getenv("AMIMICA_ANALYSIS_THOROUGH"); v != "" {
		cfg.Analysis.Thorough = v == "true" || v == "1" || v == "yes"
	}
	if v := os.Getenv("AMIMICA_SCORING_MIN_SCORE"); v != "" {
		var f float64
		if _, err := fmt.Sscanf(v, "%f", &f); err == nil {
			cfg.Scoring.MinScore = f
		}
	}
	if v := os.Getenv("AMIMICA_SCORING_MAX_FINDINGS"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			cfg.Scoring.MaxFindings = n
		}
	}
	if v := os.Getenv("AMIMICA_PERFORMANCE_JOBS"); v != "" {
		var n int
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			cfg.Performance.Jobs = n
		}
	}
	if v := os.Getenv("AMIMICA_PERFORMANCE_MAX_FILE_SIZE"); v != "" {
		var n int64
		if _, err := fmt.Sscanf(v, "%d", &n); err == nil {
			cfg.Performance.MaxFileSize = n
		}
	}
	if v := os.Getenv("AMIMICA_LOGGING_LEVEL"); v != "" {
		cfg.Logging.Level = v
	}
	if v := os.Getenv("AMIMICA_LOGGING_FORMAT"); v != "" {
		cfg.Logging.Format = v
	}
	if v := os.Getenv("AMIMICA_OUTPUT_FORMAT"); v != "" {
		cfg.Output.Format = v
	}
	if v := os.Getenv("AMIMICA_CACHE_ENABLED"); v != "" {
		cfg.Cache.Enabled = v == "true" || v == "1" || v == "yes"
	}
	if v := os.Getenv("AMIMICA_CACHE_DIR"); v != "" {
		cfg.Cache.Dir = v
	}
	if v := os.Getenv("AMIMICA_PATHS_FOLLOW_SYMLINKS"); v != "" {
		cfg.Paths.FollowSymlinks = v == "true" || v == "1" || v == "yes"
	}
	if v := os.Getenv("AMIMICA_PATHS_INCLUDE_TESTS"); v != "" {
		cfg.Paths.IncludeTests = v == "true" || v == "1" || v == "yes"
	}
	if v := os.Getenv("AMIMICA_PATHS_INCLUDE_VENDOR"); v != "" {
		cfg.Paths.IncludeVendor = v == "true" || v == "1" || v == "yes"
	}
	if v := os.Getenv("AMIMICA_PATHS_INCLUDE_GENERATED"); v != "" {
		cfg.Paths.IncludeGenerated = v == "true" || v == "1" || v == "yes"
	}
}
