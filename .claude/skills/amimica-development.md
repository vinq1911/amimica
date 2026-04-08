---
name: amimica-development
description: Core development patterns and conventions for the Amimica clone detection tool
triggers:
  - writing Go code in this project
  - creating new packages or files
  - implementing analysis pipeline components
---

# Amimica Development Skill

## Package creation checklist

When creating a new file in `internal/`:

1. Add a doc comment on the `package` declaration explaining the package's purpose
2. Define the primary type first, then its methods, then helpers
3. Create `_test.go` alongside with at least one test
4. If the package needs types from other internal packages, import only from `internal/model/`
5. Do not create interfaces unless there are 2+ implementations or testing requires substitution

## Implemented packages

| Package | Status | Key types/functions |
|---|---|---|
| `model/` | Done | Finding, NormalizedUnit, Score, Evidence, NormToken, FindingID |
| `config/` | Done | Config, Load(), ApplyEnv(), Validate(), Default() |
| `logging/` | Done | Setup(level, format) → *slog.Logger |
| `fsguard/` | Done | Guard, ValidatePath(), ValidateSymlink(), ValidateFileSize() |
| `discovery/` | Done | Walk(roots, cfg, log) → []SourceFile |
| `parser/` | Done | ParseFile(), ParseFiles() → []*ParsedFile |
| `normalize/` | Done | Normalizer.NormalizeFunc(), NormalizeBlock(), NormalizeStmts() |
| `extract/` | Done | Extract(parsedFile, cfg, level) → []NormalizedUnit |
| `fingerprint/` | Done | ComputeShingles(), ComputeMinHash(), LSHIndex |
| `match/` | Done | FindClones(units, cfg, log) → []CloneClass |
| `score/` | Done | ScoreFindings(classes, units, files, cfg) → []Finding |
| `report/` | Done | WriteText(), WriteJSON() |
| `engine/` | Done | Analyze(roots, cfg, log) → *Result |
| `app/` | Done | RunScan(args) → exit code |
| `explain/` | Planned | Finding explanation and diff generation |
| `cache/` | Planned | Content-hash incremental cache |
| `mcp/` | Planned | MCP server and tool handlers |

## Model types — JSON tags required

All model types that appear in output MUST have json tags. Enum types (CloneType, NormalizationLevel, RefactorCategory) MUST have MarshalJSON methods that serialize to strings.

## Error handling

```go
// DO: Wrap with context
return fmt.Errorf("normalize function %s: %w", fn.Name, err)

// DON'T: Bare return
return err
```

## Logging

Use `slog` via the logger from `internal/logging`:
```go
log.Debug("parsing file", "path", path, "size", size)
log.Warn("skipping file", "path", path, "reason", "exceeds max size")
```

Never use `fmt.Println` or `log.Println` for operational output. Logs go to stderr; results go to stdout.

## Normalization levels

| Level | Transforms | Identifiers | Literals | Function names |
|---|---|---|---|---|
| `NormRaw` (0) | Strip comments/whitespace | Keep | Keep | Keep |
| `NormLight` (1) | + Replace literals | Keep | `$STR`, `$INT`, `$FLOAT`, `$RUNE` | Keep |
| `NormStrong` (2) | + Replace identifiers | `$V0`, `$P0`, `$R` | Placeholders | `$FUNC` |
| `NormSemantic` (3) | + Abstract selectors/types | Placeholders | Placeholders | `$FUNC` |

At NormStrong: function names → `$FUNC`, local vars → `$V0`/`$V1`, params → `$P0`/`$P1`, receivers → `$R`. Selector method names are preserved (e.g., `.Find()` stays `.Find()`).

## Pipeline order

```
Discovery → Parser → Normalizer → Extractor → Fingerprinter → Matcher → Scorer → Reporter
```

The `engine` package orchestrates this. Individual packages should not reach across the pipeline.

## Matching layers (cheapest first)

1. **Exact hash**: SHA-256 of normalized tokens. Groups by hash. O(n).
2. **Shingles**: 7-token n-grams per unit.
3. **MinHash**: 128-function signatures for approximate Jaccard estimation.
4. **LSH**: 16-band locality-sensitive hashing index. Finds candidate pairs.
5. **Jaccard verification**: Exact Jaccard on shingle sets. Threshold ≥ 0.6.
6. **Union-find**: Clusters verified pairs into clone classes.

Important: Skip pairs from the same function (overlapping windows aren't clones).

## Scoring

Composite = weighted(confidence, similarity, impact, refactorability, repetition) × penalties.
Penalties: test code 0.5x, generated 0.3x, small regions 0.7x, 2-member classes 0.9x.
