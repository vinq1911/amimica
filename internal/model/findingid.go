package model

import "encoding/hex"

// FindingID is a deterministic identifier for a clone finding.
// It is derived from:
//   - The sorted list of source regions (file path + line range)
//   - The clone type
//   - The normalization level at which the match was detected
//
// This ensures that the same code in the same locations always produces the same
// FindingID across runs, enabling stable suppression, tracking, and diffing.
//
// FindingID is stored as 20 bytes (SHA-1 truncated) and hex-encoded for display.
type FindingID [20]byte

// String returns the hex-encoded representation of the FindingID.
// Example: "a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8a9b0"
func (id FindingID) String() string {
	return hex.EncodeToString(id[:])
}
