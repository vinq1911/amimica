package lang

import (
	"bytes"
	"strings"
)

// HasIgnoreComment checks whether the source lines near startLine contain
// an "amimica-ignore" directive. It looks at the line itself and the two
// preceding lines (where a comment would typically be placed).
//
// Recognized formats:
//   - // amimica-ignore
//   - /* amimica-ignore */
//   - # amimica-ignore
//   - // amimica-ignore: reason
func HasIgnoreComment(src []byte, startLine int) bool {
	lines := bytes.Split(src, []byte("\n"))

	// Check the function line and up to 2 lines before it.
	from := startLine - 3 // 0-indexed: startLine-1 is the line, -2 more above
	if from < 0 {
		from = 0
	}
	to := startLine // exclusive, so this includes startLine-1 (0-indexed)
	if to > len(lines) {
		to = len(lines)
	}

	for i := from; i < to; i++ {
		if strings.Contains(string(lines[i]), "amimica-ignore") {
			return true
		}
	}
	return false
}
