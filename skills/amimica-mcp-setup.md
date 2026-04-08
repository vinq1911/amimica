---
name: amimica-mcp-setup
description: Set up Amimica as an MCP server for Claude Code or other MCP clients
triggers:
  - set up amimica mcp
  - configure amimica server
  - amimica mcp server
  - add amimica to claude
---

# Setting up Amimica MCP Server

## 1. Build Amimica

```bash
git clone https://github.com/vinq1911/amimica.git
cd amimica
make build
```

Binary: `./bin/amimica`

## 2. Configure Claude Code

Add to `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "amimica": {
      "command": "/absolute/path/to/amimica",
      "args": ["serve-mcp"]
    }
  }
}
```

Or for project-level config, add to `.claude/settings.json` in your project:

```json
{
  "mcpServers": {
    "amimica": {
      "command": "/absolute/path/to/amimica",
      "args": ["serve-mcp", "--config", ".amimica.yaml"]
    }
  }
}
```

## 3. Available tools

Once configured, Claude can use these tools:

### `scan`
Scan directories for code clones.

```json
{
  "paths": ["./src", "./lib"],
  "min_score": 0.5,
  "max_results": 20
}
```

Returns a human-readable summary with a `scan_id` for follow-up queries.

### `list_findings`
Browse findings from a previous scan with pagination.

```json
{
  "scan_id": "scan-1",
  "min_score": 0.5,
  "limit": 10,
  "offset": 0
}
```

### `explain_finding`
Get detailed explanation of a specific finding.

```json
{
  "scan_id": "scan-1",
  "finding_id": "F-a1b2c3d4e5"
}
```

Returns: normalized form, scores breakdown, refactoring suggestions, penalties.

### `compare_regions`
View two code regions side by side.

```json
{
  "file_a": "src/handlers/user.go",
  "start_line_a": 24,
  "end_line_a": 68,
  "file_b": "src/handlers/product.go",
  "start_line_b": 18,
  "end_line_b": 62
}
```

## 4. Supported languages

Go (`.go`), JavaScript (`.js`, `.jsx`, `.mjs`), TypeScript (`.ts`, `.tsx`, `.mts`), Ruby (`.rb`, `.rake`)
