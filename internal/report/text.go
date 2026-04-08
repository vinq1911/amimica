// Package report formats analysis findings for output.
// Supports text (terminal), JSON, SARIF, and markdown formats.
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/user/amimica/internal/model"
)

// Result holds the complete analysis output.
type Result struct {
	Version       string           `json:"version"`
	ScanRoot      string           `json:"scan_root"`
	Timestamp     time.Time        `json:"timestamp"`
	FilesScanned  int              `json:"files_scanned"`
	FuncsAnalyzed int              `json:"functions_analyzed"`
	UnitsAnalyzed int              `json:"units_analyzed"`
	Duration      time.Duration    `json:"duration_ms"`
	Findings      []model.Finding  `json:"findings"`
}

// WriteText writes a human-readable text report to w.
func WriteText(w io.Writer, r *Result) error {
	fmt.Fprintf(w, "\nAmimica Clone Detection Report\n")
	fmt.Fprintf(w, "══════════════════════════════════════════\n")
	fmt.Fprintf(w, "Scanned: %d files, %d functions, %d units\n",
		r.FilesScanned, r.FuncsAnalyzed, r.UnitsAnalyzed)
	fmt.Fprintf(w, "Time: %s\n\n", r.Duration.Round(time.Millisecond))

	// Filter out suppressed findings for display.
	var visible []model.Finding
	for _, f := range r.Findings {
		if !f.Suppressed {
			visible = append(visible, f)
		}
	}

	if len(visible) == 0 {
		fmt.Fprintf(w, "No clones detected above threshold.\n\n")
		return nil
	}

	fmt.Fprintf(w, "Found %d clone classes\n\n", len(visible))

	for i, f := range visible {
		typeStr := f.Type.String()
		typeStr = strings.ToUpper(typeStr[:1]) + typeStr[1:]
		typeStr = strings.ReplaceAll(typeStr, "_", " ")

		fmt.Fprintf(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
		fmt.Fprintf(w, "#%-3d Score: %.2f  │  %s  │  %d regions\n",
			i+1, f.Score.Composite, typeStr, len(f.Regions))
		fmt.Fprintf(w, "     ID: F-%s\n", f.ID.String()[:10])

		for _, r := range f.Regions {
			loc := fmt.Sprintf("  %s:%d-%d", r.File, r.StartLine, r.EndLine)
			if r.FuncName != "" {
				if r.Receiver != "" {
					loc += fmt.Sprintf(" (%s) %s", r.Receiver, r.FuncName)
				} else {
					loc += fmt.Sprintf(" %s", r.FuncName)
				}
			}
			fmt.Fprintf(w, "    %s\n", loc)
		}

		fmt.Fprintf(w, "     Similarity: %.0f%% (%s)\n",
			f.Score.Similarity*100, f.Evidence.SimilarityMetric)

		if len(f.RefactorHints) > 0 {
			fmt.Fprintf(w, "     Refactor: %s\n", f.RefactorHints[0].Description)
		}

		if len(f.Score.Penalties) > 0 {
			var penaltyStrs []string
			for _, p := range f.Score.Penalties {
				penaltyStrs = append(penaltyStrs, fmt.Sprintf("%s (×%.1f)", p.Reason, p.Factor))
			}
			fmt.Fprintf(w, "     Penalties: %s\n", strings.Join(penaltyStrs, ", "))
		}
		fmt.Fprintln(w)
	}

	// Summary.
	high, medium, low := 0, 0, 0
	for _, f := range visible {
		switch {
		case f.Score.Composite > 0.8:
			high++
		case f.Score.Composite > 0.5:
			medium++
		default:
			low++
		}
	}

	fmt.Fprintf(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(w, "Summary:\n")
	fmt.Fprintf(w, "  High (>0.8):     %d\n", high)
	fmt.Fprintf(w, "  Medium (0.5-0.8): %d\n", medium)
	fmt.Fprintf(w, "  Low (<0.5):      %d\n", low)
	fmt.Fprintln(w)

	return nil
}

// WriteJSON writes findings as JSON to w.
func WriteJSON(w io.Writer, r *Result) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
