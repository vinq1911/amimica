package model

// Evidence captures the detailed justification for a clone finding.
// It answers: what matched, what differed, at what level, and why we call it a clone.
type Evidence struct {
	MatchedNormForm   string       `json:"matched_normalized_form,omitempty"`
	Diffs             []RegionDiff `json:"diffs,omitempty"`
	SharedTokens      int          `json:"shared_tokens"`
	TotalTokens       int          `json:"total_tokens"`
	SimilarityMetric  string       `json:"similarity_metric"`
	SimilarityValue   float64      `json:"similarity_value"`
	ContributingNodes []string     `json:"contributing_nodes,omitempty"`
}

// RegionDiff captures the diff between two specific regions in a clone class.
type RegionDiff struct {
	// RegionA is the first region in the diff pair.
	RegionA SourceRegion

	// RegionB is the second region in the diff pair.
	RegionB SourceRegion

	// Hunks contains the unified diff fragments between RegionA and RegionB.
	Hunks []DiffHunk
}

// DiffHunk represents a single contiguous section of the unified diff between
// two clone regions. Hunks show exactly where the code diverges.
type DiffHunk struct {
	// LineA is the starting line in RegionA for this hunk.
	LineA int

	// LineB is the starting line in RegionB for this hunk.
	LineB int

	// Content is the unified diff fragment text (with +/- context lines).
	Content string
}
