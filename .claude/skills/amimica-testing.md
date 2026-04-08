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
make check          # fmt + vet + test
```

## Testing the scan pipeline

```bash
# Scan fixtures
make run ARGS="scan testdata/fixtures/exact_clones/"

# JSON output
make run ARGS="scan --output json testdata/fixtures/exact_clones/"

# Scan with debug logging
make run ARGS="scan --debug ."

# Scan with different normalization
make run ARGS="scan --norm-level light ."

# Exclude tests, raise threshold
make run ARGS="scan --exclude-tests --min-score 0.5 ."
```

## Test file locations

- Unit tests: `internal/<pkg>/<file>_test.go` — alongside the source
- Golden tests: `testdata/golden/<scenario>/expected.json` (planned)
- Fixtures: `testdata/fixtures/<scenario>/` — purpose-built Go source files
- Regressions: `testdata/regressions/<id>/` — known FP/FN cases (planned)

## Fixture conventions

Each fixture directory is a valid Go package:
```
testdata/fixtures/exact_clones/
  a.go    # package clones — function ProcessA
  b.go    # package clones — function ProcessB (identical body)
```

Fixtures should be minimal — only enough code to exercise the detection case.

When adding new fixtures, also verify:
```bash
make run ARGS="scan testdata/fixtures/<new_fixture>/"
```

## Table-driven test pattern

```go
func TestNormalize(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  string
    }{
        {name: "empty", input: "", want: ""},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := normalize(tt.input)
            if got != tt.want {
                t.Errorf("got %q, want %q", got, tt.want)
            }
        })
    }
}
```

## Coverage target

80% line coverage. Check with `make test-cover`.

Critical packages that MUST have high coverage:
- `internal/normalize/` — every normalization rule tested
- `internal/fingerprint/` — hash determinism, shingle correctness
- `internal/match/` — matching correctness
- `internal/fsguard/` — security-critical
- `internal/config/` — all validation rules

## Benchmark conventions

```go
func BenchmarkComputeShingles(b *testing.B) {
    tokens := makeTestTokens(100)
    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ComputeShingles(tokens, 7)
    }
}
```
