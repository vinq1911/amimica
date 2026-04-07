package model

// CloneType classifies the kind of code clone detected.
type CloneType int

const (
	// CloneExact indicates identical code (ignoring whitespace and comments).
	// Detected at NormRaw level. Also called Type-1 clone.
	CloneExact CloneType = iota

	// CloneRenamed indicates structurally identical code with different identifiers
	// or literals. Detected at NormStrong level. Also called Type-2 clone.
	CloneRenamed

	// CloneNearDuplicate indicates structurally similar code with insertions,
	// deletions, or modifications. Detected via shingle similarity + MinHash/LSH.
	// Also called Type-3 clone.
	CloneNearDuplicate

	// ClonePattern indicates a recurring structural idiom (e.g., repeated handler
	// scaffolding). Detected via normalized block fingerprint clustering.
	ClonePattern
)

// String returns the canonical string representation of a CloneType.
func (t CloneType) String() string {
	switch t {
	case CloneExact:
		return "exact"
	case CloneRenamed:
		return "renamed"
	case CloneNearDuplicate:
		return "near_duplicate"
	case ClonePattern:
		return "pattern"
	default:
		return "unknown"
	}
}

// Finding represents a detected code clone. It groups one or more source regions
// that share structural similarity above the detection threshold.
//
// Findings are deterministic: the same code in the same repository always produces
// the same Finding with the same ID.
type Finding struct {
	// ID is the deterministic hash-based identifier for this finding.
	ID FindingID

	// CloneClassID groups all members of the same clone class together.
	// Multiple findings may share a CloneClassID if they belong to the same
	// transitively-connected group of similar code regions.
	CloneClassID string

	// Type is the classification of the clone relationship.
	Type CloneType

	// Regions lists all source code regions that form this clone class.
	// There are always at least two regions in a clone finding.
	Regions []SourceRegion

	// NormLevel is the normalization level at which the match was first detected.
	NormLevel NormalizationLevel

	// Score contains the composite quality scores for this finding.
	Score Score

	// Evidence contains the detailed evidence that justifies this finding.
	Evidence Evidence

	// RefactorHints contains actionable refactoring suggestions for this clone.
	RefactorHints []RefactorHint

	// Suppressed is true when this finding was filtered by noise-suppression rules.
	// Suppressed findings are retained in the output when show_suppressed is enabled.
	Suppressed bool

	// SuppressReason explains why the finding was suppressed (if Suppressed is true).
	SuppressReason string
}
