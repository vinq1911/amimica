// Package config handles loading, defaulting, environment override, and validation
// of the Amimica configuration file (.amimica.yaml or ~/.config/amimica/config.yaml).
//
// Usage:
//
//	cfg, err := config.Load("")    // auto-discover config file
//	if err != nil { ... }
//	config.ApplyEnv(cfg)           // apply AMIMICA_* env overrides
//	if err := config.Validate(cfg); err != nil { ... }
package config

// Config is the root configuration structure for Amimica.
// It is loaded from YAML and can be overridden via environment variables.
// All fields have sensible defaults returned by Default().
type Config struct {
	// Version is the configuration file format version. Currently must be 1.
	Version int `yaml:"version"`

	// Analysis controls the clone detection algorithms and their parameters.
	Analysis AnalysisConfig `yaml:"analysis"`

	// Scoring controls scoring weights, thresholds, and noise suppression.
	Scoring ScoringConfig `yaml:"scoring"`

	// Paths controls file discovery: include/exclude patterns and traversal options.
	Paths PathsConfig `yaml:"paths"`

	// Cache controls the content-hash cache for incremental scans.
	Cache CacheConfig `yaml:"cache"`

	// Output controls the report format and output destination.
	Output OutputConfig `yaml:"output"`

	// Performance controls parallelism and resource limits.
	Performance PerformanceConfig `yaml:"performance"`

	// Logging controls the log level and format.
	Logging LoggingConfig `yaml:"logging"`
}

// AnalysisConfig groups all parameters controlling clone detection algorithms.
type AnalysisConfig struct {
	// NormalizationLevel is the maximum normalization level to apply.
	// Valid values: "raw", "light", "strong", "semantic". Default: "strong".
	NormalizationLevel string `yaml:"normalization_level"`

	// MinStatements is the minimum number of statements a unit must have to be analyzed.
	// Units smaller than this are skipped as noise. Default: 3.
	MinStatements int `yaml:"min_statements"`

	// MinLines is the minimum number of source lines a region must span.
	// Regions shorter than this are not reported. Default: 6.
	MinLines int `yaml:"min_lines"`

	// WindowSize is the number of consecutive statements in a sliding window unit.
	// Default: 5.
	WindowSize int `yaml:"window_size"`

	// WindowMinFunctionSize is the minimum function size (in statements) required
	// before sliding window extraction is applied. Default: 8.
	WindowMinFunctionSize int `yaml:"window_min_function_size"`

	// Thorough enables subtree extraction and rolling hash for deeper analysis.
	// Slower but more thorough. Default: false.
	Thorough bool `yaml:"thorough"`

	// ShingleSize is the n-gram size for token shingle computation. Default: 7.
	ShingleSize int `yaml:"shingle_size"`

	// MinHashFunctions is the number of hash functions for MinHash signatures.
	// More functions reduce approximation error but increase memory usage. Default: 128.
	MinHashFunctions int `yaml:"minhash_functions"`

	// LSHBands is the number of bands for Locality-Sensitive Hashing. Default: 16.
	LSHBands int `yaml:"lsh_bands"`

	// LSHRows is the number of rows per band in LSH. Default: 8.
	LSHRows int `yaml:"lsh_rows"`
}

// ScoringConfig groups all parameters controlling finding scores and ranking.
type ScoringConfig struct {
	// MinScore is the minimum composite score for a finding to be reported.
	// Range: 0.0-1.0. Default: 0.15.
	MinScore float64 `yaml:"min_score"`

	// MaxFindings caps the total number of findings returned. 0 = no limit. Default: 0.
	MaxFindings int `yaml:"max_findings"`

	// Weights controls the relative importance of each scoring dimension.
	Weights ScoringWeights `yaml:"weights"`

	// Penalties are multiplicative factors that reduce scores for less actionable findings.
	Penalties ScoringPenalties `yaml:"penalties"`
}

// ScoringWeights defines the weighted components of the composite score.
// All weights should sum to approximately 1.0.
type ScoringWeights struct {
	Similarity      float64 `yaml:"similarity"`
	Impact          float64 `yaml:"impact"`
	Refactorability float64 `yaml:"refactorability"`
	Repetition      float64 `yaml:"repetition"`
	Confidence      float64 `yaml:"confidence"`
}

// ScoringPenalties defines multiplicative penalty factors for specific conditions.
type ScoringPenalties struct {
	// TestCode is the penalty for clones in *_test.go files. Default: 0.5.
	TestCode float64 `yaml:"test_code"`

	// GeneratedCode is the penalty for clones in generated files. Default: 0.3.
	GeneratedCode float64 `yaml:"generated_code"`

	// SmallRegion is the penalty for regions with fewer than 5 statements. Default: 0.7.
	SmallRegion float64 `yaml:"small_region"`

	// SingleErrorCheck is the penalty for pure "if err != nil { return }" patterns. Default: 0.6.
	SingleErrorCheck float64 `yaml:"single_error_check"`

	// SameFunction is the penalty for clones within the same function. Default: 0.8.
	SameFunction float64 `yaml:"same_function"`
}

