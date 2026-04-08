package model

// Score captures the multi-dimensional quality assessment for a clone finding.
// The composite score is used for ranking and noise filtering.
type Score struct {
	Confidence      float64   `json:"confidence"`
	Similarity      float64   `json:"similarity"`
	Impact          float64   `json:"impact"`
	Refactorability float64   `json:"refactorability"`
	Composite       float64   `json:"composite"`
	Penalties       []Penalty `json:"penalties,omitempty"`
}

// Penalty represents a single multiplicative penalty applied to a finding's score.
// Penalties reduce the composite score for findings that are less actionable
// (e.g., clones in test code, generated code, or very small regions).
type Penalty struct {
	Reason string  `json:"reason"`
	Factor float64 `json:"factor"`
}
