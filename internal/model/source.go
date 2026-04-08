package model

// SourceFile represents a single source file discovered during analysis.
// It captures file-level metadata needed throughout the pipeline.
type SourceFile struct {
	// Path is the absolute filesystem path to the file.
	Path string

	// RelPath is the path relative to the scan root.
	RelPath string

	// Package is the package name declared in the file (Go-specific; may be empty for other languages).
	Package string

	// Language is the detected language name (e.g., "go", "javascript", "ruby").
	Language string

	// Module is the Go module path from the nearest go.mod (empty if outside any module).
	Module string

	// ContentHash is the SHA-256 hash of the file's raw byte content.
	// Used for cache lookup and incremental scan support.
	ContentHash [32]byte

	// Size is the file size in bytes.
	Size int64

	// IsTest is true for files whose name ends in _test.go.
	IsTest bool

	// IsGenerated is true for files containing the canonical "Code generated" marker
	// in their first 2048 bytes.
	IsGenerated bool

	// ParseErrors contains non-fatal errors encountered while parsing the file.
	// A non-empty slice does not prevent analysis — Go's parser returns partial ASTs.
	ParseErrors []ParseError
}

// SourceRegion identifies a contiguous span of source code within a file.
// Regions are used to pinpoint where clones were detected.
type SourceRegion struct {
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	StartCol  int    `json:"start_col,omitempty"`
	EndCol    int    `json:"end_col,omitempty"`
	FuncName  string `json:"func_name,omitempty"`
	Receiver  string `json:"receiver,omitempty"`
	Package   string `json:"package,omitempty"`
}

// ParseError records a single non-fatal parse error encountered in a source file.
type ParseError struct {
	// Line is the 1-based line number where the error occurred.
	Line int

	// Column is the 1-based column offset where the error occurred.
	Column int

	// Msg is the human-readable error message from the Go parser.
	Msg string
}
