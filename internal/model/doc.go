// Package model defines the core shared data types used throughout the Amimica
// code clone detection pipeline.
//
// All types in this package are dependency-free — they import only the Go
// standard library's go/token package for token type constants. This makes
// model the foundation every other package can safely import without risk of
// circular dependencies.
//
// Key type groups:
//   - Source representation: SourceFile, SourceRegion, ParseError
//   - Analysis units: NormalizedUnit, UnitKind, UnitID, NormalizationLevel
//   - Tokenization: Token, NormToken
//   - Findings: Finding, FindingID, CloneType
//   - Scoring: Score, Penalty
//   - Evidence: Evidence, RegionDiff, DiffHunk
//   - Refactoring hints: RefactorHint, RefactorCategory
package model
