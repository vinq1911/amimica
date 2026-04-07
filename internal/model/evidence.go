package model

// Evidence captures the detailed justification for a clone finding.
// It answers: what matched, what differed, at what level, and why we call it a clone.
type Evidence struct {
	// MatchedNormForm is the normalized code form that matched across all regions.
	// This is the canonical normalized text that made the clone detector classify
	// these regions as clones.
	MatchedNormForm string

	// Diffs contains per-pair diffs showing exactly what differs between regions.
	// For a clone class with N regions, there are at most N*(N-1)/2 diffs.
	Diffs []RegionDiff

	// SharedTokens is the count of normalized tokens shared across all regions.
	SharedTokens int

	// TotalTokens is the total number of normalized tokens across all regions.
	TotalTokens int

	// SimilarityMetric identifies which metric produced this match.
	// Examples: "exact_hash", "jaccard_shingle", "minhash_lsh", "rolling_hash".
	SimilarityMetric string

	// SimilarityValue is the numeric value of the similarity metric (0.0-1.0).
	SimilarityValue float64

	// ContributingNodes lists the AST node types that drove similarity.
	// Examples: ["*ast.IfStmt", "*ast.RangeStmt", "*ast.CallExpr"].
	ContributingNodes []string
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
