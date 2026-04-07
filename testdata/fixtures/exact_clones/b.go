// Package clones contains test fixtures for exact clone detection.
// Both files in this package contain functions with identical bodies to exercise
// the Type-1 (exact) clone detection pipeline.
package clones

import "sort"

// ProcessB takes a slice of strings, filters out empty strings, sorts the
// remaining strings alphabetically, and returns the result.
// This function is intentionally an exact clone of ProcessA (different name only).
func ProcessB(items []string) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item == "" {
			continue
		}
		result = append(result, item)
	}
	sort.Strings(result)
	return result
}
