# Amimica

## Overview
Go code clone detection CLI tool and MCP server. Scans Go codebases and identifies repetitive code patterns using AST normalization, fingerprinting, and similarity analysis.

## Tech Stack
- Language: Go 1.26
- Standard library first (go/ast, go/parser, go/token, encoding, crypto, log/slog)
- Third-party only when clearly justified

## Key Documents
- [PLAN.md](../PLAN.md) — Full engineering plan
