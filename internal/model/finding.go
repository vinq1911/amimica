package model

import "encoding/json"

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

// MarshalJSON encodes CloneType as its string name.
func (t CloneType) MarshalJSON() ([]byte, error) { return json.Marshal(t.String()) }

// Finding represents a detected code clone. It groups one or more source regions
// that share structural similarity above the detection threshold.
//
// Findings are deterministic: the same code in the same repository always produces
// the same Finding with the same ID.
type Finding struct {
	// ID is the deterministic hash-based identifier for this finding.
	ID FindingID `json:"id"`

	// CloneClassID groups all members of the same clone class together.
	CloneClassID string `json:"clone_class_id"`

	// Type is the classification of the clone relationship.
	Type CloneType `json:"type"`

	// Regions lists all source code regions that form this clone class.
	Regions []SourceRegion `json:"regions"`

	// NormLevel is the normalization level at which the match was first detected.
	NormLevel NormalizationLevel `json:"normalization_level"`

	// Score contains the composite quality scores for this finding.
	Score Score `json:"score"`

	// Evidence contains the detailed evidence that justifies this finding.
	Evidence Evidence `json:"evidence"`

	// RefactorHints contains actionable refactoring suggestions for this clone.
	RefactorHints []RefactorHint `json:"refactor_hints,omitempty"`

	// Suppressed is true when this finding was filtered by noise-suppression rules.
	Suppressed bool `json:"suppressed"`

	// SuppressReason explains why the finding was suppressed (if Suppressed is true).
	SuppressReason string `json:"suppress_reason,omitempty"`

	// UnitKind records the dominant unit kind (function vs window) for this finding.
	// Used by the scorer for deduplication. Omitted from JSON output.
	UnitKind UnitKind `json:"-"`
}