// PathsConfig controls file discovery: which files to include or exclude.
type PathsConfig struct {
	// Include is a list of glob patterns for files to analyze. Default: ["**/*.go"].
	Include []string `yaml:"include"`

	// Exclude is a list of glob patterns for files to skip.
	Exclude []string `yaml:"exclude"`

	// IncludeTests controls whether test files (*_test.go) are analyzed.
	// When true, test files are included but receive a scoring penalty. Default: true.
	IncludeTests bool `yaml:"include_tests"`

	// IncludeVendor controls whether the vendor/ directory is analyzed. Default: false.
	IncludeVendor bool `yaml:"include_vendor"`

	// IncludeGenerated controls whether files with "Code generated" markers are analyzed.
	// Default: false.
	IncludeGenerated bool `yaml:"include_generated"`

	// FollowSymlinks controls whether symbolic links are followed during discovery.
	// Default: false (symlinks are skipped for security).
	FollowSymlinks bool `yaml:"follow_symlinks"`
}

// CacheConfig controls the incremental content-hash cache.
type CacheConfig struct {
	// Enabled controls whether caching is active. Default: true.
	Enabled bool `yaml:"enabled"`

	// Dir is the cache directory path relative to the scan root. Default: ".amimica/cache".
	Dir string `yaml:"dir"`
}

// OutputConfig controls how findings are formatted and where they are written.
type OutputConfig struct {
	// Format is the output encoding: "text", "json", "sarif", or "markdown". Default: "text".
	Format string `yaml:"format"`

	// File is the output file path. Empty string means write to stdout. Default: "".
	File string `yaml:"file"`

	// SortBy controls finding ordering: "score", "lines", "regions", or "file". Default: "score".
	SortBy string `yaml:"sort_by"`

	// ShowSuppressed controls whether noise-suppressed findings are included. Default: false.
	ShowSuppressed bool `yaml:"show_suppressed"`
}

// PerformanceConfig controls parallelism and resource usage limits.
type PerformanceConfig struct {
	// Jobs is the number of parallel worker goroutines. 0 means use GOMAXPROCS. Default: 0.
	Jobs int `yaml:"jobs"`

	// MaxFileSize is the maximum file size in bytes to analyze. Default: 1048576 (1MB).
	MaxFileSize int64 `yaml:"max_file_size"`

	// MaxUnitsPerFile is a safety cap on units extracted from a single file. Default: 1000.
	MaxUnitsPerFile int `yaml:"max_units_per_file"`
}

// LoggingConfig controls the structured logger output.
type LoggingConfig struct {
	// Level is the minimum log severity: "debug", "info", "warn", or "error". Default: "info".
	Level string `yaml:"level"`

	// Format is the log encoding: "text" or "json". Default: "text".
	Format string `yaml:"format"`
}

// Default returns a fully-populated Config with all recommended default values.
// This is the baseline that Load() merges YAML settings over.
func Default() *Config {
	return &Config{
		Version: 1,
		Analysis: AnalysisConfig{
			NormalizationLevel:    "strong",
			MinStatements:         3,
			MinLines:              6,
			WindowSize:            5,
			WindowMinFunctionSize: 8,
			Thorough:              false,
			ShingleSize:           7,
			MinHashFunctions:      128,
			LSHBands:              16,
			LSHRows:               8,
		},
		Scoring: ScoringConfig{
			MinScore:    0.15,
			MaxFindings: 0, // 0 = no limit
			Weights: ScoringWeights{
				Similarity:      0.30,
				Impact:          0.25,
				Refactorability: 0.20,
				Repetition:      0.15,
				Confidence:      0.10,
			},
			Penalties: ScoringPenalties{
				TestCode:         0.5,
				GeneratedCode:    0.3,
				SmallRegion:      0.7,
				SingleErrorCheck: 0.6,
				SameFunction:     0.8,
			},
		},
		Paths: PathsConfig{
			Include: []string{
				"**/*.go", "**/*.js", "**/*.jsx", "**/*.ts", "**/*.tsx",
				"**/*.rb", "**/*.m", "**/*.mm", "**/*.cs",
			},
			Exclude: []string{
				"vendor/**",
				"**/*.pb.go",
				"**/mock_*.go",
				"**/zz_generated*.go",
				"**/*.min.js",
				"**/*.min.css",
				"**/*.bundle.js",
				"**/*.chunk.js",
				"**/dist/**",
				"**/build/**",
				"**/public/packs/**",
				"**/public/vite-dev/**",
				"**/public/assets/**",
				"**/coverage/**",
				"**/obj/**",
				"**/*.Designer.cs",
				"**/*.g.cs",
			},
			IncludeTests:     true,
			IncludeVendor:    false,
			IncludeGenerated: false,
			FollowSymlinks:   false,
		},
		Cache: CacheConfig{
			Enabled: true,
			Dir:     ".amimica/cache",
		},
		Output: OutputConfig{
			Format:         "text",
			File:           "",
			SortBy:         "score",
			ShowSuppressed: false,
		},
		Performance: PerformanceConfig{
			Jobs:            0,
			MaxFileSize:     1048576,
			MaxUnitsPerFile: 1000,
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "text",
		},
	}
}
