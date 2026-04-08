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

// ScoreFindings converts clone classes into scored findings, then deduplicates
// window findings that are contained within a higher-scoring function finding.
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

		// Determine the dominant unit kind in this class.
		funcCount, winCount := 0, 0
		for _, idx := range class.UnitIdxs {
			switch units[idx].Kind {
			case model.UnitFunction:
				funcCount++
			case model.UnitWindow:
				winCount++
			}
		}
		dominantKind := model.UnitWindow
		if funcCount >= winCount {
			dominantKind = model.UnitFunction
		}

		// Build regions and evidence.
		var regions []model.SourceRegion
		totalTokens := 0
		totalLines := 0

		for _, idx := range class.UnitIdxs {
			u := &units[idx]
			r := u.Source
			if sf := fileMap[r.File]; sf != nil {
				r.Package = sf.Package
			}
			regions = append(regions, r)
			totalTokens += len(u.NormTokens)
			totalLines += u.Source.EndLine - u.Source.StartLine + 1
		}

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

		// Function-level matches are more actionable than window fragments.
		// Boost functions, penalize windows.
		if dominantKind == model.UnitFunction {
			composite *= 1.15 // 15% boost for full-function matches
		} else if dominantKind == model.UnitWindow {
			composite *= 0.85 // 15% penalty for window fragments
		}

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

		// Penalize findings with very few tokens — they match on generic
		// structural skeletons (JSX closing tags, short assignments) rather
		// than meaningful logic.
		avgTokens := totalTokens / len(regions)
		if avgTokens < 25 {
			p := model.Penalty{Reason: "low token count", Factor: 0.6}
			penalties = append(penalties, p)
			composite *= p.Factor
		}

		// Penalize window-level findings where all regions are in the same
		// file and function — these are internal repetition within a single
		// component, usually intentional (JSX variant lists, switch cases).
		if dominantKind == model.UnitWindow && len(regions) >= 2 {
			sameFileFn := true
			for i := 1; i < len(regions); i++ {
				if regions[i].File != regions[0].File || regions[i].FuncName != regions[0].FuncName {
					sameFileFn = false
					break
				}
			}
			if sameFileFn {
				p := model.Penalty{Reason: "same-function window repetition", Factor: 0.5}
				penalties = append(penalties, p)
				composite *= p.Factor
			}
		}

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
			UnitKind: dominantKind,
		}

		if composite < cfg.Scoring.MinScore {
			finding.Suppressed = true
			finding.SuppressReason = fmt.Sprintf("score %.2f below threshold %.2f", composite, cfg.Scoring.MinScore)
		}

		finding.RefactorHints = suggestRefactoring(class, units, regions)
		findings = append(findings, finding)
	}

	// Sort by composite score descending.
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Score.Composite > findings[j].Score.Composite
	})

	// Deduplicate: suppress window findings whose regions are subsets of a
	// higher-scoring function finding's regions.
	findings = deduplicateSubsumedWindows(findings)

	// Merge overlapping window findings that match the same file pairs.
	// e.g., fileA:42-47↔fileB:75-80 and fileA:43-48↔fileB:76-81 become
	// one finding spanning fileA:42-48↔fileB:75-81.
	findings = mergeOverlappingFindings(findings)

	// Cap at max findings (only count non-suppressed).
	if cfg.Scoring.MaxFindings > 0 {
		visible := 0
		for i := range findings {
			if !findings[i].Suppressed {
				visible++
				if visible > cfg.Scoring.MaxFindings {
					findings = findings[:i]
					break
				}
			}
		}
	}

	return findings
}

