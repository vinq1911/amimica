# Amimica

Deterministic, offline code clone detection for Go codebases.

Amimica scans Go repositories and identifies repetitive code patterns — from exact copy-paste clones to structurally similar but renamed fragments. It reports findings with enough evidence for a human to decide whether and how to refactor.

## Status

**Early development.** Core infrastructure is in place; analysis pipeline is being built.

## Quick start

```bash
# Check your environment
make doctor

# Build
make build

# Run
./bin/amimica version
./bin/amimica scan .          # not yet implemented
```

## Features (planned)

- **Clone detection**: Exact clones, renamed clones, near-duplicates, repeated patterns
- **AST-based analysis**: Uses Go's own parser for accurate structural comparison
- **Four normalization levels**: From raw syntax to fully abstracted structural patterns
- **Layered matching**: Exact hash, token shingles, MinHash/LSH, structural distance
- **Scoring and ranking**: Confidence, similarity, impact, refactorability
- **Explainability**: Every finding includes evidence — what matched, what differed, why
- **Refactoring hints**: Suggests categories like helper extraction, generics, table-driven
- **Multiple outputs**: Terminal text, JSON, SARIF (CI), Markdown
- **MCP server**: Editor/agent integration via Model Context Protocol
- **Incremental caching**: Content-hash based, fast re-scans

## Requirements

- Go 1.25+ (targeting Go 1.26)
- golangci-lint (optional, for `make lint`)

## Development

```bash
make help          # Show all available targets
make build         # Build binary
make test          # Run tests (race-enabled)
make test-cover    # Coverage report
make check         # fmt + vet + test
make doctor        # Check dev environment
make clean         # Remove artifacts
```

## Architecture

```
Discovery → Parser → Normalizer → Extractor → Fingerprinter → Matcher → Scorer → Reporter
```

The analysis pipeline is orchestrated by `internal/engine/`. Both the CLI and MCP server are thin layers over this engine.

See [PLAN.md](./PLAN.md) for the full engineering plan.

## License

TBD
