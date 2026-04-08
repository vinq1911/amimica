---
name: amimica-mcp
description: MCP server design patterns and tool implementation conventions
triggers:
  - working on MCP server
  - implementing MCP tools
  - working in internal/mcp/
---

# Amimica MCP Server Skill

## Thin layer principle

The MCP server in `internal/mcp/` is a thin translation layer. It:
1. Receives JSON-RPC requests
2. Unmarshals input parameters
3. Calls `engine.Analyze()` or other engine functions
4. Marshals results to JSON-RPC responses

**No business logic in the MCP layer.** If you find yourself writing analysis code in `internal/mcp/`, it belongs in `internal/engine/` or another core package.

## Tool handler pattern

Each tool handler should be under 50 lines:

```go
func (s *Server) handleScanRepository(params json.RawMessage) (any, error) {
    var input ScanRepositoryInput
    if err := json.Unmarshal(params, &input); err != nil {
        return nil, &jsonrpcError{Code: -32602, Message: "invalid params"}
    }
    // Validate input, call engine, return result
}
```

## Session state

- Results are stored in-memory, keyed by `scan_id`
- `scan_id` is a hash of inputs (paths + config) — deterministic
- Max `max_concurrent_scans` (default 3) results in memory
- Results are ephemeral — gone when server stops

## Transport

Default: stdio (JSON-RPC over stdin/stdout). Keep it simple.

## Error codes

| Code | Meaning |
|------|---------|
| -32600 | Invalid request |
| -32601 | Unknown tool |
| -32602 | Invalid params |
| 1001 | Scan not found |
| 1002 | Finding not found |
| 1003 | Path not accessible |
| 1004 | Analysis error |
| 1005 | Resource limit |
