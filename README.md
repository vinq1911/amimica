# Amimica

Deterministic, offline code clone detection for Go codebases.

Amimica scans Go repositories and identifies repetitive code patterns — from exact copy-paste clones to structurally similar but renamed fragments. It reports findings with enough evidence for a human to decide whether and how to refactor.

## Quick start

```bash
# Build
make build

# Scan a directory
./bin/amimica scan .

# Scan specific paths
./bin/amimica scan ./internal/handlers ./internal/services

# JSON output
./bin/amimica scan --output json .

# Exclude test files, raise threshold
./bin/amimica scan --exclude-tests --min-score 0.5 .
```

## Example output

```
Amimica Clone Detection Report
══════════════════════════════════════════
Scanned: 30 files, 69 functions, 228 units
Time: 23ms

Found 3 clone classes

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#1   Score: 0.42  │  Renamed  │  2 regions
     ID: F-eebadcb54e
      handlers/user.go:11-21  (ProcessA)
      handlers/product.go:11-21  (ProcessB)
     Similarity: 100% (exact_hash)
     Refactor: Extract a shared helper function.
```

## What it detects

| Clone type | Description | How |
|---|---|---|
| **Exact** (Type-1) | Identical code ignoring whitespace/comments | Normalized hash equality |
| **Renamed** (Type-2) | Same structure, different identifiers/literals | Strong-normalized hash equality |
| **Near-duplicate** (Type-3) | Structurally similar with small differences | Token shingle similarity + MinHash/LSH |
| **Repeated patterns** | Recurring handler/service/repo scaffolding | Normalized block fingerprint clustering |

## How it works

```
Discovery → Parser → Normalizer → Extractor → Fingerprinter → Matcher → Scorer → Reporter
```

1. **Discovery** finds Go files, applies include/exclude patterns, detects test/generated files
2. **Parser** builds ASTs using `go/parser` with error tolerance
3. **Normalizer** transforms ASTs into normalized token sequences at 4 levels:
   - **Raw**: Strip comments/whitespace
   - **Light**: Replace literals with `$STR`, `$INT`, `$FLOAT`
   - **Strong**: Replace local identifiers with positional `$V0`, `$P0`, `$R`
   - **Semantic**: Abstract selectors and type names
4. **Extractor** segments into analysis units (functions, statement windows)
5. **Fingerprinter** computes hashes, token shingles, MinHash signatures
6. **Matcher** groups exact matches by hash, then uses LSH for approximate matches
7. **Scorer** ranks findings by confidence, similarity, impact, refactorability
8. **Reporter** formats output as text or JSON

## CLI reference

```
amimica <command> [flags]

Commands:
  scan          Run clone detection analysis
  version       Print version and build info
```

### `amimica scan`

```
amimica scan [paths...] [flags]

Flags:
  --config <path>         Config file path
  --output <format>       Output format: text, json (default: text)
  --output-file <path>    Write to file instead of stdout
  --min-score <float>     Minimum score to report (default: 0.15)
  --min-lines <int>       Minimum lines per region (default: 6)
  --min-statements <int>  Minimum statements per unit (default: 3)
  --norm-level <level>    Normalization: raw, light, strong, semantic (default: strong)
  --exclude-tests         Exclude test files entirely
  --thorough              Enable deeper analysis (slower)
  --max-findings <int>    Maximum findings (default: 100)
  --no-cache              Disable caching
  --debug                 Enable debug logging

Exit codes:
  0  No findings above threshold
  1  Findings detected
  2  Configuration error
  3  Analysis error
```

## Configuration

Create `.amimica.yaml` in your project root (or `~/.config/amimica/config.yaml`):

```yaml
version: 1
analysis:
  normalization_level: strong
  min_statements: 3
  min_lines: 6
  window_size: 5
scoring:
  min_score: 0.15
  max_findings: 100
paths:
  exclude:
    - "vendor/**"
    - "**/*.pb.go"
    - "**/mock_*.go"
  include_tests: true
```

Environment variables override config: `AMIMICA_ANALYSIS_NORMALIZATION_LEVEL=semantic`

## Requirements

- Go 1.25+
- golangci-lint (optional, for `make lint`)

## Development

```bash
make help          # Show all targets
make build         # Build binary
make test          # Tests with race detection
make test-cover    # Coverage report
make check         # fmt + vet + test
make doctor        # Check dev environment
make run ARGS="scan ."  # Build and run
```

## Project status

**Active development.** The core scan pipeline is working. Upcoming:

- [x] File discovery with filtering
- [x] Go AST parsing with error tolerance
- [x] 4-level normalization (raw/light/strong/semantic)
- [x] Function and window extraction
- [x] Exact hash matching
- [x] Approximate matching (MinHash/LSH)
- [x] Scoring with penalties and noise suppression
- [x] Text and JSON output
- [ ] Explain command (detailed finding breakdown)
- [ ] Diff command (ad-hoc region comparison)
- [ ] SARIF output for CI
- [ ] MCP server for editor integration
- [ ] Content-hash caching for incremental scans
- [ ] Advanced heuristics (autocorrelation, rolling hash)

## License

TBD