// deduplicateSubsumedWindows suppresses window-level findings whose regions
// are all contained within regions of a higher-scoring function-level finding.
// This ensures that e.g. a full AudioProcessor.process match outranks the
// 5-line window fragments from the same function pair.
func deduplicateSubsumedWindows(findings []model.Finding) []model.Finding {
	// Build an index of function-level finding regions for fast lookup.
	// Key: file path → list of (startLine, endLine) from function findings.
	type lineRange struct {
		start, end int
	}
	funcRegions := make(map[string][]lineRange)

	for _, f := range findings {
		if f.Suppressed || f.UnitKind != model.UnitFunction {
			continue
		}
		for _, r := range f.Regions {
			funcRegions[r.File] = append(funcRegions[r.File], lineRange{r.StartLine, r.EndLine})
		}
	}

	for i := range findings {
		f := &findings[i]
		if f.Suppressed || f.UnitKind != model.UnitWindow {
			continue
		}

		// Check if ALL regions of this window finding are contained within
		// some function finding's regions.
		allContained := true
		for _, r := range f.Regions {
			ranges := funcRegions[r.File]
			contained := false
			for _, fr := range ranges {
				if r.StartLine >= fr.start && r.EndLine <= fr.end {
					contained = true
					break
				}
			}
			if !contained {
				allContained = false
				break
			}
		}

		if allContained {
			f.Suppressed = true
			f.SuppressReason = "window subsumed by function-level finding"
		}
	}

	return findings
}

// mergeOverlappingFindings consolidates window-level findings that share the
// same set of (file, funcName) pairs with overlapping line ranges into a single
// finding spanning the combined range. This turns three findings like:
//
//	fileA:42-47 ↔ fileB:75-80
//	fileA:43-48 ↔ fileB:76-81
//	fileA:45-50 ↔ fileB:78-83
//
// into one finding: fileA:42-50 ↔ fileB:75-83, keeping the highest score.
func mergeOverlappingFindings(findings []model.Finding) []model.Finding {
	// Build a fingerprint for each finding based on its set of (file, funcName) pairs.
	// Findings with the same fingerprint are candidates for merging.
	type fileFuncKey struct {
		file     string
		funcName string
	}

	// Group findings by their file-function pair signature.
	type groupKey string
	groups := make(map[groupKey][]int) // groupKey → finding indices

	for i, f := range findings {
		if f.Suppressed || f.UnitKind != model.UnitWindow {
			continue
		}
		// Build a canonical key from sorted (file, funcName) pairs.
		keys := make([]fileFuncKey, len(f.Regions))
		for j, r := range f.Regions {
			keys[j] = fileFuncKey{file: r.File, funcName: r.FuncName}
		}
		sort.Slice(keys, func(a, b int) bool {
			if keys[a].file != keys[b].file {
				return keys[a].file < keys[b].file
			}
			return keys[a].funcName < keys[b].funcName
		})
		var buf string
		for _, k := range keys {
			buf += k.file + ":" + k.funcName + "|"
		}
		groups[groupKey(buf)] = append(groups[groupKey(buf)], i)
	}

	// For each group with multiple findings, merge overlapping ones.
	for _, idxs := range groups {
		if len(idxs) < 2 {
			continue
		}

		// Keep the highest-scoring finding, expand its regions, suppress the rest.
		// idxs[0] has the highest score (findings are sorted by score descending).
		best := idxs[0]

		for _, other := range idxs[1:] {
			// Expand the best finding's regions to cover the other's ranges.
			for j := range findings[best].Regions {
				for _, otherRegion := range findings[other].Regions {
					if findings[best].Regions[j].File == otherRegion.File &&
						findings[best].Regions[j].FuncName == otherRegion.FuncName {
						if otherRegion.StartLine < findings[best].Regions[j].StartLine {
							findings[best].Regions[j].StartLine = otherRegion.StartLine
						}
						if otherRegion.EndLine > findings[best].Regions[j].EndLine {
							findings[best].Regions[j].EndLine = otherRegion.EndLine
						}
						break
					}
				}
			}

			// Suppress the merged finding.
			findings[other].Suppressed = true
			findings[other].SuppressReason = "merged into overlapping finding"
		}

		// Recalculate the finding ID since regions changed.
		findings[best].ID = computeFindingID(findings[best].Regions, findings[best].Type, findings[best].NormLevel)
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
	score := 0.5

	dirSet := make(map[string]bool)
	for _, r := range regions {
		dirSet[filepath.Dir(r.File)] = true
	}

	if len(dirSet) == 1 {
		score += 0.3
	} else if len(dirSet) <= 3 {
		score += 0.1
	}

	if class.Similarity >= 1.0 {
		score += 0.2
	} else if class.Similarity >= 0.8 {
		score += 0.1
	}

	return math.Min(1.0, score)
}

func suggestRefactoring(class match.CloneClass, units []model.NormalizedUnit, regions []model.SourceRegion) []model.RefactorHint {
	var hints []model.RefactorHint

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
