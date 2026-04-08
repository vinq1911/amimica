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

## Model types

All shared data types live in `internal/model/`. This package must have ZERO imports from other `internal/` packages.

When adding a new model type:
- Put it in a file named after the primary type (e.g., `finding.go` for `Finding`)
- Add a `String()` method for any `iota` type
- Use `[N]byte` for fixed-size hashes, not `[]byte`

## Error handling

```go
// DO: Wrap with context
return fmt.Errorf("normalize function %s: %w", fn.Name, err)

// DON'T: Bare return
return err
```

## Testing patterns

```go
// Table-driven tests
func TestNormalize(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  string
    }{
        {name: "empty", input: "", want: ""},
        // ...
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

## Logging

Use `slog` via the logger from `internal/logging`:
```go
slog.Debug("parsing file", "path", path, "size", size)
slog.Warn("skipping file", "path", path, "reason", "exceeds max size")
```

Never use `fmt.Println` or `log.Println` for operational output.

## Normalization levels

When working on normalization code:
- `NormRaw` (0): Strip comments and whitespace only
- `NormLight` (1): Replace literals with `$STR`, `$INT`, `$FLOAT`, `$RUNE`
- `NormStrong` (2): Replace local identifiers with positional `$V0`, `$P0`, `$R`
- `NormSemantic` (3): Abstract selectors, type names, channels

Each level includes all transformations from lower levels.

## Pipeline order

Components run in this order — respect the data flow:
```
Discovery → Parser → Normalizer → Extractor → Fingerprinter → Matcher → Scorer → Reporter
```

The `engine` package orchestrates this. Individual packages should not reach across the pipeline.
