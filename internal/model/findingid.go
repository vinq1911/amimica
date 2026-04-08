package model

import (
	"encoding/hex"
	"encoding/json"
)

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

// String returns the short display form "F-" plus the first 10 hex chars.
func (id FindingID) String() string {
	return hex.EncodeToString(id[:])
}

// MarshalJSON encodes FindingID as a JSON string "F-<hex10>".
func (id FindingID) MarshalJSON() ([]byte, error) {
	s := "F-" + hex.EncodeToString(id[:])[:10]
	return json.Marshal(s)
}

// UnmarshalJSON decodes a "F-<hex>" string back into a FindingID.
func (id *FindingID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	// Strip "F-" prefix if present.
	if len(s) > 2 && s[:2] == "F-" {
		s = s[2:]
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return err
	}
	copy(id[:], b)
	return nil
}
