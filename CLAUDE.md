# Amimica — Project Instructions

## What is this

Amimica is a production-grade multi-language code clone detection CLI tool and MCP server. It scans Go, JavaScript/TypeScript, and Ruby codebases to identify repetitive code patterns using tokenization, normalization, fingerprinting, and similarity analysis.

## Tech stack

- **Language**: Go 1.25+
- **Dependencies**: Standard library first. Only third-party dep is `gopkg.in/yaml.v3`.
- **Build**: `make build` — see `make help` for all targets
- **Test**: `make test` — race-enabled
- **Supported languages**: Go, JavaScript, TypeScript, JSX, TSX, Ruby

## Repository layout

```
cmd/amimica/              CLI entrypoint (flag-based subcommands, no cobra)
internal/
  model/                  Shared types — dependency-free, imports nothing from internal/
  config/                 YAML config loading, env overrides, validation
  logging/                Thin slog wrapper
  fsguard/                Path sandboxing, symlink policy, file size limits
  lang/                   Language abstraction: interface + registry
    golang/               Go language: wraps parser/normalize/extract
    javascript/           JS/TS/JSX/TSX: built-in tokenizer + normalizer
    ruby/                 Ruby: built-in tokenizer + normalizer
  discovery/              Multi-language file walking with filtering
  parser/                 Go AST parsing (used by lang/golang)
  normalize/              Go AST normalization (used by lang/golang)
  extract/                Go unit extraction (used by lang/golang)
  fingerprint/            SHA-256, shingles, MinHash, LSH (language-agnostic)
  match/                  Exact + approximate matching (language-agnostic)
  score/                  Scoring, penalties, refactor hints (language-agnostic)
  engine/                 Pipeline orchestration — Analyze() entrypoint
  report/                 Text + JSON output formatting
  app/                    CLI scan command
testdata/fixtures/        Clone fixtures: exact_clones/, js_clones/, rb_clones/
```

## Architecture

```
Discovery → [Language.ParseAndExtract] → Fingerprinter → Matcher → Scorer → Reporter
            ^^^^^^^^^^^^^^^^^^^^^^^^^^^^
            Per-language (Go/JS/Ruby)
                                         ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                                         Language-agnostic shared pipeline
```

### Language abstraction (`internal/lang/`)

Each language implements the `lang.Language` interface:
```go
type Language interface {
    Name() string
    Extensions() []string
    ParseAndExtract(sf SourceFile, cfg *Config, level NormLevel, log *slog.Logger) ([]NormalizedUnit, error)
    IsTestFile(path string) bool
    IsGeneratedFile(content []byte) bool
}
```

To add a new language: create `internal/lang/<name>/`, implement the interface, register in `engine.DefaultRegistry()`.

## Adding a new language

1. Create `internal/lang/<name>/<name>.go`
2. Implement `lang.Language` interface — at minimum: tokenizer, normalizer, function finder
3. Register in `internal/engine/engine.go` `DefaultRegistry()`
4. Add test fixtures in `testdata/fixtures/<name>_clones/`
5. Add extensions to default config `paths.include`

## Code style

- `slog` for logging. No `log.Println`.
- Wrap errors: `fmt.Errorf("context: %w", err)`.
- No `panic` in library code.
- JSON tags on all model types in output. `MarshalJSON` on enums.
- Table-driven tests with `testdata/` fixtures.

## Common commands

```bash
make help                    # Show all targets
make build                   # Build to bin/amimica
make run ARGS="scan ."       # Build and scan
make test                    # Tests with race detection
make check                   # fmt + vet + test
make doctor                  # Check environment
```

## Reference

- Full engineering plan: PLAN.md (not tracked in git)
- Planning state: .planning/ (not tracked in git)
