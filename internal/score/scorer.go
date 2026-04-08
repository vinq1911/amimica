// Package score assigns quality scores to clone findings and filters noise.
package score

import (
	"crypto/sha1"
	"fmt"
	"math"
	"sort"

	"github.com/user/amimica/internal/config"
	"github.com/user/amimica/internal/extract"
	"github.com/user/amimica/internal/match"
	"github.com/user/amimica/internal/model"
)

// ScoreFindings converts clone classes into scored findings.
func ScoreFindings(classes []match.CloneClass, units []model.NormalizedUnit, files []model.SourceFile, cfg *config.Config) []model.Finding {
	fileMap := make(map[string]*model.SourceFile)
	for i := range files {
		fileMap[files[i].RelPath] = &files[i]
	}

	var findings []model.Finding

	for _, class := range classes {
		if len(class.UnitIdxs) < 2 {
			continue
		}

		// Build regions and evidence.
		var regions []model.SourceRegion
		totalTokens := 0
		totalLines := 0

		for _, idx := range class.UnitIdxs {
			u := &units[idx]
			regions = append(regions, u.Source)
			totalTokens += len(u.NormTokens)
			totalLines += u.Source.EndLine - u.Source.StartLine + 1
		}

		// Normalized form from first unit (they're all similar).
		normForm := extract.TokensToString(units[class.UnitIdxs[0]].NormTokens)

		// Calculate scores.
		confidence := confidenceScore(class)
		similarity := class.Similarity
		impact := impactScore(regions, totalLines)
		refactorability := refactorabilityScore(regions, class)
		repetition := math.Min(1.0, float64(len(regions))/10.0)

		composite := cfg.Scoring.Weights.Confidence*confidence +
			cfg.Scoring.Weights.Similarity*similarity +
			cfg.Scoring.Weights.Impact*impact +
			cfg.Scoring.Weights.Refactorability*refactorability +
			cfg.Scoring.Weights.Repetition*repetition

		// Apply penalties.
		var penalties []model.Penalty

		allTest := true
		allGenerated := true
		for _, r := range regions {
			sf := fileMap[r.File]
			if sf == nil {
				allTest = false
				allGenerated = false
				continue
			}
			if !sf.IsTest {
				allTest = false
			}
			if !sf.IsGenerated {
				allGenerated = false
			}
		}

		if allTest {
			p := model.Penalty{Reason: "all regions in test files", Factor: cfg.Scoring.Penalties.TestCode}
			penalties = append(penalties, p)
			composite *= p.Factor
		}
		if allGenerated {
			p := model.Penalty{Reason: "all regions in generated files", Factor: cfg.Scoring.Penalties.GeneratedCode}
			penalties = append(penalties, p)
			composite *= p.Factor
		}

		avgStmts := 0
		for _, idx := range class.UnitIdxs {
			avgStmts += units[idx].StmtCount
		}
		avgStmts /= len(class.UnitIdxs)
		if avgStmts < 5 {
			p := model.Penalty{Reason: "small region", Factor: cfg.Scoring.Penalties.SmallRegion}
			penalties = append(penalties, p)
			composite *= p.Factor
		}

		if len(regions) == 2 {
			p := model.Penalty{Reason: "only 2 members", Factor: 0.9}
			penalties = append(penalties, p)
			composite *= p.Factor
		}

		// Build finding.
		fid := computeFindingID(regions, class.Type, class.NormLevel)

		finding := model.Finding{
			ID:           fid,
			CloneClassID: fmt.Sprintf("CC-%x", fid[:5]),
			Type:         class.Type,
			Regions:      regions,
			NormLevel:    class.NormLevel,
			Score: model.Score{
				Confidence:      confidence,
				Similarity:      similarity,
				Impact:          impact,
				Refactorability: refactorability,
				Composite:       composite,
				Penalties:       penalties,
			},
			Evidence: model.Evidence{
				MatchedNormForm:  normForm,
				SharedTokens:     len(units[class.UnitIdxs[0]].NormTokens),
				TotalTokens:      totalTokens,
				SimilarityMetric: class.Metric,
				SimilarityValue:  class.Similarity,
			},
		}

		// Noise suppression.
		if composite < cfg.Scoring.MinScore {
			finding.Suppressed = true
			finding.SuppressReason = fmt.Sprintf("score %.2f below threshold %.2f", composite, cfg.Scoring.MinScore)
		}

		// Refactor hints.
		finding.RefactorHints = suggestRefactoring(class, units, regions)

		findings = append(findings, finding)
	}

	// Sort by composite score descending.
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Score.Composite > findings[j].Score.Composite
	})

	// Cap at max findings.
	if cfg.Scoring.MaxFindings > 0 && len(findings) > cfg.Scoring.MaxFindings {
		findings = findings[:cfg.Scoring.MaxFindings]
	}

	return findings
}

