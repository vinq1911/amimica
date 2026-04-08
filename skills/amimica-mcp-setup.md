---
name: amimica-mcp-setup
description: Set up Amimica as an MCP server for Claude Code
triggers:
  - set up amimica mcp
  - configure amimica server
  - amimica mcp server
  - add amimica to claude
  - install amimica
---

# Setting up Amimica MCP Server

## 1. Build

```bash
git clone https://github.com/vinq1911/amimica.git
cd amimica
make build
# Binary: ./bin/amimica
```

Or if already cloned: `cd amimica && make build`

## 2. Configure Claude Code

**Global** (all projects) — add to `~/.claude/settings.json`:

```json
{
  "mcpServers": {
    "amimica": {
      "command": "/absolute/path/to/amimica/bin/amimica",
      "args": ["serve-mcp"]
    }
  }
}
```

**Project-level** — add to `.claude/settings.json` in the project root:

```json
{
  "mcpServers": {
    "amimica": {
      "command": "/absolute/path/to/amimica/bin/amimica",
      "args": ["serve-mcp", "--config", ".amimica.yaml"]
    }
  }
}
```

## 3. Verify

After configuring, Claude Code will show Amimica's 4 tools:

| Tool | What it does |
|------|-------------|
| `scan` | Scan dirs for clones → returns summary + scan_id |
| `list_findings` | Page through findings from a scan |
| `explain_finding` | Detailed breakdown of one finding |
| `compare_regions` | Side-by-side view of two code regions |

## 4. Tool details

### `scan`

**Input**: `paths` (string[]), `min_score` (number), `max_results` (int)

**Output**: Text with scan stats, top findings, and a `scan_id` for follow-up calls.

### `list_findings`

**Input**: `scan_id` (required), `min_score`, `limit` (default 20), `offset`

**Output**: Paginated finding list with IDs, scores, regions, refactor hints.

### `explain_finding`

**Input**: `scan_id` (required), `finding_id` (required, e.g. "F-a1b2c3d4e5")

**Output**: Full breakdown — type, score components, normalized form, penalties, suggestions.

### `compare_regions`

**Input**: `file_a`, `start_line_a`, `end_line_a`, `file_b`, `start_line_b`, `end_line_b` (all required)

**Output**: Source code from both regions, labeled.

## 5. Session behavior

- Results live in memory for the MCP session lifetime
- Each `scan` returns a unique `scan_id` (e.g., `scan-1`, `scan-2`)
- Multiple scans can coexist — scan different directories independently
- When the server stops, all results are gone (ephemeral)

## 6. Supported languages

Go (`.go`), JavaScript (`.js`, `.jsx`, `.mjs`), TypeScript (`.ts`, `.tsx`, `.mts`), Ruby (`.rb`, `.rake`, `.gemspec`)

## 7. Optional: install skills

For Claude to learn Amimica usage patterns without MCP:

```bash
cp /path/to/amimica/skills/*.md ~/.claude/skills/
```
