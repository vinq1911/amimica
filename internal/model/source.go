package model

// SourceFile represents a single Go source file discovered during analysis.
// It captures file-level metadata needed throughout the pipeline.
type SourceFile struct {
	// Path is the absolute filesystem path to the file.
	Path string

	// RelPath is the path relative to the scan root.
	RelPath string

	// Package is the Go package name declared in the file.
	Package string

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
	// File is the relative file path within the scan root.
	File string

	// StartLine is the 1-based line number where the region begins.
	StartLine int

	// EndLine is the 1-based line number where the region ends (inclusive).
	EndLine int

	// StartCol is the 1-based column offset of the first character.
	StartCol int

	// EndCol is the 1-based column offset of the last character.
	EndCol int

	// FuncName is the name of the enclosing function or method, if any.
	FuncName string

	// Receiver is the receiver type name for methods (e.g., "*UserService").
	Receiver string
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
