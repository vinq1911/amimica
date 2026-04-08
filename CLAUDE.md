# Amimica — Project Instructions

## What is this

Amimica is a production-grade Go code clone detection CLI tool and MCP server. It scans Go codebases and identifies repetitive code patterns using AST normalization, fingerprinting, and similarity analysis.

## Tech stack

- **Language**: Go (1.25+, targeting 1.26 features when available)
- **Dependencies**: Standard library first. Only third-party dep is `gopkg.in/yaml.v3` for config.
- **Build**: `make build` (see `make help` for all targets)
- **Test**: `make test` (race-enabled)
- **Lint**: `make lint` (requires golangci-lint)

## Repository layout

```
cmd/amimica/           CLI entrypoint (flag-based subcommands, no cobra)
internal/
  model/               Shared data types (dependency-free, no imports from other internal/)
  config/              YAML config loading, env overrides, validation
  logging/             Thin slog wrapper
  fsguard/             Path sandboxing, symlink policy, file size limits
  discovery/           File walking, filtering, go.mod awareness
  parser/              Go AST parsing with error tolerance
  normalize/           AST normalization at 4 levels (raw/light/strong/semantic)
  extract/             Unit extraction (functions, blocks, windows, subtrees)
  fingerprint/         Hashing, shingles, MinHash, LSH
  match/               Candidate generation, exact/approximate matching
  score/               Scoring model, ranking, noise filtering
  engine/              Pipeline orchestration — the main Analyze() entrypoint
  explain/             Finding explanation and diff generation
  report/              Output formatting (text, JSON, SARIF, markdown)
  cache/               Content-hash cache, invalidation
  mcp/                 MCP server, tool handlers, session state
  app/                 CLI command implementations
testdata/
  fixtures/            Purpose-built Go code exercising clone patterns
  golden/              Expected outputs for golden tests
  regressions/         Known FP/FN regression cases
```

## Architecture principles

- **model/ is dependency-free**: It imports nothing from other internal/ packages. All shared types live here.
- **engine/ orchestrates**: Both CLI (`app/`) and MCP (`mcp/`) call `engine.Analyze()`. No business logic in CLI or MCP layers.
- **Interfaces only where needed**: Concrete types preferred. Interfaces only for `discovery.Walker`, `cache.Store`, `report.Formatter`.
- **No premature abstraction**: Don't add layers until there's a second consumer.
- **Deterministic**: Same input + same config = same output. No randomness. Findings have stable IDs.

## Code style

- Use `slog` for structured logging (via `internal/logging`). No `log.Println`.
- Wrap errors with `fmt.Errorf("context: %w", err)`. Always add context.
- Use `context.Context` for cancellation where appropriate.
- No `panic` in library code. Return errors.
- Small files. One primary type per file. Tests in `_test.go` alongside the code.
- Table-driven tests preferred. Use `testdata/` for fixtures.
- Document exported types with Go doc comments.

## Common commands

```bash
make help          # Show all available targets
make build         # Build binary to bin/amimica
make test          # Run tests with race detection
make test-v        # Verbose test output
make test-cover    # Coverage report
make vet           # Go vet
make lint          # golangci-lint (install: https://golangci-lint.run)
make fmt           # Format code
make check         # fmt + vet + test
make doctor        # Check dev environment
make run ARGS="version"   # Build and run with args
make tidy          # go mod tidy + verify
make clean         # Remove build artifacts
```

## Testing approach

- Unit tests: Every package gets `_test.go`. 80% line coverage target.
- Golden tests: `testdata/golden/` — run pipeline on fixtures, compare to expected JSON.
- Fixture repos: `testdata/fixtures/` — purpose-built Go code for clone scenarios.
- Benchmarks: `_test.go` with `Benchmark*` functions for hot paths.
- Run with `-race` always.

## Key design decisions

1. **Syntax-only analysis (no go/types in v1)**: Faster, works on non-building code, catches Type-1/2/3 clones.
2. **stdlib `flag` for CLI**: No cobra dependency. Subcommand dispatch is manual.
3. **Four normalization levels**: NormRaw → NormLight → NormStrong → NormSemantic. Each progressively abstracts more.
4. **Layered matching**: Exact hash → token shingles → MinHash/LSH → structural distance. Cheapest first.
5. **Scoring model**: Weighted composite of confidence, similarity, impact, refactorability. Configurable.

## What NOT to do

- Don't add new third-party dependencies without strong justification.
- Don't put business logic in `cmd/`, `internal/app/`, or `internal/mcp/`. Those are thin wiring layers.
- Don't use `interface{}` or `any` for typed data. Use the model types.
- Don't skip tests. Every new function gets a test.
- Don't use `go generate` for anything critical-path.

## Reference

- Full engineering plan: [PLAN.md](./PLAN.md)
- Planning state: [.planning/STATE.md](.planning/STATE.md)
