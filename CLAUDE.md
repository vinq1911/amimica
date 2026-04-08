# Amimica — Project Instructions

## What is this

Amimica is a production-grade Go code clone detection CLI tool and MCP server. It scans Go codebases and identifies repetitive code patterns using AST normalization, fingerprinting, and similarity analysis.

## Tech stack

- **Language**: Go (1.25+)
- **Dependencies**: Standard library first. Only third-party dep is `gopkg.in/yaml.v3` for config.
- **Build**: `make build` (see `make help` for all targets)
- **Test**: `make test` (race-enabled)
- **Lint**: `make lint` (requires golangci-lint)

## Repository layout

```
cmd/amimica/           CLI entrypoint (flag-based subcommands, no cobra)
internal/
  model/               Shared data types — dependency-free, imports nothing from internal/
  config/              YAML config loading, env overrides, validation
  logging/             Thin slog wrapper
  fsguard/             Path sandboxing, symlink policy, file size limits
  discovery/           File walking, filtering, generated/test file detection
  parser/              Go AST parsing with error tolerance (partial ASTs)
  normalize/           AST normalization at 4 levels (raw/light/strong/semantic)
  extract/             Unit extraction: functions + sliding statement windows
  fingerprint/         SHA-256 hashing, token shingles, MinHash, LSH index
  match/               Exact hash grouping + approximate LSH matching + union-find clustering
  score/               Weighted composite scoring, penalties, noise suppression, refactor hints
  engine/              Pipeline orchestration — Analyze() is the single entrypoint
  report/              Output formatting (text, JSON; SARIF and markdown planned)
  explain/             Finding explanation and diff generation (planned)
  cache/               Content-hash cache for incremental scans (planned)
  mcp/                 MCP server, tool handlers, session state (planned)
  app/                 CLI command implementations (scan is working)
testdata/
  fixtures/            Purpose-built Go code exercising clone patterns
  golden/              Expected outputs for golden tests (planned)
  regressions/         Known FP/FN regression cases (planned)
```

## Architecture

```
Discovery → Parser → Normalizer → Extractor → Fingerprinter → Matcher → Scorer → Reporter
```

- **model/ is dependency-free**: Imports nothing from other internal/ packages.
- **engine/ orchestrates**: Both CLI (`app/`) and MCP (`mcp/`) call `engine.Analyze()`.
- **Thin CLI/MCP layers**: No business logic in `cmd/`, `app/`, or `mcp/`.
- **Deterministic**: Same input + same config = same output. Findings have stable IDs.

## Normalization levels

The core of the clone detection engine. Each level includes all transformations from lower levels.

| Level | What it normalizes | Example |
|---|---|---|
| **Raw** (0) | Comments, whitespace | `x := 42` → `x := 42` |
| **Light** (1) | + Literals | `x := 42` → `x := $INT` |
| **Strong** (2) | + Identifiers (positional) | `x := 42` → `$V0 := $INT` |
| **Semantic** (3) | + Selectors, types | `s.repo.Find()` → `$SEL.Find()` |

At NormStrong, function names become `$FUNC`, local vars become `$V0`/`$V1`, params become `$P0`/`$P1`, receivers become `$R`.

## Matching layers

Cheapest first, progressively more expensive:

1. **Exact hash**: Group units by SHA-256 of normalized tokens. O(n).
2. **Token shingles**: 7-token n-gram hashes per unit.
3. **MinHash + LSH**: 128-function MinHash signatures, 16-band LSH index. Finds approximate matches.
4. **Jaccard verification**: Exact Jaccard similarity for LSH candidate pairs. Threshold: 0.6.
5. **Union-find clustering**: Groups verified pairs into clone classes.

## Scoring

Composite score = weighted sum of confidence, similarity, impact, refactorability, repetition. Multiplicative penalties for test code (0.5x), generated code (0.3x), small regions (0.7x). Noise suppression below min_score threshold.

## Code style

- Use `slog` for structured logging. No `log.Println` or `fmt.Println` for operational output.
- Wrap errors: `fmt.Errorf("context: %w", err)`. Always add context.
- No `panic` in library code. Return errors.
- Small files. One primary type per file.
- Table-driven tests. Use `testdata/` for fixtures.
- JSON tags on all model types that appear in output.
- `MarshalJSON` on enum types to serialize as strings.

## Common commands

```bash
make help                    # Show all targets
make build                   # Build to bin/amimica
make run ARGS="scan ."       # Build and scan
make run ARGS="scan --output json ."  # JSON output
make test                    # Tests with race detection
make test-cover              # Coverage report
make check                   # fmt + vet + test
make doctor                  # Check environment
make tidy                    # go mod tidy + verify
```

## What NOT to do

- Don't add third-party deps without strong justification.
- Don't put analysis logic in `cmd/`, `app/`, or `mcp/`. Those are thin wiring.
- Don't use `interface{}` or `any` for typed data. Use model types.
- Don't skip tests. Every new function gets a test.
- Don't forget json tags on model types.

## Reference

- Full engineering plan: [PLAN.md](./PLAN.md)
- Planning state: [.planning/STATE.md](.planning/STATE.md)
