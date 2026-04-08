---
name: amimica-scan
description: Use Amimica to detect code clones via CLI or MCP tools
triggers:
  - find duplicate code
  - find clones
  - find repetitive code
  - detect copy paste
  - code duplication
  - amimica
  - scan for clones
  - similar code
  - refactor duplicates
---

# Amimica Clone Detection

Amimica detects repetitive code patterns in **Go**, **JavaScript/TypeScript**, and **Ruby**.

## CLI usage

```bash
amimica scan .                          # Scan current directory
amimica scan -n 20 .                    # Top 20 findings
amimica scan --output json .            # JSON for programmatic use
amimica scan --min-score 0.5 .          # High-confidence only
amimica scan --exclude-tests .          # Skip test files
amimica scan ./src ./lib ./app          # Scan specific dirs
amimica scan --output-file report.json --output json .  # Save to file
```

## MCP tools

When Amimica is configured as an MCP server, use these tools:

### `scan`

Scan directories for code clones.

```json
{"paths": ["./src"], "min_score": 0.5, "max_results": 20}
```

Returns a text summary with findings and a `scan_id` for follow-up.

### `list_findings`

Page through findings from a previous scan.

```json
{"scan_id": "scan-1", "limit": 10, "offset": 0, "min_score": 0.5}
```

### `explain_finding`

Get detailed breakdown of one finding: scores, normalized form, refactor hints.

```json
{"scan_id": "scan-1", "finding_id": "F-a1b2c3d4e5"}
```

### `compare_regions`

View two code regions side by side.

```json
{
  "file_a": "handlers/user.go", "start_line_a": 24, "end_line_a": 68,
  "file_b": "handlers/product.go", "start_line_b": 18, "end_line_b": 62
}
```

## Typical MCP workflow

1. `scan` with `paths: ["."]` → get `scan_id`
2. `list_findings` with `scan_id`, `min_score: 0.5` → browse results
3. `explain_finding` for the most interesting finding → understand the clone
4. `compare_regions` to see the actual code side by side
5. Suggest refactoring based on the clone type and hints

## Interpreting results

**Scores** (0.0-1.0):

| Range | Meaning |
|-------|---------|
| > 0.8 | High confidence — almost certainly actionable |
| 0.5-0.8 | Medium — likely real, worth reviewing |
| 0.15-0.5 | Low — possible clone, may be intentional |

**Clone types**:

| Type | Meaning |
|------|---------|
| `renamed` | Identical structure, different names. Exact match after normalization. |
| `near_duplicate` | Very similar with small differences. |
| `exact` | Identical code (whitespace/comment differences only). |

**Refactor hints**:
- `extract_helper` — pull shared logic into a function
- `table_driven` — replace repeated blocks with a data-driven approach
- `interface_extract` — unify similar implementations behind an interface

## Suppressing findings

```go
// amimica-ignore: reason
func intentionallyDuplicated() { ... }
```

```typescript
// amimica-ignore
function legacyHandler() { ... }
```

```ruby
# amimica-ignore
def process(document)
```
