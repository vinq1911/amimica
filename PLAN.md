# Amimica: Engineering Plan

## Go Code Clone Detection CLI & MCP Server

**Version**: v1 Plan
**Language**: Go 1.26
**Date**: 2026-04-07

---

# Table of Contents

1. [Product Definition](#1-product-definition)
2. [Architecture](#2-architecture)
3. [Package Layout](#3-package-layout)
4. [Data Model](#4-data-model)
5. [Algorithms](#5-algorithms)
6. [CLI UX](#6-cli-ux)
7. [MCP Server Design](#7-mcp-server-design)
8. [Repository Structure](#8-repository-structure)
9. [Configuration Model](#9-configuration-model)
10. [Reporting and Output Formats](#10-reporting-and-output-formats)
11. [Testing Strategy](#11-testing-strategy)
12. [Performance Strategy](#12-performance-strategy)
13. [Security and Safety](#13-security-and-safety)
14. [Phased Implementation Roadmap](#14-phased-implementation-roadmap)
15. [Risks and Tradeoffs](#15-risks-and-tradeoffs)
16. [Future Extensions](#16-future-extensions)
17. [Non-goals for v1](#17-non-goals-for-v1)
18. [False Positive / False Negative Taxonomy](#18-false-positive--false-negative-taxonomy)
19. [Deterministic Finding ID Design](#19-deterministic-finding-id-design)
20. [Recommended Default Thresholds](#20-recommended-default-thresholds)
21. [How to Evolve Toward Multi-Language Support Later](#21-how-to-evolve-toward-multi-language-support-later)
22. [How to Keep the MCP Layer Thin Over the Core Engine](#22-how-to-keep-the-mcp-layer-thin-over-the-core-engine)
23. [Open Design Questions Needing Early Validation](#23-open-design-questions-needing-early-validation)
24. [Final Repo Tree](#24-final-repo-tree)
25. [Configuration File Schema](#25-configuration-file-schema)
26. [Finding JSON Schema](#26-finding-json-schema)
27. [First 10-Task Execution Backlog](#27-first-10-task-execution-backlog)

---

# 1. Product Definition

## What Amimica Is

Amimica is a deterministic, offline, production-grade code clone detection tool for Go codebases. It finds repetitive code patterns — from exact copy-paste clones to structurally similar but renamed fragments — and reports them with enough evidence for a human to decide whether to refactor.

It ships as:
- A **standalone CLI** for developer workstations and CI pipelines
- An **MCP server** exposing analysis tools for editor/agent integration

## What It Detects

| Clone Type | Description | Detection Method |
|---|---|---|
| Type-1 (Exact) | Identical code ignoring whitespace/comments | Normalized hash equality |
| Type-2 (Renamed) | Identical structure, different identifiers/literals | Strongly-normalized hash equality |
| Type-3 (Near-duplicate) | Structurally similar with insertions/deletions/modifications | Token shingle similarity + structural distance |
| Repeated patterns | Recurring handler/service/repository scaffolding | Normalized block fingerprint clustering |
| Copy-paste drift | Shared shape with localized divergence | Shingle overlap + diff analysis |
| Structural idioms | Repeated switch/if trees, error-handling shapes | AST subtree hashing |

## What It Does NOT Do (v1)

- Semantic equivalence detection (different algorithms, same result)
- Cross-language detection
- Auto-refactoring or code generation
- Data-flow or control-flow graph analysis
- Type-4 clone detection (semantically equivalent but structurally different)

## Key Properties

- **Deterministic**: Same input always produces same output. No randomness. Findings have stable IDs.
- **Offline**: No network access during analysis. All computation local.
- **Explainable**: Every finding includes evidence: what matched, what differed, at what normalization level.
- **Incremental-ready**: Content-hash based caching for re-scans.
- **Production-grade**: Bounded memory, graceful degradation on malformed files, structured logging, exit codes.

---

# 2. Architecture

## High-Level Pipeline

```
                                                        ┌────────────┐
                                                        │  CLI / MCP │
                                                        │  Interface  │
                                                        └─────┬──────┘
                                                              │
                                                              ▼
┌──────────┐    ┌─────────┐    ┌────────────┐    ┌──────────────────┐
│ Discovery │───▶│ Parser  │───▶│ Normalizer │───▶│    Extractor     │
│           │    │         │    │            │    │ (func/block/win) │
└──────────┘    └─────────┘    └────────────┘    └────────┬─────────┘
                                                          │
                                                          ▼
                                              ┌──────────────────────┐
                                              │   Fingerprinter      │
                                              │ (hash, shingle, LSH) │
                                              └────────┬─────────────┘
                                                       │
                                                       ▼
                                              ┌──────────────────────┐
                                              │   Candidate Matcher  │
                                              │  (exact + approx)    │
                                              └────────┬─────────────┘
                                                       │
                                                       ▼
                                              ┌──────────────────────┐
                                              │   Scorer / Ranker    │
                                              └────────┬─────────────┘
                                                       │
                                                       ▼
                                              ┌──────────────────────┐
                                              │    Reporter          │
                                              └──────────────────────┘
```

## Data Flow

1. **Discovery** walks the filesystem, applies include/exclude rules, yields file paths.
2. **Parser** parses each Go file into `*ast.File` with `go/parser`. Collects token-level data via `go/token.FileSet`. Tolerates parse errors per-file.
3. **Normalizer** transforms AST nodes into normalized representations at multiple levels. Produces `NormalizedUnit` values.
4. **Extractor** segments normalized ASTs into analysis units: whole functions, statement windows, inner blocks.
5. **Fingerprinter** computes hashes and shingle sets for each unit.
6. **Candidate Matcher** groups exact matches by hash, then uses LSH/MinHash to find approximate matches. Expensive pairwise comparison only for shortlisted candidates.
7. **Scorer** assigns confidence, impact, refactorability scores. Filters noise.
8. **Reporter** emits findings in requested format(s).

## Concurrency Model

- Discovery is single-goroutine (filesystem walk is I/O bound, parallelism adds complexity for little gain).
- Parsing is parallelized: a worker pool of `GOMAXPROCS` goroutines reads files from a channel. Each file parsed independently.
- Normalization and extraction run inline with parsing (same goroutine, per-file).
- Fingerprinting can be parallelized per-unit if needed but likely runs fast enough inline.
- Candidate matching is the most CPU-intensive phase. Exact matching (hash grouping) is O(n) in a map. Approximate matching uses LSH index construction (parallelizable) followed by candidate-pair verification (parallelizable).
- Scoring and reporting are single-goroutine.

Work is partitioned by **file** through parsing/normalization/extraction, then by **unit** through fingerprinting, then the matcher works on the full index.

## Dependency Direction

```
cmd/amimica  ──▶  internal/app  ──▶  internal/engine
                  internal/mcp  ──▶  internal/engine
                                     internal/engine  ──▶  internal/discovery
                                                           internal/parser
                                                           internal/normalize
                                                           internal/extract
                                                           internal/fingerprint
                                                           internal/match
                                                           internal/score
                                     internal/report  (consumed by app and mcp)
                                     internal/cache
                                     internal/config
```

No circular dependencies. The `engine` package orchestrates the pipeline. `app` and `mcp` are thin shells over `engine`.

---

# 3. Package Layout

```
amimica/
├── cmd/
│   └── amimica/          # CLI entrypoint: main.go, ~50 lines
├── internal/
│   ├── app/              # CLI command implementations (scan, report, explain, diff, serve)
│   ├── config/           # Configuration loading, validation, defaults
│   ├── discovery/        # File walking, filtering, go.mod awareness
│   ├── parser/           # Go AST parsing, error tolerance, file-level metadata
│   ├── normalize/        # AST normalization at multiple levels
│   ├── extract/          # Unit extraction (functions, blocks, windows)
│   ├── fingerprint/      # Hashing, shingles, MinHash, LSH
│   ├── match/            # Candidate generation, exact/approximate matching
│   ├── score/            # Scoring model, ranking, noise filtering
│   ├── engine/           # Pipeline orchestration, the main Analyze() entrypoint
│   ├── report/           # Output formatting (text, JSON, SARIF, markdown)
│   ├── explain/          # Finding explanation and diff generation
│   ├── cache/            # Content-hash cache, invalidation
│   ├── mcp/              # MCP server, tool handlers, session state
│   ├── model/            # Shared data types: Finding, Unit, Region, Score, etc.
│   ├── fsguard/          # Path sandboxing, symlink policy, size limits
│   └── logging/          # Structured logger setup (slog wrapper)
└── testdata/             # Fixture repositories for testing
```

### Why This Layout

| Package | Rationale |
|---|---|
| `model/` | Shared types used across packages. Avoids import cycles. Kept dependency-free. |
| `engine/` | Single orchestration point. Both CLI and MCP call `engine.Analyze()`. |
| `app/` | CLI wiring only. No business logic. Thin. |
| `mcp/` | MCP protocol handling only. Translates MCP requests to engine calls. |
| `explain/` | Separate from `report/` because explanation logic (diffing, annotation) is complex enough to warrant isolation. |
| `fsguard/` | Security-sensitive code isolated for review. Used by discovery. |
| `logging/` | Thin wrapper. Sets up `slog` with appropriate handler. No custom logger abstraction. |

### What Is NOT a Separate Package

- No `pkg/api/` — no public Go API in v1. Everything is `internal/`.
- No `internal/util/` — utility functions go into the package that uses them, or into `model/` if truly shared.
- No separate `internal/hash/` — hashing logic lives in `fingerprint/` since that's its only consumer.
- No `internal/ast/` — normalization and extraction operate on `go/ast` types directly. Wrapping them adds indirection without value.

### Interfaces

Interfaces are defined only where there are multiple real implementations or where testing requires substitution:

| Interface | Location | Implementations |
|---|---|---|
| `discovery.Walker` | `discovery/walker.go` | `OSWalker` (real), `MemWalker` (test) |
| `cache.Store` | `cache/store.go` | `DirStore` (filesystem), `NullStore` (disabled) |
| `report.Formatter` | `report/formatter.go` | `TextFormatter`, `JSONFormatter`, `SARIFFormatter`, `MarkdownFormatter` |
| `engine.Analyzer` | NOT an interface | Concrete struct. No need for interface — one implementation, tested directly. |

---

# 4. Data Model

All types live in `internal/model/` unless tightly coupled to a single package.

## Core Types

### `SourceFile`

```go
type SourceFile struct {
    Path        string          // Absolute path
    RelPath     string          // Relative to scan root
    Package     string          // Go package name
    Module      string          // Go module path (from nearest go.mod)
    ContentHash [32]byte        // SHA-256 of file content
    Size        int64           // File size in bytes
    IsTest      bool            // *_test.go
    IsGenerated bool            // Contains "Code generated" marker
    ParseErrors []ParseError    // Non-fatal parse errors
}
```

### `NormalizedUnit`

```go
type NormalizedUnit struct {
    ID          UnitID              // Deterministic, content-derived
    Source      SourceRegion        // Where in the source this came from
    Kind        UnitKind            // Function, Block, Window, Subtree
    RawTokens   []Token            // Original token sequence
    NormTokens  []NormToken        // Normalized token sequence
    NormLevel   NormalizationLevel // Which normalization was applied
    ASTHash     [32]byte           // Hash of normalized AST subtree
    TokenHash   [32]byte           // Hash of normalized token sequence
    Shingles    []uint64           // Token n-gram hashes
    MinHash     []uint32           // MinHash signature (computed lazily)
    StmtCount   int                // Number of statements
    NodeCount   int                // Number of AST nodes
    Complexity  int                // Cyclomatic complexity estimate
}
```

### `SourceRegion`

```go
type SourceRegion struct {
    File      string  // Relative file path
    StartLine int
    EndLine   int
    StartCol  int
    EndCol    int
    FuncName  string  // Enclosing function, if any
    Receiver  string  // Receiver type, if method
}
```

### `UnitKind`

```go
type UnitKind int

const (
    UnitFunction UnitKind = iota  // Entire function/method body
    UnitBlock                      // If/for/switch/select block
    UnitWindow                     // Sliding window of N statements
    UnitSubtree                    // Arbitrary AST subtree
)
```

### `NormalizationLevel`

```go
type NormalizationLevel int

const (
    NormRaw       NormalizationLevel = iota // No normalization (whitespace/comment stripped only)
    NormLight                                // Literals replaced, formatting canonicalized
    NormStrong                               // Identifiers replaced with positional placeholders
    NormSemantic                             // Receiver/param types considered, selector chains abstracted
)
```

### `Finding`

```go
type Finding struct {
    ID              FindingID           // Deterministic hash-based ID
    CloneClassID    string              // Groups all members of the same clone class
    Type            CloneType           // Exact, Renamed, NearDuplicate, Pattern
    Regions         []SourceRegion      // All code regions in this clone class
    NormLevel       NormalizationLevel  // Level at which match was detected
    Score           Score
    Evidence        Evidence
    RefactorHints   []RefactorHint
    Suppressed      bool                // Filtered by noise rules
    SuppressReason  string
}
```

### `Score`

```go
type Score struct {
    Confidence      float64 // 0.0-1.0: how certain this is a real clone
    Similarity      float64 // 0.0-1.0: structural similarity
    Impact          float64 // 0.0-1.0: how much code is affected
    Refactorability float64 // 0.0-1.0: how likely this can be refactored
    Composite       float64 // Weighted combination, used for ranking
    Penalties       []Penalty
}

type Penalty struct {
    Reason string
    Factor float64 // Multiplicative penalty, e.g., 0.5 for test code
}
```

### `Evidence`

```go
type Evidence struct {
    MatchedNormForm   string          // The normalized code that matched
    Diffs             []RegionDiff    // Per-pair diffs showing what diverged
    SharedTokens      int             // How many tokens are shared
    TotalTokens       int             // Total tokens across regions
    SimilarityMetric  string          // Which metric produced the match
    SimilarityValue   float64
    ContributingNodes []string        // AST node types that drove similarity
}

type RegionDiff struct {
    RegionA   SourceRegion
    RegionB   SourceRegion
    Hunks     []DiffHunk
}

type DiffHunk struct {
    LineA   int
    LineB   int
    Content string // Unified diff fragment
}
```

### `RefactorHint`

```go
type RefactorHint struct {
    Category    RefactorCategory
    Description string
    Confidence  float64
}

type RefactorCategory int

const (
    RefactorExtractHelper RefactorCategory = iota
    RefactorGenericFunc
    RefactorTableDriven
    RefactorSharedValidator
    RefactorAdapterMapper
    RefactorInterfaceExtract
    RefactorConfigDriven
)
```

### `Token` and `NormToken`

```go
type Token struct {
    Kind  token.Token // Go token type
    Lit   string      // Literal value
    Pos   token.Pos
}

type NormToken struct {
    Kind    token.Token   // Preserved
    Norm    string        // Normalized literal: "$ID0", "$LIT", "$STR", etc.
    OrigLit string        // Original literal for explanation
}
```

### `FindingID`

```go
// FindingID is a deterministic identifier derived from:
// - Sorted list of source regions (file + line range)
// - Clone type
// - Normalization level
// This means the same code in the same locations always produces the same FindingID
// across runs, enabling suppression, tracking, and diffing.
type FindingID [20]byte // SHA-1 truncated, hex-encoded for display
```

---

# 5. Algorithms

## 5.1 File Discovery

### Algorithm

```
1. Accept one or more root paths.
2. For each root:
   a. Locate nearest go.mod to determine module boundary.
   b. Walk directory tree using filepath.WalkDir (not filepath.Walk — WalkDir is more efficient).
   c. For each entry:
      - Skip if matches exclude patterns (glob-based, e.g., "vendor/**", "*.pb.go").
      - Skip symlinks by default (configurable).
      - Skip if file size exceeds max limit (default 1MB).
      - Skip directories named "testdata" unless configured otherwise.
      - Record if file is *_test.go.
      - Record if file contains "Code generated" comment in first 4 lines.
      - Check build tags only if configured (adds cost).
   d. Yield SourceFile descriptors.
```

### go.mod Boundary Handling

Walk upward from each root to find the nearest `go.mod`. Files outside any module are still scanned but get `Module: ""`. The tool does NOT use `go list` — it operates on raw filesystem, not the Go build system. This means it works on incomplete or non-building code.

### Vendor Handling

`vendor/` directories are excluded by default. Configurable via `include_vendor: true`.

### Generated File Detection

Scan first 2048 bytes of each file for the canonical marker:
```
// Code generated .* DO NOT EDIT.
```
Use a compiled regex. Files with this marker get `IsGenerated: true` and receive a scoring penalty but are still analyzed (user may want to find clones in generated code).

## 5.2 Parsing

### Approach

Use `go/parser.ParseFile` with mode `parser.ParseComments | parser.AllErrors`.

```go
fset := token.NewFileSet()
f, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.AllErrors)
// f may be non-nil even when err != nil (partial parse)
// Record errors in SourceFile.ParseErrors but continue with partial AST
```

### Error Tolerance

Go's parser returns partial ASTs on error. We keep all successfully parsed declarations. Files that fail completely (nil AST) are logged and skipped.

### Type Information Decision

**v1 uses syntax-only analysis.** No `go/types`.

Rationale:
- `go/types` requires a complete, buildable package graph. Many monorepos have partial builds.
- Type checking is slow and memory-intensive.
- Syntax-level analysis catches Type-1, Type-2, and most Type-3 clones without type info.
- Type info would help for semantic normalization (e.g., knowing two selectors resolve to the same type), but this is deferred to v2.

Tradeoff: Without types, `foo.Bar()` and `baz.Bar()` look similar at the syntax level even if they call completely different methods. The strong normalization level handles this by abstracting selectors to `$SELECTOR.$METHOD`, which is the right thing for clone detection (structural similarity).

### What We Extract from Parsing

Per file:
- `*ast.File` — the full AST
- Token stream from `go/scanner` for shingle computation
- File-level metadata (package, imports, function declarations)

## 5.3 Normalization

Normalization is the most critical part of the system. It determines what "looks the same" means.

### Normalization Levels

#### Level 0: `NormRaw`

Strip comments and canonicalize whitespace. Preserve everything else.

Used for: Type-1 (exact) clone detection.

Example:
```go
// Before
func processOrder(order *Order) error {
    // validate the order
    if order == nil {
        return errors.New("nil order")
    }
    return order.Save()
}

// After NormRaw
func processOrder(order *Order) error {
    if order == nil {
        return errors.New("nil order")
    }
    return order.Save()
}
```

#### Level 1: `NormLight`

Everything in NormRaw, plus:
- All string literals → `$STR`
- All integer literals → `$INT`
- All float literals → `$FLOAT`
- All rune literals → `$RUNE`
- Import aliases removed (imports themselves not part of function bodies)
- Formatting canonicalized (single space between tokens, standardized newlines)

Used for: Detecting clones that differ only in literal values.

Example:
```go
// Before
if resp.StatusCode != 200 {
    return fmt.Errorf("unexpected status: %d", resp.StatusCode)
}

// After NormLight
if resp.StatusCode != $INT {
    return fmt.Errorf($STR, resp.StatusCode)
}
```

#### Level 2: `NormStrong`

Everything in NormLight, plus:
- Local variable names → positional placeholders: `$V0`, `$V1`, `$V2`, ...
  Assigned in order of first appearance within the unit.
- Parameter names → `$P0`, `$P1`, `$P2`, ...
- Receiver name → `$R`
- Function/method being defined keeps its name (for matching across packages).
  But within function bodies, called function names are kept as-is.
- Anonymous function names → `$ANON`
- Label names → `$LABEL0`, `$LABEL1`, ...
- Short variable declarations `:=` treated same as `var` declarations for naming purposes.

Used for: Type-2 (renamed) clone detection.

Example:
```go
// Before (Function A)
func (s *UserService) GetUser(ctx context.Context, id string) (*User, error) {
    user, err := s.repo.FindByID(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("get user: %w", err)
    }
    return user, nil
}

// Before (Function B)
func (r *ProductRepo) FetchProduct(c context.Context, pid string) (*Product, error) {
    product, fetchErr := r.store.FindByID(c, pid)
    if fetchErr != nil {
        return nil, fmt.Errorf("fetch product: %w", fetchErr)
    }
    return product, nil
}

// After NormStrong (both become):
func ($R *$T) $FUNCNAME($P0 context.Context, $P1 string) (*$T1, error) {
    $V0, $V1 := $R.$FIELD.FindByID($P0, $P1)
    if $V1 != nil {
        return nil, fmt.Errorf($STR, $V1)
    }
    return $V0, nil
}
```

Note: `$T`, `$T1`, `$FIELD` are type/field placeholders. In v1 (syntax-only), we normalize:
- Receiver type → `$T` (since we don't have type info, we just use a placeholder)
- Selector base → preserve if it's a parameter/receiver reference, else `$SEL`
- Method name in selectors → preserved (FindByID stays FindByID)

**Identifier numbering algorithm**:
```
counter := 0
seenIdents := map[string]string{}
for each identifier in AST walk (pre-order):
    if isLocal(ident) && !seenIdents[ident.Name]:
        seenIdents[ident.Name] = fmt.Sprintf("$V%d", counter)
        counter++
    replace ident with seenIdents[ident.Name] (or keep if not local)
```

Where `isLocal` means: defined within the function body (not package-level, not imported).

#### Level 3: `NormSemantic` (partially implemented in v1)

Everything in NormStrong, plus:
- Selector expressions normalized: `x.Method()` → `$SEL.$METHOD()`
  (all selector bases become `$SEL` unless they're a parameter or receiver)
- Type names in composite literals → `$TYPE{}`
- Channel operations normalized: `ch <- v` → `$CHAN <- $V`
- Generic type parameters → `$TPARAM0`, `$TPARAM1`

This level aggressively abstracts, useful for finding repeated patterns across very different domains.

### Normalization Implementation Strategy

Normalization operates on the AST, not on text. The normalizer walks `ast.Node` trees and produces a new sequence of `NormToken` values.

```go
// normalize/normalizer.go

type Normalizer struct {
    level NormalizationLevel
    fset  *token.FileSet
}

func (n *Normalizer) NormalizeFunc(fn *ast.FuncDecl) []NormToken {
    // 1. Walk AST in pre-order
    // 2. For each node, emit NormToken(s) according to level
    // 3. Track identifier bindings for renaming
    // 4. Return token sequence
}
```

The normalizer does NOT modify the original AST. It produces a new token stream.

### Specific Normalization Rules

| Construct | NormLight | NormStrong | NormSemantic |
|---|---|---|---|
| `x := 42` | `x := $INT` | `$V0 := $INT` | `$V0 := $INT` |
| `"hello"` | `$STR` | `$STR` | `$STR` |
| `err != nil` | `err != nil` | `$V1 != nil` | `$V1 != nil` |
| `s.repo.Find(ctx, id)` | `s.repo.Find(ctx, id)` | `$R.$FIELD.Find($P0, $P1)` | `$SEL.Find($P0, $P1)` |
| `User{Name: n}` | `User{Name: n}` | `User{Name: $V0}` | `$TYPE{$FIELD: $V0}` |
| `for _, item := range items` | `for _, item := range items` | `for _, $V0 := range $V1` | `for _, $V0 := range $V1` |
| `switch x.(type)` | `switch x.(type)` | `switch $V0.(type)` | `switch $V0.(type)` |
| `func(a int) { ... }` | `func(a int) { ... }` | `func($P0 int) { ... }` | `func($P0 $TYPE) { ... }` |
| `go func() { ... }()` | `go func() { ... }()` | `go func() { ... }()` | `go func() { ... }()` |
| `T[K comparable]` | `T[K comparable]` | `T[$TPARAM0 comparable]` | `T[$TPARAM0 $CONSTRAINT]` |
| Import aliases | Stripped | Stripped | Stripped |
| Comments | Stripped | Stripped | Stripped |
| Blank identifier `_` | `_` | `_` | `_` |

### Error Handling Pattern Normalization

Error handling is the most duplicated pattern in Go. Special handling:

```go
// These two should match at NormStrong:

// Version A
val, err := doThing(ctx)
if err != nil {
    return nil, fmt.Errorf("doing thing: %w", err)
}

// Version B
result, e := performAction(c)
if e != nil {
    return nil, fmt.Errorf("performing action: %w", e)
}

// Both normalize to:
$V0, $V1 := $CALL($ARGS)
if $V1 != nil {
    return nil, fmt.Errorf($STR, $V1)
}
```

This is handled naturally by the NormStrong rules — no special case needed.

## 5.4 Extraction Granularity

### Function/Method Extraction

Extract the body of every `*ast.FuncDecl` and `*ast.FuncLit` (anonymous functions).

- Minimum size: 3 statements (configurable). Functions shorter than this are noise.
- The function signature is included in the normalized form (helps detect renamed clones of whole functions).
- Methods include receiver info.

**Tradeoff**: Function-level extraction is the most natural unit for Go. It misses clones that span parts of two functions or repeated blocks within a function.

### Statement Window Extraction (Sliding Window)

Slide a window of `W` consecutive statements (default W=5) through each function body, advancing by 1 statement.

For a function with 20 statements, this produces 16 windows of size 5.

- Captures repeated statement sequences that don't align with function boundaries.
- The main source of repeated error-handling detection.
- Windows are extracted at the `[]ast.Stmt` level from block statements.

**Tradeoff**: Generates many more units than function extraction (O(statements) vs O(functions)). This is the primary scalability concern. Mitigated by:
- Only extracting windows from functions above a minimum size (default: 8 statements)
- Not extracting windows from generated files
- Configurable window sizes

### Inner Block Extraction

Extract bodies of:
- `if` blocks (including `else` branches as separate units)
- `for` / `range` loop bodies
- `switch` / `select` case bodies (each case as a unit, plus the whole switch)
- Nested function literals

Minimum size: 2 statements.

**Tradeoff**: Many blocks are tiny (1-2 statements). The minimum size filter is critical.

### AST Subtree Extraction

For each function, extract every `ast.BlockStmt` subtree that contains at least `MinSubtreeNodes` nodes (default: 15).

This catches structural patterns that don't align with statement boundaries, like repeated composite literal constructions or nested control flow.

**Tradeoff**: Highest unit count. Only enabled when the user opts into thorough scanning mode.

### Extraction Summary

| Granularity | Default | Units per typical file | Main use case |
|---|---|---|---|
| Function | Always on | 5-20 | Whole-function clones |
| Window (W=5) | On | 20-200 | Repeated statement sequences |
| Block | On | 10-50 | Repeated control flow |
| Subtree | Off (--thorough) | 50-500 | Deep structural patterns |

## 5.5 Fingerprinting and Similarity

### Layer 1: Exact Hash Matching

For each `NormalizedUnit`, compute SHA-256 of the normalized token sequence (at each normalization level).

```go
func HashTokens(tokens []NormToken) [32]byte {
    h := sha256.New()
    for _, t := range tokens {
        binary.Write(h, binary.BigEndian, t.Kind)
        h.Write([]byte(t.Norm))
        h.Write([]byte{0}) // separator
    }
    return [32]byte(h.Sum(nil))
}
```

Units with identical hashes at a given normalization level are exact clones at that level.

**When this runs**: First. O(n) to build hash → []UnitID map. Grouping takes O(n) with a map.

**Why**: Cheapest operation. Catches all Type-1 and Type-2 clones.

### Layer 2: Token Shingle Fingerprinting

Compute n-gram (shingle) hashes over the normalized token sequence.

```go
func ComputeShingles(tokens []NormToken, n int) []uint64 {
    if len(tokens) < n {
        return nil
    }
    shingles := make([]uint64, 0, len(tokens)-n+1)
    for i := 0; i <= len(tokens)-n; i++ {
        h := fnv.New64a()
        for j := i; j < i+n; j++ {
            binary.Write(h, binary.BigEndian, tokens[j].Kind)
            h.Write([]byte(tokens[j].Norm))
        }
        shingles = append(shingles, h.Sum64())
    }
    return shingles
}
```

Default shingle size: `n=7` tokens. This balances specificity and recall.

**When this runs**: After hash grouping. For all units not yet matched exactly.

### Layer 3: MinHash + LSH for Approximate Matching

Compute MinHash signatures from shingle sets:

```go
const NumHashFunctions = 128

func ComputeMinHash(shingles []uint64) [NumHashFunctions]uint32 {
    var sig [NumHashFunctions]uint32
    for i := range sig {
        sig[i] = math.MaxUint32
    }
    for _, s := range shingles {
        for i := 0; i < NumHashFunctions; i++ {
            h := murmurHash(s, uint32(i)) // seeded hash
            if h < sig[i] {
                sig[i] = h
            }
        }
    }
    return sig
}
```

Then build an LSH index with `b` bands of `r` rows each (b*r = 128). Default: b=16, r=8, giving approximate Jaccard threshold of ~0.5.

```go
type LSHIndex struct {
    bands    int
    rows     int
    buckets  []map[uint64][]UnitID  // one bucket map per band
}

func (idx *LSHIndex) Insert(id UnitID, sig [NumHashFunctions]uint32) {
    for band := 0; band < idx.bands; band++ {
        h := fnv.New64a()
        for row := 0; row < idx.rows; row++ {
            binary.Write(h, binary.BigEndian, sig[band*idx.rows+row])
        }
        bucket := h.Sum64()
        idx.buckets[band][bucket] = append(idx.buckets[band][bucket], id)
    }
}

func (idx *LSHIndex) QueryCandidates(sig [NumHashFunctions]uint32) []UnitID {
    seen := map[UnitID]bool{}
    var candidates []UnitID
    for band := 0; band < idx.bands; band++ {
        h := fnv.New64a()
        for row := 0; row < idx.rows; row++ {
            binary.Write(h, binary.BigEndian, sig[band*idx.rows+row])
        }
        bucket := h.Sum64()
        for _, id := range idx.buckets[band][bucket] {
            if !seen[id] {
                seen[id] = true
                candidates = append(candidates, id)
            }
        }
    }
    return candidates
}
```

**When this runs**: After exact matching. Builds index from all unmatched units, then queries each unit for candidates.

**Why LSH over brute-force pairwise**: For N=100,000 units, brute-force is O(N^2) = 10^10 comparisons. LSH reduces to O(N * average_candidates), typically O(N * 100).

### Layer 4: Structural Distance for Shortlisted Candidates

For candidate pairs from LSH, compute actual Jaccard similarity from shingle sets:

```go
func JaccardSimilarity(a, b []uint64) float64 {
    setA := toSet(a)
    intersection, union := 0, len(setA)
    for _, s := range b {
        if setA[s] {
            intersection++
        } else {
            union++
        }
    }
    return float64(intersection) / float64(union)
}
```

If Jaccard > threshold (default 0.6), compute a more expensive structural comparison:

**Token-level edit distance** (Myers diff on normalized token sequences). This is O(N*D) where D is the edit distance, fast for similar sequences.

This produces the actual diff hunks used in evidence.

**When this runs**: Only for candidate pairs above the LSH threshold. Typically <1% of all pairs.

### Layer 5: Rolling Hash for Subsequence Detection

Use a rolling hash (Rabin-Karp style) over the normalized token stream to detect repeated subsequences that may not align with statement boundaries.

```go
const (
    rollingBase   = 257
    rollingMod    = 1_000_000_007
    rollingWindow = 50 // tokens
)
```

This catches embedded repetition: a 50-token pattern that appears in multiple different functions. It is complementary to the window-based extraction.

**When this runs**: Optionally, during the `--thorough` mode. Applied to the concatenated token stream of all functions in a package.

### Clustering

After matching, group findings into **clone classes**:

1. Start with pairwise matches (from exact grouping + LSH-verified pairs).
2. Build a graph: nodes = units, edges = matched pairs with similarity.
3. Find connected components (each component = a clone class).
4. For components with many members, verify all-pairs similarity is above a minimum threshold. If not, split into subclusters using single-linkage clustering.

## 5.6 Auxiliary Repetition Heuristics (Fourier-Inspired / Autocorrelation)

**This section describes an optional, secondary signal. It does NOT drive clone detection decisions alone.**

### Concept

Convert a normalized token stream to a numeric sequence:

```go
func TokensToSequence(tokens []NormToken) []float64 {
    seq := make([]float64, len(tokens))
    for i, t := range tokens {
        // Map each distinct NormToken to a unique integer
        // Use a consistent hash: FNV-1a of (Kind, Norm)
        h := fnv.New32a()
        binary.Write(h, binary.BigEndian, t.Kind)
        h.Write([]byte(t.Norm))
        seq[i] = float64(h.Sum32())
    }
    return seq
}
```

### Autocorrelation

Compute autocorrelation of this sequence at various lag values:

```go
func Autocorrelation(seq []float64, maxLag int) []float64 {
    n := len(seq)
    mean := mean(seq)
    variance := variance(seq, mean)
    result := make([]float64, maxLag)
    for lag := 1; lag <= maxLag; lag++ {
        sum := 0.0
        for i := 0; i < n-lag; i++ {
            sum += (seq[i] - mean) * (seq[i+lag] - mean)
        }
        result[lag-1] = sum / (float64(n-lag) * variance)
    }
    return result
}
```

Peaks in the autocorrelation at lag `L` suggest a repeating pattern of period `L` tokens.

### What This Surfaces

- **Periodic repetition within a single file**: e.g., a handler file where every 30 lines is a similar HTTP handler.
- **Characteristic frequencies**: A package with many similar functions will have autocorrelation peaks at the function-length scale.
- **Copy-paste cadence**: Regular spacing between similar blocks.

### Limitations

- High autocorrelation is necessary but not sufficient for clones. Regular code structure (e.g., consistent function lengths) can produce peaks.
- Does not identify WHICH code is repeated, only THAT repetition exists at a given period.
- Sensitive to file ordering and concatenation.
- Computational cost: O(N * maxLag) per file.

### Integration

The autocorrelation signal is used only as:
1. A **file-level triaging heuristic**: Files with high autocorrelation peaks are prioritized for deeper analysis.
2. A **validation signal**: If clone detection finds many clones in a file AND autocorrelation confirms periodicity at the expected clone size, confidence increases.
3. A **reporting annotation**: "This file exhibits periodic repetition at ~35-token intervals" as auxiliary evidence.

It is NOT used as:
- A primary similarity metric between code regions
- A filter to exclude files from analysis
- A standalone finding source

## 5.7 Scoring and Ranking

### Scoring Model

Each finding receives a composite score from weighted components:

```go
type ScoringWeights struct {
    Similarity      float64 // default 0.30
    Impact          float64 // default 0.25
    Refactorability float64 // default 0.20
    Repetition      float64 // default 0.15
    Confidence      float64 // default 0.10
}
```

#### Confidence

How certain we are this is a real clone (not a false positive).

- Exact hash match at NormRaw: 1.0
- Exact hash match at NormLight: 0.95
- Exact hash match at NormStrong: 0.85
- LSH candidate with Jaccard > 0.8: 0.80
- LSH candidate with Jaccard > 0.6: 0.65
- Rolling hash subsequence match: 0.50

#### Similarity

Token-level Jaccard similarity of normalized forms. Directly from the matching step.

#### Impact

```
impact = (total_lines_across_all_regions) / (max_function_lines_in_repo)
```

Capped at 1.0. More lines of duplicate code = higher impact.

Also consider:
- Number of regions in the clone class (more = higher impact)
- Number of distinct packages affected (cross-package clones are higher impact)

#### Refactorability

Heuristic based on:
- Are all regions in the same package? (easier to refactor) → +0.2
- Are parameter signatures similar? → +0.2
- Is the difference only in literals? (table-driven candidate) → +0.3
- Is the difference only in a selector base? (interface extraction candidate) → +0.2
- Are regions in test files? → -0.3 (test duplication is often acceptable)

#### Repetition Count

```
repetition_factor = min(1.0, num_regions / 10.0)
```

More copies → more incentive to fix.

### Penalties

| Condition | Penalty Factor |
|---|---|
| Both regions in `*_test.go` | 0.5 |
| Both regions in generated files | 0.3 |
| Region < 5 statements | 0.7 |
| Region is pure error handling | 0.6 |
| Clone class has exactly 2 members | 0.9 |
| Regions are in the same function | 0.8 |

### Noise Suppression Rules

Suppress findings (set `Suppressed: true`) when:
- Composite score < `min_score` (default 0.15)
- Region is a single `if err != nil { return err }` block (too common to be useful)
- All regions are interface method stubs (empty or single-return methods)
- All regions are in `vendor/` or generated code (if user didn't opt in)
- Total duplicated lines < `min_lines` (default 6)

### Sorting

Findings are sorted by `Score.Composite` descending. Ties broken by total line count.

## 5.8 Explainability

### Explain Model

Every finding carries `Evidence` that answers:

1. **What matched?** — The normalized form that was identical/similar across regions.
2. **What differed?** — A unified diff between each pair of regions.
3. **At what level?** — The normalization level at which the match was detected.
4. **Why is it a clone?** — The similarity metric and its value.
5. **What are the contributing factors?** — Which AST node types drove similarity (e.g., "matching if-chains", "matching range loops").
6. **What could be done?** — Refactor hint category and description.

### Diff Generation

For each pair of regions in a clone class:

```go
func GenerateDiff(unitA, unitB NormalizedUnit, fset *token.FileSet, fileContents map[string][]byte) RegionDiff {
    // 1. Extract raw source text for both regions
    // 2. Run Myers diff algorithm on the lines
    // 3. Annotate diff hunks with which lines are structurally similar vs. different
    // 4. Return structured diff
}
```

The diff is computed on **raw source** (not normalized), so the user sees the actual code differences.

### Normalized Form Display

For the `explain` command, show the normalized form that caused the match:

```
Finding F-a1b2c3:
  Type: Renamed Clone (NormStrong)
  Regions:
    - api/handlers/user.go:24-35 (func GetUser)
    - api/handlers/product.go:18-29 (func GetProduct)

  Normalized form (both reduce to):
    $V0, $V1 := $R.$FIELD.Find($P0, $P1)
    if $V1 != nil {
        return nil, fmt.Errorf($STR, $V1)
    }
    if $V0 == nil {
        return nil, $PKG.ErrNotFound
    }
    return $V0, nil

  Key differences:
    - Variable names: user/product, err/fetchErr
    - String literal: "get user: %w" / "fetch product: %w"
    - Receiver field: s.repo / r.store

  Refactor hint: Extract helper function with generic type parameter
    func findByID[T any](ctx context.Context, finder interface{ FindByID(context.Context, string) (*T, error) }, id string, errMsg string) (*T, error)
```

## 5.9 Persistence and Caching

### Content-Hash Caching

Each `SourceFile` has a `ContentHash`. When re-scanning a repository:

1. Load previous scan's cache index: `{file_path → content_hash → []NormalizedUnit}`.
2. For each file in the current scan:
   - Compute content hash.
   - If hash matches cache, reuse the cached units. Skip parsing + normalization.
   - If hash differs, parse and normalize. Update cache.
3. Matching/scoring always runs on the full unit set (cached + fresh).

### Cache Storage

Cache is stored in a directory (default: `.amimica/cache/` relative to scan root, configurable).

Structure:
```
.amimica/cache/
├── index.json          # {relpath: contenthash} mapping
├── units/
│   ├── <contenthash>.gob  # Serialized []NormalizedUnit
│   └── ...
└── meta.json           # Cache format version, amimica version
```

Format: Go `encoding/gob` for speed. JSON would be 3-5x slower to load for large caches.

### Invalidation

- If any configuration changes that affect normalization (level, window size, etc.), the entire cache is invalidated.
- Configuration hash is stored in `meta.json` and compared on load.
- If amimica version changes (minor or major), cache is invalidated.

### Deterministic Finding IDs

Finding IDs must be stable across runs with the same codebase. See [Section 19](#19-deterministic-finding-id-design).

---

# 6. CLI UX

## Command Overview

```
amimica <command> [flags]

Commands:
  scan          Run clone detection analysis
  report        Re-format a previous scan's results
  explain       Show detailed explanation of a specific finding
  diff          Show diff between two code regions
  serve-mcp     Start MCP server for editor/agent integration
  version       Print version and build info
```

## `amimica scan`

```
amimica scan [paths...] [flags]

Flags:
  --config <path>         Config file path (default: .amimica.yaml, then ~/.config/amimica/config.yaml)
  --output <format>       Output format: text, json, sarif, markdown (default: text)
  --output-file <path>    Write output to file instead of stdout
  --min-score <float>     Minimum composite score to report (default: 0.15)
  --min-lines <int>       Minimum lines in a region to consider (default: 6)
  --min-statements <int>  Minimum statements in a unit (default: 3)
  --norm-level <level>    Max normalization level: raw, light, strong, semantic (default: strong)
  --window-size <int>     Statement window size (default: 5)
  --include <glob>        Include file patterns (repeatable)
  --exclude <glob>        Exclude file patterns (repeatable)
  --include-tests         Include test files (default: test files included with penalty)
  --exclude-tests         Exclude test files entirely
  --include-generated     Include generated files without penalty
  --include-vendor        Include vendor directory
  --thorough              Enable subtree extraction and rolling hash (slower)
  --jobs <int>            Parallelism level (default: GOMAXPROCS)
  --cache-dir <path>      Cache directory (default: .amimica/cache/)
  --no-cache              Disable caching
  --max-findings <int>    Maximum findings to report (default: 100)
  --sort <field>          Sort by: score, lines, regions, file (default: score)
  --debug                 Enable debug logging
  --trace                 Enable trace logging (very verbose)
  --profile <path>        Write CPU profile to file
  --memprofile <path>     Write memory profile to file

Exit Codes:
  0    Success, no findings above threshold
  1    Success, findings above threshold reported
  2    Configuration or argument error
  3    Analysis error (partial results may exist)
  4    Fatal error
```

### Example Invocations

```bash
# Scan current directory
amimica scan .

# Scan specific packages, output JSON
amimica scan ./internal/handlers ./internal/services --output json --output-file clones.json

# CI mode: fail if high-severity clones found
amimica scan . --min-score 0.7 --output sarif --output-file results.sarif

# Thorough analysis with custom config
amimica scan . --config .amimica.yaml --thorough --max-findings 500

# Quick scan excluding tests
amimica scan . --exclude-tests --min-lines 10

# Debug mode
amimica scan . --debug 2>debug.log
```

### Terminal Output (text format)

```
Amimica Clone Detection Report
═══════════════════════════════
Scanned: 342 files, 48 packages, 1847 functions
Time: 2.3s | Cache: 298 hits, 44 misses

Found 23 clone classes (67 regions)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
#1  Score: 0.94  │  Renamed Clone  │  5 regions  │  45 lines each
    internal/handlers/user.go:24-68
    internal/handlers/product.go:18-62
    internal/handlers/order.go:31-75
    internal/handlers/payment.go:22-66
    internal/handlers/invoice.go:15-59
    ↳ All reduce to identical CRUD handler pattern
    ↳ Refactor: Extract generic handler with type parameter

#2  Score: 0.87  │  Near-Duplicate  │  3 regions  │  28-31 lines
    internal/repo/user_repo.go:45-72
    internal/repo/product_repo.go:52-82
    internal/repo/order_repo.go:39-69
    ↳ 89% token similarity after strong normalization
    ↳ Refactor: Extract shared repository base

...

Summary:
  High (>0.8):    4 clone classes, 18 regions
  Medium (0.5-0.8): 8 clone classes, 24 regions
  Low (<0.5):     11 clone classes, 25 regions
```

## `amimica explain`

```
amimica explain <finding-id> [flags]

Flags:
  --scan-result <path>    Path to scan result file (default: latest .amimica/results/)
  --show-normalized       Show normalized forms
  --show-diff             Show diffs between regions
  --show-all              Show everything
  --output <format>       text, json, markdown (default: text)
```

## `amimica diff`

```
amimica diff <file:line-line> <file:line-line> [flags]

Flags:
  --norm-level <level>    Normalization level for comparison
  --output <format>       text, json (default: text)
```

Example:
```bash
amimica diff internal/handlers/user.go:24-68 internal/handlers/product.go:18-62
```

## `amimica report`

```
amimica report [flags]

Flags:
  --scan-result <path>    Path to scan result (default: latest)
  --output <format>       text, json, sarif, markdown
  --output-file <path>    Write to file
  --min-score <float>     Filter by minimum score
  --sort <field>          Sort order
  --max-findings <int>    Limit output
```

## `amimica serve-mcp`

```
amimica serve-mcp [flags]

Flags:
  --transport <type>      stdio (default), sse, streamable-http
  --port <int>            Port for HTTP transports (default: 9100)
  --sandbox <path>        Restrict filesystem access to this directory
  --max-file-size <size>  Maximum file size to analyze (default: 1MB)
  --cache-dir <path>      Cache directory
  --config <path>         Config file path
  --log-file <path>       Log to file (default: stderr)
```

## Config Loading Order

1. Flags (highest priority)
2. Environment variables (`AMIMICA_*` prefix)
3. Config file specified by `--config`
4. `.amimica.yaml` in scan root
5. `~/.config/amimica/config.yaml`
6. Built-in defaults (lowest priority)

---

# 7. MCP Server Design

## Transport

Default: `stdio` (JSON-RPC over stdin/stdout). Also supports SSE and Streamable HTTP for network-attached use.

## Session/State Model

The MCP server is **stateful within a session**:

- A `scan_repository` or `scan_paths` call runs the analysis pipeline and stores results in memory.
- Subsequent `list_findings`, `explain_finding`, etc. reference the in-memory results.
- Results are keyed by a `scan_id` (returned from scan calls).
- Multiple scans can coexist in memory (bounded by `max_concurrent_scans`, default 3).
- Results are **ephemeral** — they exist only for the lifetime of the MCP session unless explicitly persisted via config.
- The server can optionally write results to the cache directory for persistence across sessions.

## Tool Definitions

### `scan_repository`

Runs full analysis on a repository.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "path": {
      "type": "string",
      "description": "Absolute path to repository root"
    },
    "config_overrides": {
      "type": "object",
      "description": "Optional overrides for analysis config",
      "properties": {
        "min_score": { "type": "number" },
        "min_lines": { "type": "integer" },
        "norm_level": { "type": "string", "enum": ["raw", "light", "strong", "semantic"] },
        "window_size": { "type": "integer" },
        "include_patterns": { "type": "array", "items": { "type": "string" } },
        "exclude_patterns": { "type": "array", "items": { "type": "string" } },
        "include_tests": { "type": "boolean" },
        "thorough": { "type": "boolean" },
        "max_findings": { "type": "integer" }
      }
    }
  },
  "required": ["path"]
}
```

**Output Schema:**
```json
{
  "type": "object",
  "properties": {
    "scan_id": { "type": "string" },
    "summary": {
      "type": "object",
      "properties": {
        "files_scanned": { "type": "integer" },
        "packages_scanned": { "type": "integer" },
        "functions_analyzed": { "type": "integer" },
        "units_analyzed": { "type": "integer" },
        "clone_classes_found": { "type": "integer" },
        "total_regions": { "type": "integer" },
        "duration_ms": { "type": "integer" },
        "cache_hits": { "type": "integer" },
        "cache_misses": { "type": "integer" }
      }
    },
    "top_findings": {
      "type": "array",
      "description": "Top 10 findings by score (use list_findings for full results)",
      "items": { "$ref": "#/definitions/FindingSummary" }
    }
  }
}
```

### `scan_paths`

Runs analysis on specific files or directories.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "paths": {
      "type": "array",
      "items": { "type": "string" },
      "description": "Absolute paths to files or directories"
    },
    "config_overrides": { "$ref": "#/definitions/ConfigOverrides" }
  },
  "required": ["paths"]
}
```

**Output**: Same as `scan_repository`.

### `list_findings`

Paginated listing of findings from a previous scan.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "scan_id": { "type": "string" },
    "offset": { "type": "integer", "default": 0 },
    "limit": { "type": "integer", "default": 20, "maximum": 100 },
    "min_score": { "type": "number" },
    "clone_type": { "type": "string", "enum": ["exact", "renamed", "near_duplicate", "pattern"] },
    "file_pattern": { "type": "string", "description": "Glob pattern to filter by file path" },
    "sort_by": { "type": "string", "enum": ["score", "lines", "regions"], "default": "score" }
  },
  "required": ["scan_id"]
}
```

**Output Schema:**
```json
{
  "type": "object",
  "properties": {
    "findings": {
      "type": "array",
      "items": { "$ref": "#/definitions/Finding" }
    },
    "total": { "type": "integer" },
    "offset": { "type": "integer" },
    "limit": { "type": "integer" },
    "has_more": { "type": "boolean" }
  }
}
```

### `explain_finding`

Detailed explanation of a specific finding.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "scan_id": { "type": "string" },
    "finding_id": { "type": "string" },
    "include_normalized_form": { "type": "boolean", "default": true },
    "include_diffs": { "type": "boolean", "default": true },
    "include_source_snippets": { "type": "boolean", "default": true },
    "max_snippet_lines": { "type": "integer", "default": 50 }
  },
  "required": ["scan_id", "finding_id"]
}
```

**Output Schema:**
```json
{
  "type": "object",
  "properties": {
    "finding": { "$ref": "#/definitions/Finding" },
    "normalized_form": { "type": "string" },
    "source_snippets": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "region": { "$ref": "#/definitions/SourceRegion" },
          "source": { "type": "string" },
          "annotations": {
            "type": "array",
            "items": {
              "type": "object",
              "properties": {
                "line": { "type": "integer" },
                "message": { "type": "string" }
              }
            }
          }
        }
      }
    },
    "diffs": {
      "type": "array",
      "items": { "$ref": "#/definitions/RegionDiff" }
    },
    "explanation_text": { "type": "string", "description": "Human-readable explanation paragraph" }
  }
}
```

### `compare_regions`

Ad-hoc comparison between two arbitrary code regions.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "region_a": {
      "type": "object",
      "properties": {
        "file": { "type": "string" },
        "start_line": { "type": "integer" },
        "end_line": { "type": "integer" }
      },
      "required": ["file", "start_line", "end_line"]
    },
    "region_b": {
      "type": "object",
      "properties": {
        "file": { "type": "string" },
        "start_line": { "type": "integer" },
        "end_line": { "type": "integer" }
      },
      "required": ["file", "start_line", "end_line"]
    },
    "norm_level": { "type": "string", "enum": ["raw", "light", "strong", "semantic"], "default": "strong" }
  },
  "required": ["region_a", "region_b"]
}
```

**Output Schema:**
```json
{
  "type": "object",
  "properties": {
    "similarity": { "type": "number" },
    "norm_level": { "type": "string" },
    "is_clone": { "type": "boolean" },
    "clone_type": { "type": "string" },
    "normalized_a": { "type": "string" },
    "normalized_b": { "type": "string" },
    "diff": { "$ref": "#/definitions/RegionDiff" },
    "explanation": { "type": "string" }
  }
}
```

### `suggest_refactor_classes`

Returns refactoring suggestions for a scan.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {
    "scan_id": { "type": "string" },
    "min_confidence": { "type": "number", "default": 0.5 },
    "categories": {
      "type": "array",
      "items": {
        "type": "string",
        "enum": ["extract_helper", "generic_func", "table_driven", "shared_validator", "adapter_mapper", "interface_extract", "config_driven"]
      }
    }
  },
  "required": ["scan_id"]
}
```

**Output Schema:**
```json
{
  "type": "object",
  "properties": {
    "suggestions": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "category": { "type": "string" },
          "finding_ids": { "type": "array", "items": { "type": "string" } },
          "description": { "type": "string" },
          "confidence": { "type": "number" },
          "estimated_lines_saved": { "type": "integer" }
        }
      }
    }
  }
}
```

### `get_supported_rules`

Returns detection capabilities and configuration options.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {}
}
```

**Output Schema:**
```json
{
  "type": "object",
  "properties": {
    "clone_types": { "type": "array", "items": { "type": "string" } },
    "normalization_levels": { "type": "array", "items": { "type": "string" } },
    "extraction_granularities": { "type": "array", "items": { "type": "string" } },
    "output_formats": { "type": "array", "items": { "type": "string" } },
    "version": { "type": "string" }
  }
}
```

### `get_configuration_schema`

Returns the JSON Schema for the configuration file.

**Input Schema:**
```json
{
  "type": "object",
  "properties": {}
}
```

**Output**: The full JSON Schema for `.amimica.yaml`.

## Error Model

All MCP tool errors use standard JSON-RPC error codes plus application-specific codes:

| Code | Meaning |
|---|---|
| -32600 | Invalid request (missing required fields) |
| -32601 | Unknown tool |
| -32602 | Invalid params |
| 1001 | Scan not found (invalid scan_id) |
| 1002 | Finding not found (invalid finding_id) |
| 1003 | Path not accessible (sandbox violation or doesn't exist) |
| 1004 | Analysis error (partial results available) |
| 1005 | Resource limit exceeded (too many concurrent scans) |

Error responses include a human-readable `message` and optionally a `data` object with details.

## Determinism Expectations

- Same inputs (same files, same config) → same scan results.
- Finding IDs are deterministic.
- `scan_id` is a hash of inputs (paths + config), so repeated identical requests can hit cache.

## Security Considerations

- **Path sandboxing**: When `--sandbox <path>` is set, all file access is restricted to that directory. The `fsguard` package validates every path before access.
- **No code execution**: The tool only reads and parses files. It never executes code.
- **File size limits**: Files exceeding `max_file_size` are skipped.
- **No network access**: The analysis engine never makes network calls.
- **Symlink policy**: Symlinks are not followed by default. When enabled, they are resolved and checked against the sandbox boundary.
- **Memory limits**: The server caps in-memory scan storage at `max_concurrent_scans * estimated_memory_per_scan`. Beyond this, oldest scans are evicted.

---

# 8. Repository Structure

See [Section 24](#24-final-repo-tree) for the complete tree.

Key files at the root level:

| File | Purpose |
|---|---|
| `go.mod` | Module definition: `github.com/user/amimica` |
| `go.sum` | Dependency checksums |
| `Makefile` | Build, test, lint, benchmark targets |
| `.golangci.yml` | Linter configuration |
| `.goreleaser.yml` | Release automation (future) |
| `.amimica.yaml.example` | Example configuration file |
| `PLAN.md` | This document |
| `LICENSE` | License file |

---

# 9. Configuration Model

## Configuration File Format

YAML, loaded from `.amimica.yaml` or `~/.config/amimica/config.yaml`.

```yaml
# .amimica.yaml
version: 1

analysis:
  normalization_level: strong       # raw | light | strong | semantic
  min_statements: 3                 # Minimum statements per unit
  min_lines: 6                      # Minimum lines per region
  window_size: 5                    # Sliding window statement count
  window_min_function_size: 8       # Only extract windows from functions >= this many statements
  thorough: false                   # Enable subtree extraction + rolling hash
  shingle_size: 7                   # Token n-gram size
  minhash_functions: 128            # Number of MinHash hash functions
  lsh_bands: 16                     # LSH band count
  lsh_rows: 8                       # LSH rows per band

scoring:
  min_score: 0.15                   # Minimum composite score to report
  max_findings: 100                 # Maximum findings
  weights:
    similarity: 0.30
    impact: 0.25
    refactorability: 0.20
    repetition: 0.15
    confidence: 0.10
  penalties:
    test_code: 0.5
    generated_code: 0.3
    small_region: 0.7               # < 5 statements
    single_error_check: 0.6
    same_function: 0.8

paths:
  include:
    - "**/*.go"
  exclude:
    - "vendor/**"
    - "**/*.pb.go"
    - "**/mock_*.go"
    - "**/zz_generated*.go"
  include_tests: true               # Include with penalty (default)
  include_vendor: false
  include_generated: false          # Include without penalty
  follow_symlinks: false

cache:
  enabled: true
  dir: ".amimica/cache"

output:
  format: text                      # text | json | sarif | markdown
  file: ""                          # Empty = stdout
  sort_by: score                    # score | lines | regions | file
  show_suppressed: false            # Show suppressed findings

performance:
  jobs: 0                           # 0 = GOMAXPROCS
  max_file_size: 1048576            # 1MB
  max_units_per_file: 1000          # Safety limit

logging:
  level: info                       # debug | info | warn | error
  format: text                      # text | json
```

## Environment Variable Overrides

Every config key can be overridden via environment variable:

```
AMIMICA_ANALYSIS_NORMALIZATION_LEVEL=semantic
AMIMICA_SCORING_MIN_SCORE=0.5
AMIMICA_PERFORMANCE_JOBS=4
```

Pattern: `AMIMICA_<SECTION>_<KEY>`, underscores for nesting, all uppercase.

---

# 10. Reporting and Output Formats

## Text Format

Human-readable terminal output with ANSI colors (when stdout is a TTY). Example shown in CLI section above.

## JSON Format

Array of Finding objects conforming to the Finding JSON Schema (Section 26).

## SARIF Format

SARIF v2.1.0 for CI integration:

```json
{
  "$schema": "https://raw.githubusercontent.com/oasis-tcs/sarif-spec/master/Schemata/sarif-schema-2.1.0.json",
  "version": "2.1.0",
  "runs": [{
    "tool": {
      "driver": {
        "name": "amimica",
        "version": "0.1.0",
        "rules": [{
          "id": "clone/exact",
          "shortDescription": { "text": "Exact code clone detected" },
          "defaultConfiguration": { "level": "warning" }
        }, {
          "id": "clone/renamed",
          "shortDescription": { "text": "Renamed code clone detected" },
          "defaultConfiguration": { "level": "warning" }
        }, {
          "id": "clone/near-duplicate",
          "shortDescription": { "text": "Near-duplicate code detected" },
          "defaultConfiguration": { "level": "note" }
        }]
      }
    },
    "results": [{
      "ruleId": "clone/renamed",
      "level": "warning",
      "message": { "text": "Code region is a renamed clone of 4 other regions" },
      "locations": [{
        "physicalLocation": {
          "artifactLocation": { "uri": "internal/handlers/user.go" },
          "region": { "startLine": 24, "endLine": 68 }
        }
      }],
      "relatedLocations": [{
        "physicalLocation": {
          "artifactLocation": { "uri": "internal/handlers/product.go" },
          "region": { "startLine": 18, "endLine": 62 }
        },
        "message": { "text": "Related clone region" }
      }],
      "properties": {
        "score": 0.94,
        "finding_id": "F-a1b2c3d4e5",
        "clone_class_id": "CC-x9y8z7"
      }
    }]
  }]
}
```

## Markdown Format

```markdown
# Amimica Clone Detection Report

**Repository**: /path/to/repo
**Date**: 2026-04-07T14:30:00Z
**Version**: 0.1.0

## Summary

| Metric | Value |
|---|---|
| Files scanned | 342 |
| Clone classes | 23 |
| Total regions | 67 |

## Findings

### #1: Renamed Clone (Score: 0.94)

**Regions** (5):
- `internal/handlers/user.go:24-68`
- `internal/handlers/product.go:18-62`
- ...

**Refactor suggestion**: Extract generic handler function
```

---

# 11. Testing Strategy

## Test Categories

### Unit Tests

Every package gets `_test.go` files. Coverage target: 80% line coverage.

Key test areas:
- `normalize/`: Test every normalization rule with before/after pairs. This is the most test-critical package.
- `fingerprint/`: Test hash stability, shingle computation, MinHash properties.
- `match/`: Test exact matching, LSH candidate generation, Jaccard computation.
- `score/`: Test scoring model with known inputs and expected outputs.
- `discovery/`: Test file walking with in-memory filesystem (`fstest.MapFS`).
- `config/`: Test loading order, validation, defaults, overrides.

### Golden Tests

Directory: `testdata/golden/`

Each golden test has:
- An input fixture (Go source files)
- An expected output (JSON findings)
- A config (optional overrides)

The test runner runs the full pipeline on the fixture and compares output to the golden file.

```go
func TestGolden(t *testing.T) {
    entries, _ := os.ReadDir("testdata/golden")
    for _, e := range entries {
        t.Run(e.Name(), func(t *testing.T) {
            // Load fixture, run engine.Analyze(), compare to expected.json
            // Use go-cmp for comparison with options to ignore timestamps
        })
    }
}
```

Golden files are updated with `go test -update-golden`.

### Fixture Repositories

Directory: `testdata/fixtures/`

Purpose-built Go code that exercises specific clone patterns:

| Fixture | Tests |
|---|---|
| `exact_clones/` | Two identical functions in different files |
| `renamed_clones/` | Same structure, different variable names |
| `near_duplicates/` | Similar functions with small differences |
| `handler_pattern/` | 5+ HTTP handlers with same structure |
| `error_handling/` | Repeated if-err-return patterns |
| `table_driven_candidate/` | Switch with similar cases → refactoring candidate |
| `generated_code/` | Files with "Code generated" markers |
| `malformed/` | Files with syntax errors |
| `large_file/` | File with 5000+ lines |
| `minimal_clone/` | Clone at the minimum detection threshold |
| `false_positive_traps/` | Patterns that look similar but shouldn't match |
| `cross_package/` | Clones spanning multiple packages |
| `generics/` | Generic function clone patterns |
| `nested_blocks/` | Deep nesting with repeated inner blocks |

### Benchmark Tests

```go
func BenchmarkNormalizeFunction(b *testing.B) { ... }
func BenchmarkComputeShingles(b *testing.B) { ... }
func BenchmarkMinHash(b *testing.B) { ... }
func BenchmarkLSHInsert(b *testing.B) { ... }
func BenchmarkLSHQuery(b *testing.B) { ... }
func BenchmarkFullPipeline_100Files(b *testing.B) { ... }
func BenchmarkFullPipeline_1000Files(b *testing.B) { ... }
```

Benchmark targets (order of magnitude):
- Normalize a typical function: < 50 μs
- Compute shingles for 100-token sequence: < 10 μs
- MinHash for 50-shingle set: < 5 μs
- Full pipeline for 100 files: < 1 s
- Full pipeline for 1000 files: < 10 s

### Property Tests

Using `testing/quick` or explicit generators:

- **Normalization idempotency**: `normalize(normalize(x)) == normalize(x)`
- **Hash determinism**: Same input always produces same hash.
- **Jaccard bounds**: `0 <= jaccard(a, b) <= 1`
- **MinHash approximation**: Over many random sets, MinHash Jaccard estimate converges to true Jaccard.
- **Finding ID stability**: Same regions always produce same FindingID.

### MCP Protocol Tests

Test each MCP tool handler:
- Valid requests produce valid responses.
- Invalid requests produce appropriate errors.
- Pagination works correctly.
- Scan results are correctly stored and retrieved.
- Concurrent tool calls don't interfere.
- Error recovery (scan a nonexistent path, explain a nonexistent finding).

Implementation: Start an in-process MCP server, send JSON-RPC requests, validate responses against schemas.

### CLI Snapshot Tests

Run CLI commands against fixture repos and compare stdout/stderr to expected snapshots.

```go
func TestCLIScan(t *testing.T) {
    cmd := exec.Command("go", "run", "./cmd/amimica", "scan", "testdata/fixtures/renamed_clones", "--output", "json")
    output, _ := cmd.Output()
    // Compare to testdata/snapshots/renamed_clones_scan.json
}
```

### Regression Suite

Maintain a list of known false-positive and false-negative cases:

```
testdata/regressions/
├── fp_001_interface_stubs/    # Should NOT be reported
├── fp_002_single_err_check/   # Should NOT be reported
├── fn_001_split_clone/        # SHOULD be reported (was missed in v0.1)
└── fn_002_nested_similar/     # SHOULD be reported
```

Each regression case has a `want.json` specifying expected finding count and IDs.

### Malformed Code Resilience Tests

Test with:
- Files that don't parse (syntax errors)
- Files with zero functions
- Empty files
- Files with only comments
- Files with only import statements
- Binary files accidentally included
- Files with embedded null bytes
- Extremely long lines (>10,000 chars)
- Deeply nested blocks (>50 levels)

Expected: No panics. Graceful errors logged. Other files processed normally.

---

# 12. Performance Strategy

## Bounded Memory

### Problem

A monorepo with 10,000 Go files can produce millions of `NormalizedUnit` values. All units must be in memory for matching.

### Strategy

1. **Lazy MinHash computation**: Compute MinHash only for units that survive exact-hash deduplication. If a unit matches exactly at NormStrong level, its MinHash is never computed.

2. **Streaming extraction**: Parse → Normalize → Extract → Fingerprint is done per-file. Units are appended to a slice but the AST is freed after extraction (don't hold all ASTs in memory simultaneously).

3. **Compact representation**: `NormToken` uses `token.Token` (int) + short string. Not full AST nodes.

4. **Memory budget**: At scan start, estimate memory budget:
   ```
   budget := min(available_memory * 0.7, 4GB)
   estimated_units := total_files * avg_units_per_file
   estimated_memory := estimated_units * avg_unit_size
   if estimated_memory > budget:
       reduce window extraction
       disable subtree extraction
       warn user
   ```

5. **Unit pooling**: Reuse `[]NormToken` slices via `sync.Pool`.

### Estimated Memory Usage

| Component | Per-unit memory | For 100K units |
|---|---|---|
| NormTokens (avg 80 tokens) | ~2 KB | ~200 MB |
| Shingles (avg 74 × 8 bytes) | ~600 B | ~60 MB |
| MinHash (128 × 4 bytes) | 512 B | ~50 MB |
| Metadata (region, hashes, etc.) | ~200 B | ~20 MB |
| **Total** | ~3.3 KB | **~330 MB** |

For 100K units (a large monorepo), total ~330 MB. Acceptable.

## Concurrency Model

```
       ┌─────────────┐
       │  Discovery   │  (single goroutine, yields file paths)
       └──────┬───────┘
              │ paths channel
              ▼
    ┌─────────────────────┐
    │  Worker Pool (N=CPU) │  (each worker: parse + normalize + extract + fingerprint)
    └──────────┬──────────┘
               │ units channel
               ▼
    ┌─────────────────────┐
    │  Collector           │  (single goroutine, builds index)
    └──────────┬──────────┘
               │ (all units collected)
               ▼
    ┌─────────────────────┐
    │  Matcher             │  (exact grouping, then LSH build + query, parallelized)
    └──────────┬──────────┘
               │
               ▼
    ┌─────────────────────┐
    │  Scorer + Reporter   │  (single goroutine)
    └─────────────────────┘
```

## Candidate Pruning

The matching pipeline is designed to avoid expensive comparisons:

1. **Exact grouping** (O(n)): Hash map. Groups of 1 are singletons — removed.
2. **LSH index** (O(n)): Insert all non-singleton units. Query each.
3. **Jaccard verification** (O(candidates)): Only for LSH candidate pairs. Reject pairs below threshold.
4. **Structural distance** (O(expensive)): Only for pairs above Jaccard threshold. Produces diffs.

Expected pruning ratios:
- 100K units → ~5K non-singleton groups from exact matching
- 95K remaining units → ~200K LSH candidate pairs (0.002% of all pairs)
- 200K candidate pairs → ~20K verified pairs above Jaccard threshold
- 20K pairs → ~2K high-quality findings after scoring

## Incremental Caching

As described in Section 5.9. Expected speedup: 3-10x for repeated scans of a slowly-changing repo.

## Profiling Strategy

Built-in support via `--profile` and `--memprofile` flags:

```go
if *cpuProfile != "" {
    f, _ := os.Create(*cpuProfile)
    pprof.StartCPUProfile(f)
    defer pprof.StopCPUProfile()
}
```

Additionally, expose `runtime/metrics` in the MCP server for monitoring.

### Benchmark Targets

| Operation | Target | Monorepo (10K files) |
|---|---|---|
| Full scan (cold) | 100 files/sec | ~100 s |
| Full scan (cached) | 500 files/sec | ~20 s |
| Matching (100K units) | < 30 s | < 30 s |
| Memory peak | < 2 GB | < 2 GB |
| Finding export | < 1 s | < 1 s |

---

# 13. Security and Safety

## Path Sandboxing (`fsguard` package)

```go
// fsguard/guard.go

type Guard struct {
    roots    []string  // Allowed root directories (resolved to absolute, cleaned)
    maxSize  int64     // Maximum file size
    followSymlinks bool
}

func (g *Guard) ValidatePath(path string) error {
    abs, err := filepath.Abs(path)
    if err != nil {
        return fmt.Errorf("resolve path: %w", err)
    }
    abs = filepath.Clean(abs)

    for _, root := range g.roots {
        if strings.HasPrefix(abs, root + string(filepath.Separator)) || abs == root {
            return nil
        }
    }
    return fmt.Errorf("path %q outside allowed roots", path)
}

func (g *Guard) ValidateSymlink(path string) error {
    if !g.followSymlinks {
        info, err := os.Lstat(path)
        if err != nil { return err }
        if info.Mode()&os.ModeSymlink != 0 {
            return fmt.Errorf("symlink not followed: %q", path)
        }
        return nil
    }
    // If following symlinks, resolve and check target is within roots
    resolved, err := filepath.EvalSymlinks(path)
    if err != nil { return err }
    return g.ValidatePath(resolved)
}
```

## Safe File Traversal

- Use `filepath.WalkDir` (not `Walk`) to avoid `os.Lstat` per entry.
- Check symlink status via `DirEntry.Type()` before reading.
- Skip entries where `DirEntry.Info()` fails (permission errors).

## Maximum File Size

Files exceeding `max_file_size` (default 1MB) are skipped with a warning. This prevents pathological cases (e.g., a 100MB generated file).

## Denial-of-Service Avoidance

- `max_units_per_file` (default 1000): If extraction produces more units from a single file, truncate and warn.
- Overall memory budget check before matching.
- LSH query results capped at `max_candidates_per_unit` (default 500).
- Timeout for MCP scan operations (configurable, default 5 minutes).

## Malformed Go Files

- Parser returns partial AST → analyze what we can.
- If parser returns nil AST → skip file, log error.
- Never panic on malformed input. All AST walking uses nil checks.
- Scanner errors → skip file.

## Untrusted Source Trees

The tool only reads files. It never:
- Executes `go build`, `go test`, or `go generate`
- Runs any subprocess
- Opens network connections
- Writes to the analyzed repository (writes only to cache dir and output file)

---

# 14. Phased Implementation Roadmap

## Phase 0: Project Bootstrap (1-2 days)

### Objectives
Set up the repository skeleton, build system, and CI.

### Tasks
1. Initialize Go module: `go mod init github.com/user/amimica`
2. Create directory structure per package layout
3. Create `cmd/amimica/main.go` with placeholder `cobra`-style flag parsing (use `flag` package, not cobra — stdlib first)
4. Create `Makefile` with targets: `build`, `test`, `lint`, `bench`, `vet`
5. Set up `.golangci.yml` with standard linters
6. Create `internal/model/` with all core type definitions
7. Create `internal/logging/` with `slog` setup
8. Create `internal/config/` with default config, YAML loading, validation
9. Add `testdata/fixtures/exact_clones/` with two identical Go functions in different files
10. Verify `go build ./...` and `go test ./...` pass

### Packages Touched
`cmd/amimica`, `internal/model`, `internal/config`, `internal/logging`

### Acceptance Criteria
- `amimica version` prints version string
- `go vet ./...` and `golangci-lint run` pass
- `go test ./...` passes (trivial tests)
- All core types compile

### Risks
- Premature decisions on type representations. Mitigate by keeping types in `model/` simple and adding fields later.

### Deferred
- CLI commands beyond `version`
- Any analysis logic

---

## Phase 1: Parser + Normalization MVP (3-5 days)

### Objectives
Parse Go files and normalize at all four levels. Produce `NormalizedUnit` values for functions.

### Tasks
1. Implement `internal/discovery/walker.go`: Walk directories, apply glob filters, detect test/generated files, respect max file size.
2. Implement `internal/parser/parser.go`: Parse Go files with error tolerance. Return `*ast.File` + metadata.
3. Implement `internal/normalize/normalizer.go`:
   - `NormRaw`: Strip comments, canonicalize whitespace.
   - `NormLight`: Replace literals with placeholders.
   - `NormStrong`: Replace local identifiers with positional placeholders.
   - `NormSemantic`: Abstract selectors and types.
4. Implement `internal/normalize/ident_tracker.go`: Track identifier bindings and assign placeholder names.
5. Write extensive unit tests for normalizer with before/after pairs for every Go construct.
6. Add golden tests: parse fixture files, normalize, compare token output to expected.
7. Add `testdata/fixtures/` for: basic functions, error handling, generics, selectors, anonymous functions, methods with receivers.

### Packages Touched
`internal/discovery`, `internal/parser`, `internal/normalize`, `internal/model`

### Acceptance Criteria
- Discovery correctly walks fixture repos, excludes vendor/generated as configured.
- Parser handles files with syntax errors without panicking.
- All four normalization levels produce correct output for ≥20 test cases.
- `NormStrong` normalization of the UserService/ProductRepo example from Section 5.3 produces identical token sequences.

### Risks
- Normalization rules may be too aggressive (lose semantics) or too weak (miss clones). Mitigate with comprehensive test fixtures and manual review of output.
- Go AST walking is tedious. Many node types to handle. Mitigate by using `ast.Inspect` and a switch on node type. Start with the most common constructs, add edge cases incrementally.

### Deferred
- Block and window extraction
- Fingerprinting
- Matching

---

## Phase 2: Exact Clone Detection (2-3 days)

### Objectives
Detect Type-1 and Type-2 clones using exact hash matching.

### Tasks
1. Implement `internal/extract/extractor.go`: Extract function-level units from parsed+normalized ASTs. Apply minimum size filters.
2. Implement `internal/fingerprint/hash.go`: SHA-256 hash of normalized token sequences.
3. Implement `internal/match/exact.go`: Group units by hash. Each group with 2+ members is a clone class.
4. Implement `internal/engine/engine.go`: Orchestrate discovery → parse → normalize → extract → fingerprint → match.
5. Implement `internal/report/json.go`: JSON output of findings.
6. Wire up `amimica scan` command to call `engine.Analyze()`.
7. Create golden test for exact clone detection with fixtures.
8. Create golden test for renamed clone detection (NormStrong matches).

### Packages Touched
`internal/extract`, `internal/fingerprint`, `internal/match`, `internal/engine`, `internal/report`, `internal/app`

### Acceptance Criteria
- `amimica scan testdata/fixtures/exact_clones --output json` detects the exact clone.
- `amimica scan testdata/fixtures/renamed_clones --output json` detects the renamed clone.
- Findings include correct source regions, clone type, and normalization level.
- Exit code 1 when findings exist, 0 when none.

### Risks
- Hash collisions in SHA-256 are negligible (2^-128).
- False positives from NormStrong being too aggressive. Mitigate by manually reviewing fixture results.

### Deferred
- Near-duplicate detection
- Scoring
- Window/block extraction

---

## Phase 3: Near-Duplicate Detection (4-6 days)

### Objectives
Detect Type-3 clones using shingles, MinHash, and LSH.

### Tasks
1. Implement `internal/extract/window.go`: Sliding window extraction.
2. Implement `internal/extract/block.go`: Inner block extraction.
3. Implement `internal/fingerprint/shingle.go`: Token n-gram computation.
4. Implement `internal/fingerprint/minhash.go`: MinHash signature computation.
5. Implement `internal/fingerprint/lsh.go`: LSH index (insert + query).
6. Implement `internal/match/approximate.go`: LSH candidate generation + Jaccard verification.
7. Implement `internal/match/cluster.go`: Connected component clustering of matched pairs.
8. Implement structural diff for verified candidate pairs (Myers diff on token sequences).
9. Extend `engine.Analyze()` to run approximate matching after exact matching.
10. Add golden tests for near-duplicate scenarios.
11. Benchmark LSH on 10K, 50K, 100K unit sets.

### Packages Touched
`internal/extract`, `internal/fingerprint`, `internal/match`, `internal/engine`

### Acceptance Criteria
- `amimica scan testdata/fixtures/near_duplicates` detects near-duplicate clones.
- Handler pattern fixture: all 5 similar handlers grouped into one clone class.
- Jaccard similarity values are accurate (verified against brute-force on small sets).
- LSH produces no false negatives at similarity > 0.8 (tested on fixture).
- Benchmark: 100K units indexed and queried in < 30s.

### Risks
- LSH threshold tuning. Too low → many false positives. Too high → misses. Mitigate by exposing thresholds as config and testing with multiple values.
- MinHash approximation error. Mitigate with 128 hash functions (expected error < 9%).
- Memory for MinHash signatures: 100K × 512 bytes = 50 MB. Acceptable.

### Deferred
- Scoring model
- Explainability
- Autocorrelation heuristics

---

## Phase 4: Scoring + Explainability (3-4 days)

### Objectives
Score findings meaningfully. Generate human-readable explanations.

### Tasks
1. Implement `internal/score/scorer.go`: Apply scoring model per Section 5.7.
2. Implement noise suppression rules.
3. Implement `internal/explain/explainer.go`: Generate explanation text, normalized form display, refactor hints.
4. Implement `internal/explain/diff.go`: Generate unified diffs between clone regions (raw source).
5. Implement `internal/explain/refactor.go`: Classify refactoring opportunities.
6. Implement `internal/report/text.go`: Pretty terminal output.
7. Wire up `amimica explain <finding-id>`.
8. Add golden tests for scoring (known inputs → expected scores).
9. Add golden tests for explanation output.

### Packages Touched
`internal/score`, `internal/explain`, `internal/report`, `internal/app`

### Acceptance Criteria
- Findings are ranked by composite score.
- Generated code and test code receive penalties.
- Trivial error-handling blocks are suppressed.
- `amimica explain <id>` produces human-readable output with normalized form and diff.
- Refactor hints are attached to appropriate findings.

### Risks
- Scoring weights are subjective. Mitigate by making them configurable and documenting the reasoning.
- Refactor classification accuracy is low without type info. Mitigate by keeping confidence on hints low and framing them as suggestions.

### Deferred
- MCP server
- Caching
- SARIF output

---

## Phase 5: CLI Polish (2-3 days)

### Objectives
Complete CLI commands, configuration loading, output formats.

### Tasks
1. Implement `amimica report` (re-format saved results).
2. Implement `amimica diff` (ad-hoc region comparison).
3. Implement SARIF output format.
4. Implement markdown output format.
5. Implement full config loading order (flags → env → file → defaults).
6. Add `--include`, `--exclude` pattern support.
7. Add progress output to stderr (when stdout is a file).
8. Add CLI snapshot tests for all commands.
9. Test exit codes for all scenarios.
10. Write man-page style help text for all commands.

### Packages Touched
`internal/app`, `internal/report`, `internal/config`

### Acceptance Criteria
- All 6 CLI commands work.
- All 4 output formats produce valid output.
- Config loading order is correct (verified by test).
- Exit codes match spec.
- `amimica scan . --output sarif` produces valid SARIF.

### Risks
- SARIF format complexity. Mitigate by implementing only the required fields, not the full spec.

### Deferred
- MCP server
- Caching

---

## Phase 6: MCP Server (3-5 days)

### Objectives
Implement the MCP server with all planned tools.

### Tasks
1. Implement MCP JSON-RPC protocol handling (stdio transport).
   - Use a lightweight JSON-RPC library or implement directly (it's ~200 lines for stdio).
2. Implement `internal/mcp/server.go`: Server lifecycle, session state management.
3. Implement `internal/mcp/tools.go`: Tool handler registration.
4. Implement each tool handler:
   - `scan_repository` → calls `engine.Analyze()`, stores results
   - `scan_paths` → same, for specific paths
   - `list_findings` → paginated query on stored results
   - `explain_finding` → calls `explain.Explain()`
   - `compare_regions` → ad-hoc parse + normalize + compare
   - `suggest_refactor_classes` → group findings by refactor category
   - `get_supported_rules` → static response
   - `get_configuration_schema` → static response
5. Implement scan result storage (in-memory map of `scan_id → Results`).
6. Wire up `amimica serve-mcp`.
7. Implement `fsguard` integration for path sandboxing.
8. Write MCP protocol tests (send JSON-RPC, validate response).
9. Test with a real MCP client (Claude Code or similar).

### Packages Touched
`internal/mcp`, `internal/fsguard`, `internal/app`

### Acceptance Criteria
- `amimica serve-mcp` starts and accepts JSON-RPC over stdin/stdout.
- All 8 tools respond correctly to valid and invalid inputs.
- Scan results persist across tool calls within a session.
- Path sandboxing blocks access outside allowed roots.
- Error responses use correct error codes.

### Risks
- MCP protocol spec compliance. Mitigate by testing with real clients and reading the spec carefully.
- Session state management complexity. Mitigate by keeping it simple: a map, bounded size, no persistence.

### Deferred
- SSE/HTTP transport
- Persistent scan results

---

## Phase 7: Performance / Caching (2-3 days)

### Objectives
Add content-hash caching, optimize hot paths, benchmark on real repos.

### Tasks
1. Implement `internal/cache/store.go`: DirStore (filesystem cache) and NullStore.
2. Implement cache index management (load, save, invalidate).
3. Implement gob serialization for `NormalizedUnit`.
4. Integrate cache into `engine.Analyze()`.
5. Add `--no-cache` flag.
6. Profile on a real 1000+ file Go repo (e.g., kubernetes client-go).
7. Optimize hot paths identified by profiling.
8. Add memory budget checking.
9. Add benchmark tests for full pipeline at various scales.

### Packages Touched
`internal/cache`, `internal/engine`

### Acceptance Criteria
- Second scan of unchanged repo is 3x+ faster.
- Changing one file invalidates only that file's cache.
- Config change invalidates entire cache.
- Memory stays under 2GB for 10K-file repo.
- Benchmarks documented.

### Risks
- Gob serialization compatibility across versions. Mitigate with version field in cache metadata.
- Cache corruption. Mitigate with content-hash verification on load.

### Deferred
- Persistent MCP results
- SSE transport

---

## Phase 8: CI / Reporting Integrations (1-2 days)

### Objectives
Make the tool CI-ready.

### Tasks
1. Validate SARIF output with official SARIF validator.
2. Document GitHub Actions integration (upload SARIF via `github/codeql-action/upload-sarif`).
3. Add `--baseline` flag: compare against a previous scan and report only new findings.
4. Add exit code documentation for CI usage.
5. Test in a real CI pipeline.

### Packages Touched
`internal/report`, `internal/app`

### Acceptance Criteria
- SARIF output accepted by GitHub code scanning.
- `--baseline` correctly identifies new vs. existing findings.
- Documentation includes CI integration guide.

### Deferred
- GitLab/other CI integrations

---

## Phase 9: Advanced Heuristics and Future Work (ongoing)

### Objectives
Add autocorrelation heuristics, rolling hash, optional features.

### Tasks
1. Implement autocorrelation module (Section 5.6).
2. Implement rolling hash subsequence detection.
3. Implement NormSemantic level fully (may require `go/types` for some features).
4. Add AST subtree extraction for `--thorough` mode.
5. Explore SSE transport for MCP.
6. Explore persistent scan results.

### Deferred (to v2+)
- Multi-language support
- go/types integration
- IDE plugins
- Web dashboard

---

# 15. Risks and Tradeoffs

| Risk | Impact | Mitigation |
|---|---|---|
| Normalization too aggressive → false positives | Medium | Multiple norm levels, configurable, test fixtures |
| Normalization too weak → missed clones | Medium | NormSemantic level, iterative refinement |
| Memory usage on large repos | High | Budget checking, streaming, compact representations |
| LSH threshold tuning | Medium | Configurable, tested with multiple repos |
| Scoring subjectivity | Low | Configurable weights, transparent scoring |
| MCP protocol changes | Low | Thin MCP layer, easy to update |
| Go parser limitations on malformed code | Low | Error tolerance, partial AST support |
| Cache invalidation bugs | Medium | Version field, content-hash verification |
| Performance regression | Medium | Benchmark suite, CI benchmarks |

### Fundamental Tradeoffs

1. **Syntax-only vs. type-aware analysis**: We choose syntax-only for v1. This means we miss some semantic clones but gain: faster analysis, no dependency resolution requirement, works on non-building code. The right tradeoff for a v1.

2. **Precision vs. recall**: The default thresholds favor precision (fewer false positives). Users wanting higher recall can lower `min_score` and use `--thorough`.

3. **Memory vs. speed**: We keep all units in memory for matching. This limits the tool to repos where units fit in ~2GB. For larger repos, we would need on-disk matching (deferred).

4. **Granularity vs. noise**: Finer-grained extraction (windows, blocks, subtrees) finds more clones but also more noise. Controlled by configuration and the `--thorough` flag.

---

# 16. Future Extensions

| Extension | Complexity | Value |
|---|---|---|
| Multi-language support | High | High — see Section 21 |
| `go/types` integration | Medium | Medium — improves NormSemantic |
| IDE plugin (VS Code) | Medium | High — direct developer workflow |
| Web dashboard | Medium | Medium — for team-wide analysis |
| Git-aware analysis | Medium | High — track clone evolution |
| Auto-refactoring suggestions | High | High — generate refactored code |
| Cross-repo analysis | Medium | Medium — detect clones across repos |
| Custom rule definitions | Medium | Medium — user-defined patterns |
| Pre-commit hook integration | Low | Medium — prevent new clones |
| Baseline diffing against branches | Low | High — PR-level clone detection |

---

# 17. Non-goals for v1

1. **Semantic equivalence detection**: Detecting that two different algorithms produce the same result is out of scope. We detect structural similarity.
2. **Cross-language detection**: Go only.
3. **Auto-refactoring**: We suggest refactoring categories but do not generate refactored code.
4. **Data-flow analysis**: No taint tracking, no value analysis.
5. **Build system integration**: No `go build`, `go list`, or module resolution. Pure filesystem analysis.
6. **Real-time / watch mode**: No file watching. Batch analysis only.
7. **Web UI**: No web interface. CLI + MCP only.
8. **Distributed analysis**: Single-machine only.
9. **Binary analysis**: Source code only.
10. **Obfuscated code detection**: Not designed for intentionally disguised code.

---

# 18. False Positive / False Negative Taxonomy

## Common False Positives

| Pattern | Why It's a FP | Mitigation |
|---|---|---|
| Interface method stubs | Every implementation has `func (x *T) Method() error { ... }` | Suppress methods with ≤1 statement |
| Single `if err != nil { return err }` | Ubiquitous in Go | Suppress units that are only error returns |
| Test setup boilerplate | `t.Run(...)` patterns repeat by design | Test-code penalty |
| Generated code patterns | Proto stubs, mock implementations | Generated-code penalty + marker detection |
| Import blocks | Similar import sets | Imports are not extracted as units |
| Package-level var declarations | Similar patterns across files | Not extracted in v1 |
| Standard library usage patterns | Everyone calls `http.HandleFunc` similarly | Minimum statement threshold |

## Common False Negatives

| Pattern | Why It's a FN | Mitigation |
|---|---|---|
| Same logic, different control flow | `if/else` vs. early return vs. switch | NormSemantic partially helps |
| Inlined vs. extracted helpers | One version calls a helper, other is inline | Not detected in v1 |
| Different iteration patterns | `for i := range` vs. `for i := 0; i < len` | NormSemantic partially helps |
| Clones split across functions | Two halves of a pattern in different funcs | Window extraction partially helps |
| Very small clones (< 3 statements) | Below minimum threshold | User can lower threshold |
| Cross-file patterns | Repeated file structures (same package layout) | Not detected in v1 (file-level patterns are deferred) |

## Tuning Guidance

- Too many FPs → increase `min_score`, increase `min_statements`, remove `--thorough`
- Too many FNs → decrease `min_score`, decrease `min_lines`, use `--thorough`, use `norm_level: semantic`

---

# 19. Deterministic Finding ID Design

## Requirements

1. Same codebase + same config → same finding IDs across runs.
2. Finding ID changes if the code in any region changes.
3. Finding ID is stable when unrelated code changes.
4. Finding ID is human-referable (short, printable).

## Construction

```go
func ComputeFindingID(regions []SourceRegion, cloneType CloneType, normLevel NormalizationLevel) FindingID {
    // 1. Sort regions by (File, StartLine, EndLine) — deterministic order
    sort.Slice(regions, func(i, j int) bool {
        if regions[i].File != regions[j].File {
            return regions[i].File < regions[j].File
        }
        if regions[i].StartLine != regions[j].StartLine {
            return regions[i].StartLine < regions[j].StartLine
        }
        return regions[i].EndLine < regions[j].EndLine
    })

    // 2. Hash the sorted regions + clone type + norm level
    h := sha1.New()
    for _, r := range regions {
        fmt.Fprintf(h, "%s:%d:%d\n", r.File, r.StartLine, r.EndLine)
    }
    fmt.Fprintf(h, "type:%d\nlevel:%d\n", cloneType, normLevel)

    var id FindingID
    copy(id[:], h.Sum(nil)[:20])
    return id
}

func (id FindingID) String() string {
    return "F-" + hex.EncodeToString(id[:])[:10] // "F-a1b2c3d4e5"
}
```

## Properties

- **Stable**: Adding a new file to the repo doesn't change existing finding IDs (unless the new file creates a new clone class that absorbs an existing region — but then the finding genuinely changed).
- **Unique enough**: 40-bit prefix gives 1 trillion possible IDs. Collision probability negligible for typical result sets.
- **Human-friendly**: `F-a1b2c3d4e5` is easy to copy, paste, search.

## Limitation

Finding IDs change when code is moved to different line numbers (even if content is identical). This is inherent in a line-number-based approach. A content-hash-based alternative would be stable under moves but would group different findings when the same code appears at different locations. We choose line-number-based because:
- It matches how developers think about code locations.
- It enables baseline diffing ("this finding existed before, same location").
- Content-hash-based grouping is what clone classes already provide.

---

# 20. Recommended Default Thresholds

| Parameter | Default | Rationale |
|---|---|---|
| `min_score` | 0.15 | Low bar — show more results, let user filter |
| `min_lines` | 6 | Below 6 lines, most patterns are too trivial |
| `min_statements` | 3 | 1-2 statement clones are usually error checks |
| `window_size` | 5 | Sweet spot: catches meaningful sequences, not too noisy |
| `shingle_size` | 7 | 7-token n-grams balance specificity and generality |
| `lsh_bands` | 16 | With 8 rows: ~0.5 Jaccard threshold for candidacy |
| `lsh_rows` | 8 | 128 / 16 = 8 rows per band |
| `minhash_functions` | 128 | Standard choice. Error ~1/sqrt(128) ≈ 9% |
| `jaccard_threshold` | 0.6 | Below 0.6, structural similarity is marginal |
| `max_findings` | 100 | Reasonable for terminal output. CI may want more. |
| `test_code_penalty` | 0.5 | Tests often repeat by design. Don't suppress, but penalize. |
| `generated_code_penalty` | 0.3 | Generated clones are usually not actionable. |
| `max_file_size` | 1 MB | Handles 99.9% of Go files. Larger → generated/binary. |

---

# 21. How to Evolve Toward Multi-Language Support Later

## Architecture Preparation

The key insight: **normalization is language-specific; everything else is language-agnostic**.

Current architecture already separates:
- `discovery/` — Already language-agnostic (file walking). Only needs language-specific file extension filtering.
- `parser/` — **Language-specific.** For multi-language, this becomes `parser/goparser/`, `parser/pyparser/`, etc.
- `normalize/` — **Language-specific.** Same pattern: `normalize/gonorm/`, `normalize/pynorm/`.
- `extract/` — **Partially language-specific.** Function/block concepts differ. But window extraction on token streams is generic.
- `fingerprint/` — **Language-agnostic.** Operates on `[]NormToken`, which is a language-neutral representation.
- `match/` — **Language-agnostic.** Operates on hashes and shingle sets.
- `score/` — **Language-agnostic.** May need language-specific penalty rules.
- `report/` — **Language-agnostic.**

## Evolution Path

1. Define a `Language` interface:
   ```go
   type Language interface {
       Name() string
       Extensions() []string
       Parse(path string, src []byte) (ParseResult, error)
       Normalize(parsed ParseResult, level NormalizationLevel) []NormToken
       ExtractUnits(parsed ParseResult, normalized []NormToken, cfg ExtractConfig) []NormalizedUnit
   }
   ```

2. Register languages at startup:
   ```go
   registry.Register(golang.New())
   registry.Register(python.New())  // future
   ```

3. Discovery assigns files to languages by extension.
4. Parsing, normalization, and extraction are dispatched per-language.
5. Everything from fingerprinting onward is shared.

## What NOT to Do Now

- Do NOT add the Language interface prematurely. It would be over-engineering for a Go-only tool.
- Do NOT use tree-sitter or other generic parsers. Go's own parser is better for Go.
- Do NOT try to make `NormToken` representation "universal" — it already is (token kind + normalized string).

---

# 22. How to Keep the MCP Layer Thin Over the Core Engine

## Principle

The MCP server is a **thin translation layer**. It converts MCP JSON-RPC requests into `engine.Analyze()` calls and formats the results.

## Concrete Rules

1. **No business logic in `internal/mcp/`**. Every tool handler should be < 50 lines. It unmarshals input, calls engine/explain/report, marshals output.

2. **The engine API is the source of truth**. If a feature works via CLI but not via MCP (or vice versa), something is wrong.

3. **The engine returns Go types**. The MCP layer serializes them to JSON. The CLI layer formats them to text. Neither layer transforms the data.

4. **Session state is the only MCP-specific concept**. The engine doesn't know about sessions. The MCP layer maintains a `map[string]*AnalysisResult` and passes the right result to each tool call.

5. **MCP tool schemas are generated from Go types** (or at minimum, kept in sync manually with a test that validates them).

## Example: How `explain_finding` Works

```
MCP Request → mcp.HandleExplainFinding()
    → find result by scan_id in session map
    → find finding by finding_id in result
    → call explain.Explain(finding, result.Units, fileContents)
    → serialize explanation to JSON response
```

The `explain.Explain()` function is the same one called by `amimica explain` CLI command. No duplication.

---

# 23. Open Design Questions Needing Early Validation

## Q1: Shingle Size

Is 7-token shingles optimal? Need to test with real Go code. Too small → many false positives (common token patterns match). Too large → misses smaller similarities.

**Validation**: Run on 3-5 real Go repos with shingle sizes 5, 7, 9, 11. Measure precision and recall against manually labeled clone sets.

## Q2: Window Size

Is 5-statement windows the right default? Go functions are often short (5-15 statements). A window of 5 might produce too many false matches.

**Validation**: Run on real repos with window sizes 3, 5, 7, 10. Measure noise ratio.

## Q3: NormStrong Identifier Numbering

Should identifiers be numbered in order of first appearance (left-to-right, top-to-bottom)? Or should they be numbered by declaration order (parameters first, then locals)?

Current plan: Order of first appearance in AST walk. This is simpler and deterministic.

**Validation**: Compare both approaches on 10 known clone pairs to see which produces more matches.

## Q4: Cross-Package Clone Reporting

Should a clone class that spans 5 packages show all 5 packages as regions? Or should there be a separate "cross-package pattern" finding type?

Current plan: One clone class, all regions listed. Cross-package span noted in evidence.

**Validation**: Show sample output to 3-5 Go developers. Ask if the presentation is useful.

## Q5: Minimum Clone Size Across Levels

Should `min_statements` be the same for all normalization levels? At NormRaw, a 3-statement exact match is significant. At NormSemantic, a 3-statement near-match might be noise.

Current plan: Same minimum across levels. Scoring penalizes small regions.

**Validation**: Review findings from real repos at each level.

## Q6: `flag` vs. `cobra` for CLI

The plan says stdlib `flag` first. But `flag` doesn't support subcommands well.

Options:
- **A**: Use `flag` with manual subcommand dispatch. Simple, no dependencies.
- **B**: Use a minimal subcommand library (no cobra — too heavy).
- **C**: Accept the one dependency: `github.com/spf13/cobra`.

**Recommendation**: Option A. Subcommand dispatch is ~30 lines of code. The flag package handles everything within each subcommand. Avoid the cobra dependency tree.

---

# 24. Final Repo Tree

```
amimica/
├── cmd/
│   └── amimica/
│       └── main.go                    # Entrypoint, subcommand dispatch
├── internal/
│   ├── app/
│   │   ├── scan.go                    # 'scan' command implementation
│   │   ├── report.go                  # 'report' command implementation
│   │   ├── explain.go                 # 'explain' command implementation
│   │   ├── diff.go                    # 'diff' command implementation
│   │   ├── serve.go                   # 'serve-mcp' command implementation
│   │   └── version.go                # 'version' command implementation
│   ├── config/
│   │   ├── config.go                  # Config struct, defaults
│   │   ├── load.go                    # YAML loading, env overrides, merge
│   │   ├── validate.go               # Config validation
│   │   └── config_test.go
│   ├── discovery/
│   │   ├── walker.go                  # Filesystem walking
│   │   ├── filter.go                  # Include/exclude pattern matching
│   │   ├── detect.go                  # Test file, generated file detection
│   │   ├── walker_test.go
│   │   └── filter_test.go
│   ├── parser/
│   │   ├── parser.go                  # Go file parsing with error tolerance
│   │   ├── metadata.go               # File-level metadata extraction
│   │   └── parser_test.go
│   ├── normalize/
│   │   ├── normalizer.go             # Main normalizer, dispatches by level
│   │   ├── raw.go                     # NormRaw implementation
│   │   ├── light.go                   # NormLight implementation
│   │   ├── strong.go                  # NormStrong implementation
│   │   ├── semantic.go               # NormSemantic implementation
│   │   ├── ident.go                   # Identifier tracking and renaming
│   │   ├── walk.go                    # AST walking helpers
│   │   ├── normalizer_test.go
│   │   ├── strong_test.go            # Extensive NormStrong tests
│   │   └── testdata/                  # Normalization test fixtures
│   ├── extract/
│   │   ├── extractor.go              # Unit extraction orchestration
│   │   ├── function.go               # Function/method extraction
│   │   ├── window.go                 # Sliding window extraction
│   │   ├── block.go                  # Inner block extraction
│   │   ├── subtree.go                # AST subtree extraction (--thorough)
│   │   ├── extractor_test.go
│   │   └── window_test.go
│   ├── fingerprint/
│   │   ├── hash.go                    # SHA-256 token sequence hashing
│   │   ├── shingle.go               # Token n-gram computation
│   │   ├── minhash.go               # MinHash signature computation
│   │   ├── lsh.go                     # Locality-sensitive hashing index
│   │   ├── rolling.go               # Rolling hash (--thorough)
│   │   ├── hash_test.go
│   │   ├── shingle_test.go
│   │   ├── minhash_test.go
│   │   └── lsh_test.go
│   ├── match/
│   │   ├── exact.go                   # Exact hash grouping
│   │   ├── approximate.go           # LSH candidate + Jaccard verification
│   │   ├── cluster.go               # Connected component clustering
│   │   ├── jaccard.go               # Jaccard similarity computation
│   │   ├── exact_test.go
│   │   ├── approximate_test.go
│   │   └── cluster_test.go
│   ├── score/
│   │   ├── scorer.go                  # Scoring model
│   │   ├── penalties.go              # Penalty rules
│   │   ├── suppress.go              # Noise suppression
│   │   ├── refactor.go              # Refactoring classification
│   │   ├── scorer_test.go
│   │   └── refactor_test.go
│   ├── engine/
│   │   ├── engine.go                  # Pipeline orchestration
│   │   ├── analyze.go                # Main Analyze() function
│   │   ├── options.go                # Analysis options from config
│   │   └── engine_test.go
│   ├── explain/
│   │   ├── explainer.go              # Finding explanation generation
│   │   ├── diff.go                    # Source diff computation
│   │   ├── format.go                # Explanation text formatting
│   │   ├── explainer_test.go
│   │   └── diff_test.go
│   ├── report/
│   │   ├── formatter.go              # Formatter interface
│   │   ├── text.go                    # Terminal text output
│   │   ├── json.go                    # JSON output
│   │   ├── sarif.go                  # SARIF output
│   │   ├── markdown.go              # Markdown output
│   │   ├── text_test.go
│   │   ├── json_test.go
│   │   └── sarif_test.go
│   ├── cache/
│   │   ├── store.go                   # Store interface + DirStore + NullStore
│   │   ├── index.go                  # Cache index management
│   │   ├── serial.go                 # Gob serialization
│   │   └── store_test.go
│   ├── mcp/
│   │   ├── server.go                  # MCP server lifecycle
│   │   ├── jsonrpc.go                # JSON-RPC handling (stdio)
│   │   ├── tools.go                   # Tool registration
│   │   ├── handlers.go              # Individual tool handlers
│   │   ├── session.go               # Session state (scan result storage)
│   │   ├── schemas.go               # Input/output JSON schemas
│   │   ├── server_test.go
│   │   └── handlers_test.go
│   ├── model/
│   │   ├── source.go                  # SourceFile, SourceRegion
│   │   ├── unit.go                    # NormalizedUnit, UnitKind, UnitID
│   │   ├── token.go                  # Token, NormToken, NormalizationLevel
│   │   ├── finding.go                # Finding, FindingID, CloneType
│   │   ├── score.go                  # Score, Penalty
│   │   ├── evidence.go              # Evidence, RegionDiff, DiffHunk
│   │   ├── refactor.go              # RefactorHint, RefactorCategory
│   │   └── findingid.go             # FindingID computation
│   ├── fsguard/
│   │   ├── guard.go                   # Path sandboxing
│   │   └── guard_test.go
│   └── logging/
│       └── logging.go                # slog setup
├── testdata/
│   ├── fixtures/
│   │   ├── exact_clones/
│   │   │   ├── a.go
│   │   │   └── b.go
│   │   ├── renamed_clones/
│   │   │   ├── user_service.go
│   │   │   └── product_service.go
│   │   ├── near_duplicates/
│   │   │   ├── handler_a.go
│   │   │   └── handler_b.go
│   │   ├── handler_pattern/
│   │   │   ├── user.go
│   │   │   ├── product.go
│   │   │   ├── order.go
│   │   │   ├── payment.go
│   │   │   └── invoice.go
│   │   ├── error_handling/
│   │   │   └── mixed.go
│   │   ├── table_driven_candidate/
│   │   │   └── validate.go
│   │   ├── generated_code/
│   │   │   └── generated.go
│   │   ├── malformed/
│   │   │   ├── syntax_error.go
│   │   │   ├── empty.go
│   │   │   └── comments_only.go
│   │   ├── large_file/
│   │   │   └── big.go
│   │   ├── false_positive_traps/
│   │   │   ├── interface_stubs.go
│   │   │   └── trivial_err.go
│   │   ├── cross_package/
│   │   │   ├── pkg_a/a.go
│   │   │   └── pkg_b/b.go
│   │   ├── generics/
│   │   │   └── generic_clones.go
│   │   └── nested_blocks/
│   │       └── deep.go
│   ├── golden/
│   │   ├── exact_clones/
│   │   │   └── expected.json
│   │   ├── renamed_clones/
│   │   │   └── expected.json
│   │   └── ... (one per fixture)
│   ├── regressions/
│   │   ├── fp_001_interface_stubs/
│   │   │   ├── input.go
│   │   │   └── want.json
│   │   └── fn_001_split_clone/
│   │       ├── input.go
│   │       └── want.json
│   └── snapshots/
│       ├── scan_text_output.txt
│       └── scan_json_output.json
├── .amimica.yaml.example
├── .golangci.yml
├── go.mod
├── go.sum
├── Makefile
├── LICENSE
├── PLAN.md
└── README.md
```

---

# 25. Configuration File Schema

JSON Schema for `.amimica.yaml` (the YAML file is validated against this schema):

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Amimica Configuration",
  "type": "object",
  "properties": {
    "version": {
      "type": "integer",
      "const": 1,
      "description": "Configuration schema version"
    },
    "analysis": {
      "type": "object",
      "properties": {
        "normalization_level": {
          "type": "string",
          "enum": ["raw", "light", "strong", "semantic"],
          "default": "strong"
        },
        "min_statements": { "type": "integer", "minimum": 1, "default": 3 },
        "min_lines": { "type": "integer", "minimum": 1, "default": 6 },
        "window_size": { "type": "integer", "minimum": 2, "maximum": 50, "default": 5 },
        "window_min_function_size": { "type": "integer", "minimum": 1, "default": 8 },
        "thorough": { "type": "boolean", "default": false },
        "shingle_size": { "type": "integer", "minimum": 3, "maximum": 20, "default": 7 },
        "minhash_functions": { "type": "integer", "enum": [64, 128, 256], "default": 128 },
        "lsh_bands": { "type": "integer", "minimum": 1, "default": 16 },
        "lsh_rows": { "type": "integer", "minimum": 1, "default": 8 }
      },
      "additionalProperties": false
    },
    "scoring": {
      "type": "object",
      "properties": {
        "min_score": { "type": "number", "minimum": 0, "maximum": 1, "default": 0.15 },
        "max_findings": { "type": "integer", "minimum": 1, "default": 100 },
        "weights": {
          "type": "object",
          "properties": {
            "similarity": { "type": "number", "minimum": 0, "maximum": 1 },
            "impact": { "type": "number", "minimum": 0, "maximum": 1 },
            "refactorability": { "type": "number", "minimum": 0, "maximum": 1 },
            "repetition": { "type": "number", "minimum": 0, "maximum": 1 },
            "confidence": { "type": "number", "minimum": 0, "maximum": 1 }
          },
          "additionalProperties": false
        },
        "penalties": {
          "type": "object",
          "properties": {
            "test_code": { "type": "number", "minimum": 0, "maximum": 1 },
            "generated_code": { "type": "number", "minimum": 0, "maximum": 1 },
            "small_region": { "type": "number", "minimum": 0, "maximum": 1 },
            "single_error_check": { "type": "number", "minimum": 0, "maximum": 1 },
            "same_function": { "type": "number", "minimum": 0, "maximum": 1 }
          },
          "additionalProperties": false
        }
      },
      "additionalProperties": false
    },
    "paths": {
      "type": "object",
      "properties": {
        "include": { "type": "array", "items": { "type": "string" }, "default": ["**/*.go"] },
        "exclude": { "type": "array", "items": { "type": "string" } },
        "include_tests": { "type": "boolean", "default": true },
        "include_vendor": { "type": "boolean", "default": false },
        "include_generated": { "type": "boolean", "default": false },
        "follow_symlinks": { "type": "boolean", "default": false }
      },
      "additionalProperties": false
    },
    "cache": {
      "type": "object",
      "properties": {
        "enabled": { "type": "boolean", "default": true },
        "dir": { "type": "string", "default": ".amimica/cache" }
      },
      "additionalProperties": false
    },
    "output": {
      "type": "object",
      "properties": {
        "format": { "type": "string", "enum": ["text", "json", "sarif", "markdown"], "default": "text" },
        "file": { "type": "string", "default": "" },
        "sort_by": { "type": "string", "enum": ["score", "lines", "regions", "file"], "default": "score" },
        "show_suppressed": { "type": "boolean", "default": false }
      },
      "additionalProperties": false
    },
    "performance": {
      "type": "object",
      "properties": {
        "jobs": { "type": "integer", "minimum": 0, "default": 0 },
        "max_file_size": { "type": "integer", "minimum": 1024, "default": 1048576 },
        "max_units_per_file": { "type": "integer", "minimum": 10, "default": 1000 }
      },
      "additionalProperties": false
    },
    "logging": {
      "type": "object",
      "properties": {
        "level": { "type": "string", "enum": ["debug", "info", "warn", "error"], "default": "info" },
        "format": { "type": "string", "enum": ["text", "json"], "default": "text" }
      },
      "additionalProperties": false
    }
  },
  "required": ["version"],
  "additionalProperties": false
}
```

---

# 26. Finding JSON Schema

```json
{
  "$schema": "http://json-schema.org/draft-07/schema#",
  "title": "Amimica Finding",
  "type": "object",
  "properties": {
    "id": {
      "type": "string",
      "description": "Deterministic finding ID (e.g., F-a1b2c3d4e5)",
      "pattern": "^F-[0-9a-f]{10}$"
    },
    "clone_class_id": {
      "type": "string",
      "description": "Groups all members of the same clone class (e.g., CC-x9y8z7w6v5)"
    },
    "type": {
      "type": "string",
      "enum": ["exact", "renamed", "near_duplicate", "pattern"],
      "description": "Clone type classification"
    },
    "normalization_level": {
      "type": "string",
      "enum": ["raw", "light", "strong", "semantic"],
      "description": "Normalization level at which match was detected"
    },
    "regions": {
      "type": "array",
      "minItems": 2,
      "items": {
        "type": "object",
        "properties": {
          "file": { "type": "string" },
          "start_line": { "type": "integer", "minimum": 1 },
          "end_line": { "type": "integer", "minimum": 1 },
          "start_col": { "type": "integer", "minimum": 1 },
          "end_col": { "type": "integer", "minimum": 1 },
          "func_name": { "type": "string" },
          "receiver": { "type": "string" },
          "package": { "type": "string" }
        },
        "required": ["file", "start_line", "end_line"]
      }
    },
    "score": {
      "type": "object",
      "properties": {
        "composite": { "type": "number", "minimum": 0, "maximum": 1 },
        "confidence": { "type": "number", "minimum": 0, "maximum": 1 },
        "similarity": { "type": "number", "minimum": 0, "maximum": 1 },
        "impact": { "type": "number", "minimum": 0, "maximum": 1 },
        "refactorability": { "type": "number", "minimum": 0, "maximum": 1 },
        "penalties": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "reason": { "type": "string" },
              "factor": { "type": "number" }
            },
            "required": ["reason", "factor"]
          }
        }
      },
      "required": ["composite", "confidence", "similarity"]
    },
    "evidence": {
      "type": "object",
      "properties": {
        "matched_normalized_form": { "type": "string" },
        "shared_tokens": { "type": "integer" },
        "total_tokens": { "type": "integer" },
        "similarity_metric": { "type": "string" },
        "similarity_value": { "type": "number" },
        "contributing_nodes": {
          "type": "array",
          "items": { "type": "string" }
        },
        "diffs": {
          "type": "array",
          "items": {
            "type": "object",
            "properties": {
              "region_a": { "$ref": "#/properties/regions/items" },
              "region_b": { "$ref": "#/properties/regions/items" },
              "hunks": {
                "type": "array",
                "items": {
                  "type": "object",
                  "properties": {
                    "line_a": { "type": "integer" },
                    "line_b": { "type": "integer" },
                    "content": { "type": "string" }
                  }
                }
              }
            }
          }
        }
      }
    },
    "refactor_hints": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "category": {
            "type": "string",
            "enum": [
              "extract_helper",
              "generic_func",
              "table_driven",
              "shared_validator",
              "adapter_mapper",
              "interface_extract",
              "config_driven"
            ]
          },
          "description": { "type": "string" },
          "confidence": { "type": "number", "minimum": 0, "maximum": 1 }
        },
        "required": ["category", "description", "confidence"]
      }
    },
    "suppressed": { "type": "boolean", "default": false },
    "suppress_reason": { "type": "string" }
  },
  "required": ["id", "clone_class_id", "type", "normalization_level", "regions", "score"]
}
```

Example finding:

```json
{
  "id": "F-a1b2c3d4e5",
  "clone_class_id": "CC-x9y8z7w6v5",
  "type": "renamed",
  "normalization_level": "strong",
  "regions": [
    {
      "file": "internal/handlers/user.go",
      "start_line": 24,
      "end_line": 68,
      "func_name": "GetUser",
      "receiver": "*UserHandler",
      "package": "handlers"
    },
    {
      "file": "internal/handlers/product.go",
      "start_line": 18,
      "end_line": 62,
      "func_name": "GetProduct",
      "receiver": "*ProductHandler",
      "package": "handlers"
    }
  ],
  "score": {
    "composite": 0.94,
    "confidence": 0.85,
    "similarity": 1.0,
    "impact": 0.72,
    "refactorability": 0.90,
    "penalties": []
  },
  "evidence": {
    "matched_normalized_form": "$V0, $V1 := $R.$FIELD.Find($P0, $P1)\nif $V1 != nil {\n    return nil, fmt.Errorf($STR, $V1)\n}\nif $V0 == nil {\n    return nil, $PKG.ErrNotFound\n}\nreturn $V0, nil",
    "shared_tokens": 47,
    "total_tokens": 47,
    "similarity_metric": "exact_hash_normstrong",
    "similarity_value": 1.0,
    "contributing_nodes": ["FuncDecl", "IfStmt", "ReturnStmt", "CallExpr"]
  },
  "refactor_hints": [
    {
      "category": "generic_func",
      "description": "Both functions have identical structure with different types. Consider extracting a generic function parameterized on the entity type.",
      "confidence": 0.80
    }
  ],
  "suppressed": false
}
```

---

# 27. First 10-Task Execution Backlog

| # | Task | Phase | Est. | Dependencies | Deliverable |
|---|---|---|---|---|---|
| 1 | Initialize Go module, create directory skeleton, Makefile | P0 | 2h | None | `go build` works, all dirs exist |
| 2 | Define all types in `internal/model/` (Section 4) | P0 | 3h | Task 1 | Types compile, documented |
| 3 | Implement `internal/config/` (load YAML, validate, defaults, env overrides) | P0 | 4h | Task 2 | Config test suite green |
| 4 | Implement `internal/logging/` (slog setup) + `internal/fsguard/` (path sandboxing) | P0 | 2h | Task 1 | Guard test suite green |
| 5 | Implement `internal/discovery/` (file walking, filtering, metadata detection) | P1 | 4h | Tasks 2, 4 | Discovery walks fixtures correctly |
| 6 | Implement `internal/parser/` (Go parsing with error tolerance) | P1 | 3h | Task 2 | Parses all fixtures including malformed |
| 7 | Implement `internal/normalize/` — NormRaw and NormLight levels | P1 | 6h | Tasks 2, 6 | 15+ test cases pass for both levels |
| 8 | Implement `internal/normalize/` — NormStrong level with identifier tracking | P1 | 8h | Task 7 | UserService/ProductRepo example normalizes identically; 20+ test cases pass |
| 9 | Implement `internal/extract/function.go` (function extraction) + `internal/fingerprint/hash.go` (SHA-256 hashing) | P2 | 4h | Task 8 | Units extracted, hashed for fixtures |
| 10 | Implement `internal/match/exact.go` + `internal/engine/engine.go` (exact clone pipeline) + `internal/report/json.go` | P2 | 6h | Task 9 | `amimica scan testdata/fixtures/exact_clones --output json` detects clone |

**Total estimated hours for first 10 tasks: ~42 hours (5-6 full working days)**

After these 10 tasks, the tool produces its first real findings — exact and renamed clones — with JSON output. This is the minimum viable prototype that validates the core approach.

---

*End of plan.*