func confidenceScore(class match.CloneClass) float64 {
	switch class.Metric {
	case "exact_hash":
		switch {
		case class.NormLevel <= model.NormRaw:
			return 1.0
		case class.NormLevel == model.NormLight:
			return 0.95
		default:
			return 0.85
		}
	case "minhash_lsh":
		if class.Similarity > 0.8 {
			return 0.80
		}
		return 0.65
	default:
		return 0.50
	}
}

func impactScore(regions []model.SourceRegion, totalLines int) float64 {
	// More regions and more lines = higher impact.
	regionFactor := math.Min(1.0, float64(len(regions))/5.0)
	lineFactor := math.Min(1.0, float64(totalLines)/200.0)
	return (regionFactor + lineFactor) / 2.0
}

func refactorabilityScore(regions []model.SourceRegion, class match.CloneClass) float64 {
	score := 0.5 // base

	// Same package?
	pkgSet := make(map[string]bool)
	for _, r := range regions {
		// Extract package from file path (heuristic: directory name).
		pkgSet[r.File] = true
	}
	if len(pkgSet) <= 3 {
		score += 0.2
	}

	// Exact match (easy to refactor).
	if class.Similarity >= 1.0 {
		score += 0.2
	}

	return math.Min(1.0, score)
}

func suggestRefactoring(class match.CloneClass, units []model.NormalizedUnit, regions []model.SourceRegion) []model.RefactorHint {
	var hints []model.RefactorHint

	if class.Similarity >= 1.0 && len(regions) >= 2 {
		hints = append(hints, model.RefactorHint{
			Category:    model.RefactorExtractHelper,
			Description: "All regions have identical normalized structure. Extract a shared helper function.",
			Confidence:  0.8,
		})
	}

	if class.Similarity >= 0.8 && class.Similarity < 1.0 {
		hints = append(hints, model.RefactorHint{
			Category:    model.RefactorExtractHelper,
			Description: "Regions are structurally similar. Consider extracting shared logic with parameters for differences.",
			Confidence:  0.6,
		})
	}

	if len(regions) >= 3 && class.Similarity >= 1.0 {
		hints = append(hints, model.RefactorHint{
			Category:    model.RefactorTableDriven,
			Description: "Multiple identical regions suggest a table-driven approach could eliminate repetition.",
			Confidence:  0.7,
		})
	}

	return hints
}

func computeFindingID(regions []model.SourceRegion, cloneType model.CloneType, normLevel model.NormalizationLevel) model.FindingID {
	sort.Slice(regions, func(i, j int) bool {
		if regions[i].File != regions[j].File {
			return regions[i].File < regions[j].File
		}
		if regions[i].StartLine != regions[j].StartLine {
			return regions[i].StartLine < regions[j].StartLine
		}
		return regions[i].EndLine < regions[j].EndLine
	})

	h := sha1.New()
	for _, r := range regions {
		fmt.Fprintf(h, "%s:%d:%d\n", r.File, r.StartLine, r.EndLine)
	}
	fmt.Fprintf(h, "type:%d\nlevel:%d\n", cloneType, normLevel)

	var id model.FindingID
	copy(id[:], h.Sum(nil)[:20])
	return id
}
