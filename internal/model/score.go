package model

// Score captures the multi-dimensional quality assessment for a clone finding.
// The composite score is used for ranking and noise filtering.
type Score struct {
	// Confidence is how certain the detector is that this is a real clone (0.0-1.0).
	// Exact hash matches at NormRaw score 1.0; approximate LSH matches score 0.50-0.80.
	Confidence float64

	// Similarity is the structural similarity between regions (0.0-1.0).
	// Computed as the Jaccard coefficient of normalized token shingle sets.
	Similarity float64

	// Impact measures how much code is affected by this clone (0.0-1.0).
	// Higher when more lines and more packages are involved.
	Impact float64

	// Refactorability estimates how likely this clone is to be safely refactored (0.0-1.0).
	// Higher when all regions are in the same package and differ only in literals.
	Refactorability float64

	// Composite is the weighted combination of the above metrics, used for ranking.
	// Computed as: w_similarity*Similarity + w_impact*Impact + w_refactorability*Refactorability
	//              + w_repetition*RepetitionFactor + w_confidence*Confidence
	Composite float64

	// Penalties is the list of multiplicative penalties applied to reduce the composite score.
	Penalties []Penalty
}

// Penalty represents a single multiplicative penalty applied to a finding's score.
// Penalties reduce the composite score for findings that are less actionable
// (e.g., clones in test code, generated code, or very small regions).
type Penalty struct {
	// Reason is a human-readable description of why the penalty was applied.
	Reason string

	// Factor is the multiplicative factor applied to the composite score.
	// Values less than 1.0 reduce the score (e.g., 0.5 halves it).
	Factor float64
}
