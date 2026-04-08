package model

import (
	"encoding/json"
	"go/token"
)

// NormalizationLevel controls how aggressively source code is normalized before
// comparison. Higher levels abstract more details, detecting more classes of clones
// at the cost of potential false positives.
type NormalizationLevel int

const (
	// NormRaw strips only whitespace and comments. Detects Type-1 (exact) clones.
	NormRaw NormalizationLevel = iota

	// NormLight replaces literals with typed placeholders ($INT, $STR, $FLOAT).
	// Detects clones that differ only in literal values.
	NormLight

	// NormStrong replaces local identifiers with positional placeholders ($V0, $V1, ...).
	// Detects Type-2 (renamed) clones.
	NormStrong

	// NormSemantic additionally abstracts selectors, type names, and channel operations.
	// Detects structural clones across very different domains.
	NormSemantic
)

// String returns the canonical string representation of a NormalizationLevel.
func (l NormalizationLevel) String() string {
	switch l {
	case NormRaw:
		return "raw"
	case NormLight:
		return "light"
	case NormStrong:
		return "strong"
	case NormSemantic:
		return "semantic"
	default:
		return "unknown"
	}
}

// MarshalJSON encodes NormalizationLevel as its string name.
func (l NormalizationLevel) MarshalJSON() ([]byte, error) { return json.Marshal(l.String()) }

// Token represents a single token from the original Go source code before normalization.
type Token struct {
	// Kind is the Go token type (e.g., token.IDENT, token.INT, token.IF).
	Kind token.Token

	// Lit is the literal string value of the token.
	// For keywords and operators, this is the symbol text.
	// For identifiers, this is the identifier name.
	// For literals, this is the literal value (e.g., "42", `"hello"`).
	Lit string

	// Pos is the position of the token in the source file's FileSet.
	Pos token.Pos
}

// NormToken represents a single token after normalization.
// The Kind is preserved from the original; only the textual form is transformed.
type NormToken struct {
	// Kind is the original Go token type, preserved through normalization.
	Kind token.Token

	// Norm is the normalized representation of the token.
	// Examples: "$ID0" (strong-normalized identifier), "$LIT" (light-normalized literal),
	// "$STR" (normalized string literal), "$INT" (normalized integer literal).
	Norm string

	// OrigLit is the original literal value before normalization.
	// Retained for explanation and diff generation.
	OrigLit string
}
