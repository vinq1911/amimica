// Package fingerprint computes hash-based fingerprints for normalized code units.
// It provides exact hashing, token shingle computation, MinHash signatures,
// and a locality-sensitive hashing (LSH) index for approximate matching.
package fingerprint

import (
	"encoding/binary"
	"hash/fnv"
	"math"

	"github.com/vinq1911/amimica/internal/model"
)

// ComputeShingles computes n-gram hashes over a normalized token sequence.
// Each shingle is the FNV-1a hash of n consecutive (Kind, Norm) pairs.
func ComputeShingles(tokens []model.NormToken, n int) []uint64 {
	if len(tokens) < n {
		return nil
	}
	shingles := make([]uint64, 0, len(tokens)-n+1)
	for i := 0; i <= len(tokens)-n; i++ {
		h := fnv.New64a()
		for j := i; j < i+n; j++ {
			_ = binary.Write(h, binary.BigEndian, int32(tokens[j].Kind))
			h.Write([]byte(tokens[j].Norm))
		}
		shingles = append(shingles, h.Sum64())
	}
	return shingles
}

// ComputeMinHash computes a MinHash signature from a set of shingle hashes.
// numFuncs is the number of hash functions (signature length).
func ComputeMinHash(shingles []uint64, numFuncs int) []uint32 {
	sig := make([]uint32, numFuncs)
	for i := range sig {
		sig[i] = math.MaxUint32
	}
	for _, s := range shingles {
		for i := 0; i < numFuncs; i++ {
			h := murmurMix(s, uint32(i))
			if h < sig[i] {
				sig[i] = h
			}
		}
	}
	return sig
}

// murmurMix is a fast hash mixing function seeded with a value.
func murmurMix(val uint64, seed uint32) uint32 {
	h := uint32(val) ^ seed
	h ^= uint32(val >> 32)
	h *= 0xcc9e2d51
	h = (h << 15) | (h >> 17)
	h *= 0x1b873593
	h ^= seed
	h = (h << 13) | (h >> 19)
	h = h*5 + 0xe6546b64
	return h
}

// EstimateJaccard estimates the Jaccard similarity between two MinHash signatures.
func EstimateJaccard(a, b []uint32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	matches := 0
	for i := range a {
		if a[i] == b[i] {
			matches++
		}
	}
	return float64(matches) / float64(len(a))
}

// JaccardShingles computes exact Jaccard similarity between two shingle sets.
func JaccardShingles(a, b []uint64) float64 {
	if len(a) == 0 && len(b) == 0 {
		return 0
	}
	setA := make(map[uint64]struct{}, len(a))
	for _, s := range a {
		setA[s] = struct{}{}
	}
	intersection := 0
	setB := make(map[uint64]struct{}, len(b))
	for _, s := range b {
		setB[s] = struct{}{}
		if _, ok := setA[s]; ok {
			intersection++
		}
	}
	// Union = |A| + |B| - |intersection|
	union := len(setA) + len(setB) - intersection
	if union == 0 {
		return 0
	}
	return float64(intersection) / float64(union)
}

// LSHIndex is a locality-sensitive hashing index for approximate nearest-neighbor
// queries on MinHash signatures.
type LSHIndex struct {
	bands   int
	rows    int
	buckets []map[uint64][]int // one bucket map per band, maps band hash → unit indices
}

// NewLSHIndex creates a new LSH index with the given band and row configuration.
// bands * rows must equal the MinHash signature length.
func NewLSHIndex(bands, rows int) *LSHIndex {
	buckets := make([]map[uint64][]int, bands)
	for i := range buckets {
		buckets[i] = make(map[uint64][]int)
	}
	return &LSHIndex{bands: bands, rows: rows, buckets: buckets}
}

// Insert adds a unit's MinHash signature to the index.
func (idx *LSHIndex) Insert(unitIdx int, sig []uint32) {
	for band := 0; band < idx.bands; band++ {
		h := fnv.New64a()
		start := band * idx.rows
		end := start + idx.rows
		if end > len(sig) {
			end = len(sig)
		}
		for i := start; i < end; i++ {
			_ = binary.Write(h, binary.BigEndian, sig[i])
		}
		bucket := h.Sum64()
		idx.buckets[band][bucket] = append(idx.buckets[band][bucket], unitIdx)
	}
}

// QueryCandidates returns indices of units that share at least one LSH bucket
// with the given signature (excluding the query unit itself).
func (idx *LSHIndex) QueryCandidates(sig []uint32, selfIdx int) []int {
	seen := make(map[int]bool)
	var candidates []int
	for band := 0; band < idx.bands; band++ {
		h := fnv.New64a()
		start := band * idx.rows
		end := start + idx.rows
		if end > len(sig) {
			end = len(sig)
		}
		for i := start; i < end; i++ {
			_ = binary.Write(h, binary.BigEndian, sig[i])
		}
		bucket := h.Sum64()
		for _, idx := range idx.buckets[band][bucket] {
			if idx != selfIdx && !seen[idx] {
				seen[idx] = true
				candidates = append(candidates, idx)
			}
		}
	}
	return candidates
}
