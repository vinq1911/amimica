package model

// UnitID is a deterministic, content-derived identifier for a NormalizedUnit.
// It is a SHA-256 hash of the unit's normalized token sequence combined with
// its source location, making it stable across repeated scans of the same code.
type UnitID = [32]byte

// UnitKind classifies the structural type of a code unit extracted for analysis.
type UnitKind int

const (
	// UnitFunction represents an entire function or method body.
	UnitFunction UnitKind = iota

	// UnitBlock represents a single control-flow block (if/for/switch/select body).
	UnitBlock

	// UnitWindow represents a sliding window of N consecutive statements within a function.
	UnitWindow

	// UnitSubtree represents an arbitrary AST subtree (enabled only in thorough mode).
	UnitSubtree
)

// String returns the canonical string name for a UnitKind.
func (k UnitKind) String() string {
	switch k {
	case UnitFunction:
		return "function"
	case UnitBlock:
		return "block"
	case UnitWindow:
		return "window"
	case UnitSubtree:
		return "subtree"
	default:
		return "unknown"
	}
}

// NormalizedUnit is the fundamental analysis unit produced by the pipeline.
// Each unit represents a code fragment (function, block, window, or subtree)
// after normalization. Units are fingerprinted and compared to detect clones.
type NormalizedUnit struct {
	// ID is the deterministic content-derived identifier.
	ID UnitID

	// Source identifies where in the source code this unit came from.
	Source SourceRegion

	// Kind classifies the structural type of this unit.
	Kind UnitKind

	// RawTokens is the original token sequence before normalization.
	RawTokens []Token

	// NormTokens is the normalized token sequence used for similarity analysis.
	NormTokens []NormToken

	// NormLevel is the normalization level applied to produce NormTokens.
	NormLevel NormalizationLevel

	// ASTHash is the SHA-256 hash of the normalized AST subtree representation.
	ASTHash [32]byte

	// TokenHash is the SHA-256 hash of the NormTokens sequence.
	TokenHash [32]byte

	// Shingles contains the n-gram hashes computed from NormTokens.
	// Populated lazily during fingerprinting.
	Shingles []uint64

	// MinHash is the MinHash signature for approximate similarity computation.
	// Populated lazily — only computed for units that survive exact-hash grouping.
	MinHash []uint32

	// StmtCount is the number of statements in this unit.
	StmtCount int

	// NodeCount is the total number of AST nodes in this unit.
	NodeCount int

	// Complexity is an estimate of cyclomatic complexity.
	Complexity int
}
