---
name: amimica-testing
description: Testing conventions and fixture management for Amimica
triggers:
  - writing tests
  - creating test fixtures
  - running tests
  - checking coverage
---

# Amimica Testing Skill

## Running tests

```bash
make test           # All tests, race-enabled
make test-v         # Verbose output
make test-cover     # With coverage report
make bench          # Benchmarks
```

## Test file locations

- Unit tests: `internal/<pkg>/<file>_test.go` — alongside the source
- Golden tests: `testdata/golden/<scenario>/expected.json`
- Fixtures: `testdata/fixtures/<scenario>/` — purpose-built Go source files
- Regressions: `testdata/regressions/<id>/` — known FP/FN cases with `want.json`

## Fixture conventions

Each fixture directory is a valid Go package:
```
testdata/fixtures/exact_clones/
  a.go    # package clones — one function
  b.go    # package clones — identical function with different name
```

Fixtures should be minimal — only enough code to exercise the detection case.

## Golden test pattern

```go
func TestGolden(t *testing.T) {
    entries, err := os.ReadDir("testdata/golden")
    if err != nil {
        t.Fatal(err)
    }
    for _, e := range entries {
        if !e.IsDir() { continue }
        t.Run(e.Name(), func(t *testing.T) {
            // Run engine.Analyze() on corresponding fixture
            // Compare output to expected.json
            // Use go-cmp or manual comparison
        })
    }
}
```

Update golden files: `go test -run TestGolden -update-golden`

## Benchmark conventions

```go
func BenchmarkNormalizeFunction(b *testing.B) {
    // Setup: parse a representative function once
    // b.ResetTimer()
    for i := 0; i < b.N; i++ {
        // Call the function under test
    }
}
```

Name benchmarks after the operation: `BenchmarkComputeShingles`, `BenchmarkLSHQuery`, etc.

## Coverage target

80% line coverage. Check with `make test-cover`.

Packages that MUST have high coverage:
- `internal/normalize/` — every normalization rule tested
- `internal/fingerprint/` — hash determinism, shingle correctness
- `internal/match/` — matching correctness
- `internal/fsguard/` — security-critical
