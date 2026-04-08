// Package score assigns quality scores to clone findings and filters noise.
package score

import (
	"crypto/sha1"
	"fmt"
	"math"
	"path/filepath"
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
			r := u.Source
			// Enrich region with package name from file metadata.
			if sf := fileMap[r.File]; sf != nil {
				r.Package = sf.Package
			}
			regions = append(regions, r)
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

		// Cap composite to [0, 1].
		if composite > 1.0 {
			composite = 1.0
		}

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
	regionFactor := math.Min(1.0, float64(len(regions))/5.0)
	lineFactor := math.Min(1.0, float64(totalLines)/200.0)
	return (regionFactor + lineFactor) / 2.0
}

func refactorabilityScore(regions []model.SourceRegion, class match.CloneClass) float64 {
	score := 0.5 // base

	// Count distinct directories (proxy for packages).
	dirSet := make(map[string]bool)
	for _, r := range regions {
		dirSet[filepath.Dir(r.File)] = true
	}

	if len(dirSet) == 1 {
		// All in same directory/package — easiest to refactor.
		score += 0.3
	} else if len(dirSet) <= 3 {
		// Few packages — still manageable.
		score += 0.1
	}

	// Exact match is easier to refactor than near-duplicate.
	if class.Similarity >= 1.0 {
		score += 0.2
	} else if class.Similarity >= 0.8 {
		score += 0.1
	}

	return math.Min(1.0, score)
}

func suggestRefactoring(class match.CloneClass, units []model.NormalizedUnit, regions []model.SourceRegion) []model.RefactorHint {
	var hints []model.RefactorHint

	// Count distinct packages.
	dirSet := make(map[string]bool)
	for _, r := range regions {
		dirSet[filepath.Dir(r.File)] = true
	}
	crossPackage := len(dirSet) > 1

	if class.Similarity >= 1.0 && len(regions) >= 3 {
		if crossPackage {
			hints = append(hints, model.RefactorHint{
				Category:    model.RefactorExtractHelper,
				Description: fmt.Sprintf("Identical structure repeated in %d regions across %d packages. Extract into a shared library.", len(regions), len(dirSet)),
				Confidence:  0.85,
			})
		} else {
			hints = append(hints, model.RefactorHint{
				Category:    model.RefactorTableDriven,
				Description: fmt.Sprintf("Identical structure repeated %d times in the same package. Consider a table-driven approach or shared helper.", len(regions)),
				Confidence:  0.80,
			})
		}
	} else if class.Similarity >= 1.0 && len(regions) == 2 {
		hints = append(hints, model.RefactorHint{
			Category:    model.RefactorExtractHelper,
			Description: "Two regions with identical normalized structure. Extract a shared helper function.",
			Confidence:  0.75,
		})
	} else if class.Similarity >= 0.8 {
		hints = append(hints, model.RefactorHint{
			Category:    model.RefactorExtractHelper,
			Description: fmt.Sprintf("Regions are %.0f%% similar. Extract shared logic and parameterize the differences.", class.Similarity*100),
			Confidence:  0.60,
		})
	} else if class.Similarity >= 0.6 {
		hints = append(hints, model.RefactorHint{
			Category:    model.RefactorInterfaceExtract,
			Description: fmt.Sprintf("Regions share %.0f%% structure. Consider an interface or strategy pattern to unify.", class.Similarity*100),
			Confidence:  0.45,
		})
	}

	return hints
}

func computeFindingID(regions []model.SourceRegion, cloneType model.CloneType, normLevel model.NormalizationLevel) model.FindingID {
	sorted := make([]model.SourceRegion, len(regions))
	copy(sorted, regions)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].File != sorted[j].File {
			return sorted[i].File < sorted[j].File
		}
		if sorted[i].StartLine != sorted[j].StartLine {
			return sorted[i].StartLine < sorted[j].StartLine
		}
		return sorted[i].EndLine < sorted[j].EndLine
	})

	h := sha1.New()
	for _, r := range sorted {
		fmt.Fprintf(h, "%s:%d:%d\n", r.File, r.StartLine, r.EndLine)
	}
	fmt.Fprintf(h, "type:%d\nlevel:%d\n", cloneType, normLevel)

	var id model.FindingID
	copy(id[:], h.Sum(nil)[:20])
	return id
}
