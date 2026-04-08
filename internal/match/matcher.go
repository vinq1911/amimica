// Package match groups normalized units into clone classes using exact hash
// matching and approximate similarity via MinHash/LSH.
package match

import (
	"log/slog"

	"github.com/user/amimica/internal/config"
	"github.com/user/amimica/internal/fingerprint"
	"github.com/user/amimica/internal/model"
)

// CloneClass represents a group of code units that are clones of each other.
type CloneClass struct {
	Type       model.CloneType
	NormLevel  model.NormalizationLevel
	UnitIdxs   []int   // Indices into the units slice
	Similarity float64 // Average pairwise similarity (1.0 for exact)
	Metric     string  // Which metric detected this class
}

// FindClones detects clone classes among the given units.
// It runs exact hash matching first, then approximate matching via LSH.
func FindClones(units []model.NormalizedUnit, cfg *config.Config, log *slog.Logger) []CloneClass {
	var classes []CloneClass

	// Phase 1: Exact matching by token hash.
	exactGroups := groupByHash(units)
	matched := make(map[int]bool)

	for _, group := range exactGroups {
		if len(group) < 2 {
			continue
		}

		// Determine clone type based on normalization level.
		cloneType := model.CloneExact
		if len(units) > 0 && units[group[0]].NormLevel >= model.NormStrong {
			cloneType = model.CloneRenamed
		}

		classes = append(classes, CloneClass{
			Type:       cloneType,
			NormLevel:  units[group[0]].NormLevel,
			UnitIdxs:   group,
			Similarity: 1.0,
			Metric:     "exact_hash",
		})

		for _, idx := range group {
			matched[idx] = true
		}
	}

	log.Debug("exact matching done", "classes", len(classes), "matched_units", len(matched))

	// Phase 2: Approximate matching via MinHash + LSH for unmatched units.
	unmatched := make([]int, 0)
	for i := range units {
		if !matched[i] {
			unmatched = append(unmatched, i)
		}
	}

	if len(unmatched) < 2 {
		return classes
	}

	// Compute shingles and MinHash for unmatched units.
	shingleSize := cfg.Analysis.ShingleSize
	numHash := cfg.Analysis.MinHashFunctions

	for _, idx := range unmatched {
		u := &units[idx]
		u.Shingles = fingerprint.ComputeShingles(u.NormTokens, shingleSize)
		if len(u.Shingles) > 0 {
			u.MinHash = fingerprint.ComputeMinHash(u.Shingles, numHash)
		}
	}

	// Build LSH index.
	lsh := fingerprint.NewLSHIndex(cfg.Analysis.LSHBands, cfg.Analysis.LSHRows)
	for _, idx := range unmatched {
		if len(units[idx].MinHash) > 0 {
			lsh.Insert(idx, units[idx].MinHash)
		}
	}

	// Query candidates and verify.
	approxMatched := make(map[int]bool)
	pairSeen := make(map[[2]int]bool)

	type matchPair struct {
		a, b       int
		similarity float64
	}
	var pairs []matchPair

	for _, idx := range unmatched {
		if len(units[idx].MinHash) == 0 {
			continue
		}
		candidates := lsh.QueryCandidates(units[idx].MinHash, idx)
		for _, cand := range candidates {
			pair := [2]int{min(idx, cand), max(idx, cand)}
			if pairSeen[pair] {
				continue
			}
			pairSeen[pair] = true

			// Skip pairs from the same function (overlapping windows are not interesting clones).
			if units[idx].Source.File == units[cand].Source.File &&
				units[idx].Source.FuncName == units[cand].Source.FuncName {
				continue
			}

			// Verify with exact Jaccard.
			sim := fingerprint.JaccardShingles(units[idx].Shingles, units[cand].Shingles)
			if sim > 1.0 {
				sim = 1.0
			}
			if sim >= 0.6 { // threshold
				pairs = append(pairs, matchPair{a: idx, b: cand, similarity: sim})
			}
		}
	}

	// Cluster pairs into clone classes using union-find.
	if len(pairs) > 0 {
		uf := newUnionFind(len(units))
		for _, p := range pairs {
			uf.union(p.a, p.b)
		}

		// Group by root.
		groups := make(map[int][]int)
		simSums := make(map[int]float64)
		simCounts := make(map[int]int)

		for _, p := range pairs {
			root := uf.find(p.a)
			simSums[root] += p.similarity
			simCounts[root]++
		}

		for _, idx := range unmatched {
			root := uf.find(idx)
			if _, ok := simSums[root]; ok {
				groups[root] = append(groups[root], idx)
			}
		}

		for root, group := range groups {
			if len(group) < 2 {
				continue
			}
			avgSim := simSums[root] / float64(simCounts[root])
			classes = append(classes, CloneClass{
				Type:       model.CloneNearDuplicate,
				NormLevel:  units[group[0]].NormLevel,
				UnitIdxs:   group,
				Similarity: avgSim,
				Metric:     "minhash_lsh",
			})
			for _, idx := range group {
				approxMatched[idx] = true
			}
		}
	}

	log.Debug("approximate matching done", "new_classes", len(classes)-len(exactGroups), "matched_units", len(approxMatched))

	return classes
}

func groupByHash(units []model.NormalizedUnit) map[[32]byte][]int {
	groups := make(map[[32]byte][]int)
	for i, u := range units {
		groups[u.TokenHash] = append(groups[u.TokenHash], i)
	}
	return groups
}

// unionFind for clustering matched pairs.
type unionFind struct {
	parent []int
	rank   []int
}

func newUnionFind(n int) *unionFind {
	parent := make([]int, n)
	rank := make([]int, n)
	for i := range parent {
		parent[i] = i
	}
	return &unionFind{parent: parent, rank: rank}
}

func (uf *unionFind) find(x int) int {
	for uf.parent[x] != x {
		uf.parent[x] = uf.parent[uf.parent[x]]
		x = uf.parent[x]
	}
	return x
}

func (uf *unionFind) union(x, y int) {
	rx, ry := uf.find(x), uf.find(y)
	if rx == ry {
		return
	}
	if uf.rank[rx] < uf.rank[ry] {
		rx, ry = ry, rx
	}
	uf.parent[ry] = rx
	if uf.rank[rx] == uf.rank[ry] {
		uf.rank[rx]++
	}
}
