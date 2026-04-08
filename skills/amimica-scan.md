---
name: amimica-scan
description: Use Amimica to detect code clones in Go, JavaScript/TypeScript, and Ruby codebases
triggers:
  - find duplicate code
  - find clones
  - find repetitive code
  - detect copy paste
  - code duplication
  - amimica
  - scan for clones
---

# Amimica Clone Detection

Amimica detects repetitive code patterns in **Go**, **JavaScript/TypeScript**, and **Ruby**.

## As CLI

```bash
# Build (one time)
cd <amimica-repo> && make build

# Scan current project
amimica scan .

# Top 20 results only
amimica scan -n 20 .

# JSON output
amimica scan --output json .

# High-confidence only
amimica scan --min-score 0.5 .

# Exclude test files
amimica scan --exclude-tests .
```

## As MCP server

Add to Claude Code settings (`~/.claude/settings.json`):

```json
{
  "mcpServers": {
    "amimica": {
      "command": "<path-to>/amimica",
      "args": ["serve-mcp"]
    }
  }
}
```

### MCP tools

| Tool | Description |
|------|-------------|
| `scan` | Scan directories for clones. Returns summary + scan_id. |
| `list_findings` | Paginated list of findings from a scan. |
| `explain_finding` | Detailed explanation of a specific finding. |
| `compare_regions` | Side-by-side view of two code regions. |

### Example MCP workflow

1. `scan` with `paths: ["."]` → get scan_id
2. `list_findings` with `scan_id` → browse results
3. `explain_finding` with `scan_id` + `finding_id` → get details
4. `compare_regions` with file paths and line ranges → see actual code

## Interpreting results

| Score | Meaning |
|-------|---------|
| > 0.8 | High confidence — almost certainly actionable |
| 0.5-0.8 | Medium — likely real, worth reviewing |
| 0.15-0.5 | Low — possible clone, may be intentional |

## Suppressing findings

```go
// amimica-ignore
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
