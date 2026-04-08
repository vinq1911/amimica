// Package mcp implements an MCP (Model Context Protocol) server that exposes
// Amimica's clone detection as tools for editor and agent integration.
// It uses the stdio transport (JSON-RPC over stdin/stdout).
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"github.com/user/amimica/internal/config"
	"github.com/user/amimica/internal/engine"
	"github.com/user/amimica/internal/report"
)

// Server is the MCP server that handles JSON-RPC requests over stdio.
type Server struct {
	cfg     *config.Config
	log     *slog.Logger
	results map[string]*report.Result // scan_id → results
	mu      sync.RWMutex
}

// NewServer creates a new MCP server.
func NewServer(cfg *config.Config, log *slog.Logger) *Server {
	return &Server{
		cfg:     cfg,
		log:     log,
		results: make(map[string]*report.Result),
	}
}

// --- JSON-RPC types ---

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      any         `json:"id,omitempty"`
	Result  any         `json:"result,omitempty"`
	Error   *rpcError   `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// --- MCP protocol types ---

type initializeResult struct {
	ProtocolVersion string       `json:"protocolVersion"`
	Capabilities    capabilities `json:"capabilities"`
	ServerInfo      serverInfo   `json:"serverInfo"`
}

type capabilities struct {
	Tools *toolsCap `json:"tools,omitempty"`
}

type toolsCap struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

type serverInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type toolsListResult struct {
	Tools []toolDef `json:"tools"`
}

type toolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type callToolParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type callToolResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// Serve runs the MCP server on stdio until EOF.
func (s *Server) Serve() error {
	reader := bufio.NewReader(os.Stdin)
	encoder := json.NewEncoder(os.Stdout)

	s.log.Info("amimica MCP server started")

	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err == io.EOF {
				s.log.Info("stdin closed, shutting down")
				return nil
			}
			return fmt.Errorf("read stdin: %w", err)
		}

		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
			s.log.Debug("invalid JSON-RPC", "error", err)
			continue
		}

		resp := s.handleRequest(req)
		if resp != nil {
			if err := encoder.Encode(resp); err != nil {
				s.log.Error("write response", "error", err)
			}
		}
	}
}

func (s *Server) handleRequest(req jsonrpcRequest) *jsonrpcResponse {
	switch req.Method {
	case "initialize":
		return s.respond(req.ID, initializeResult{
			ProtocolVersion: "2024-11-05",
			Capabilities:    capabilities{Tools: &toolsCap{}},
			ServerInfo:      serverInfo{Name: "amimica", Version: "0.1.0-dev"},
		})

	case "notifications/initialized":
		return nil // notification, no response

	case "tools/list":
		return s.respond(req.ID, toolsListResult{Tools: s.toolDefinitions()})

	case "tools/call":
		var params callToolParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return s.respondError(req.ID, -32602, "invalid params", nil)
		}
		return s.handleToolCall(req.ID, params)

	case "ping":
		return s.respond(req.ID, map[string]string{})

	default:
		// Unknown methods — ignore notifications, error on requests.
		if req.ID == nil {
			return nil
		}
		return s.respondError(req.ID, -32601, fmt.Sprintf("unknown method: %s", req.Method), nil)
	}
}

func (s *Server) handleToolCall(id any, params callToolParams) *jsonrpcResponse {
	switch params.Name {
	case "scan":
		return s.handleScan(id, params.Arguments)
	case "list_findings":
		return s.handleListFindings(id, params.Arguments)
	case "explain_finding":
		return s.handleExplainFinding(id, params.Arguments)
	case "compare_regions":
		return s.handleCompareRegions(id, params.Arguments)
	default:
		return s.respondError(id, -32601, fmt.Sprintf("unknown tool: %s", params.Name), nil)
	}
}

// --- Tool handlers ---

type scanArgs struct {
	Paths    []string `json:"paths"`
	MinScore float64  `json:"min_score,omitempty"`
	MaxResults int    `json:"max_results,omitempty"`
}

func (s *Server) handleScan(id any, raw json.RawMessage) *jsonrpcResponse {
	var args scanArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return s.respondError(id, -32602, "invalid arguments: "+err.Error(), nil)
	}
	if len(args.Paths) == 0 {
		args.Paths = []string{"."}
	}

	cfg := *s.cfg // copy
	if args.MinScore > 0 {
		cfg.Scoring.MinScore = args.MinScore
	}
	if args.MaxResults > 0 {
		cfg.Scoring.MaxFindings = args.MaxResults
	}

	result, err := engine.Analyze(args.Paths, &cfg, s.log)
	if err != nil {
		return s.toolError(id, "analysis failed: "+err.Error())
	}

	// Store result for later reference.
	scanID := fmt.Sprintf("scan-%d", len(s.results)+1)
	s.mu.Lock()
	s.results[scanID] = result
	s.mu.Unlock()

	// Build summary.
	visible := 0
	for _, f := range result.Findings {
		if !f.Suppressed {
			visible++
		}
	}

	summary := fmt.Sprintf("Scanned %d files (%d functions, %d units) in %s.\n"+
		"Found %d clone classes (scan_id: %s).\n\n",
		result.FilesScanned, result.FuncsAnalyzed, result.UnitsAnalyzed,
		result.Duration.Round(1e6), visible, scanID)

	// Add top findings.
	count := 0
	for _, f := range result.Findings {
		if f.Suppressed {
			continue
		}
		count++
		if count > 10 {
			summary += fmt.Sprintf("\n... and %d more. Use list_findings with scan_id=%q to see all.\n", visible-10, scanID)
			break
		}
		summary += fmt.Sprintf("#%d Score: %.2f | %s | %d regions\n", count, f.Score.Composite, f.Type.String(), len(f.Regions))
		for _, r := range f.Regions {
			fn := r.FuncName
			if r.Receiver != "" {
				fn = r.Receiver + "." + fn
			}
			summary += fmt.Sprintf("   %s:%d-%d %s\n", r.File, r.StartLine, r.EndLine, fn)
		}
		if len(f.RefactorHints) > 0 {
			summary += fmt.Sprintf("   → %s\n", f.RefactorHints[0].Description)
		}
		summary += "\n"
	}

	return s.toolResult(id, summary)
}

type listFindingsArgs struct {
	ScanID   string  `json:"scan_id"`
	MinScore float64 `json:"min_score,omitempty"`
	Limit    int     `json:"limit,omitempty"`
	Offset   int     `json:"offset,omitempty"`
}

func (s *Server) handleListFindings(id any, raw json.RawMessage) *jsonrpcResponse {
	var args listFindingsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return s.respondError(id, -32602, "invalid arguments", nil)
	}

	s.mu.RLock()
	result, ok := s.results[args.ScanID]
	s.mu.RUnlock()
	if !ok {
		return s.toolError(id, fmt.Sprintf("scan_id %q not found. Run scan first.", args.ScanID))
	}

	if args.Limit == 0 {
		args.Limit = 20
	}

	var text string
	count := 0
	shown := 0
	for _, f := range result.Findings {
		if f.Suppressed {
			continue
		}
		if args.MinScore > 0 && f.Score.Composite < args.MinScore {
			continue
		}
		count++
		if count <= args.Offset {
			continue
		}
		if shown >= args.Limit {
			text += fmt.Sprintf("\n... %d more findings available (offset=%d)\n", count-args.Offset-shown, args.Offset+shown)
			break
		}
		shown++
		text += fmt.Sprintf("#%d [F-%s] Score: %.2f | %s\n", count, f.ID.String()[:10], f.Score.Composite, f.Type.String())
		for _, r := range f.Regions {
			text += fmt.Sprintf("   %s:%d-%d %s\n", r.File, r.StartLine, r.EndLine, r.FuncName)
		}
		if len(f.RefactorHints) > 0 {
			text += fmt.Sprintf("   → %s\n", f.RefactorHints[0].Description)
		}
		text += "\n"
	}

	if text == "" {
		text = "No findings match the criteria."
	}

	return s.toolResult(id, text)
}

type explainFindingArgs struct {
	ScanID    string `json:"scan_id"`
	FindingID string `json:"finding_id"`
}

func (s *Server) handleExplainFinding(id any, raw json.RawMessage) *jsonrpcResponse {
	var args explainFindingArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return s.respondError(id, -32602, "invalid arguments", nil)
	}

	s.mu.RLock()
	result, ok := s.results[args.ScanID]
	s.mu.RUnlock()
	if !ok {
		return s.toolError(id, fmt.Sprintf("scan_id %q not found", args.ScanID))
	}

	for _, f := range result.Findings {
		fid := "F-" + f.ID.String()[:10]
		if fid == args.FindingID || f.ID.String() == args.FindingID {
			text := fmt.Sprintf("Finding %s\n", fid)
			text += fmt.Sprintf("Type: %s\n", f.Type.String())
			text += fmt.Sprintf("Score: %.2f (confidence=%.2f, similarity=%.2f, impact=%.2f)\n",
				f.Score.Composite, f.Score.Confidence, f.Score.Similarity, f.Score.Impact)
			text += fmt.Sprintf("Normalization level: %s\n\n", f.NormLevel.String())

			text += "Regions:\n"
			for _, r := range f.Regions {
				text += fmt.Sprintf("  %s:%d-%d %s\n", r.File, r.StartLine, r.EndLine, r.FuncName)
			}

			if f.Evidence.MatchedNormForm != "" {
				text += fmt.Sprintf("\nNormalized form:\n%s\n", f.Evidence.MatchedNormForm)
			}

			if len(f.RefactorHints) > 0 {
				text += "\nRefactoring suggestions:\n"
				for _, h := range f.RefactorHints {
					text += fmt.Sprintf("  [%s] %s (confidence: %.0f%%)\n", h.Category, h.Description, h.Confidence*100)
				}
			}

			if len(f.Score.Penalties) > 0 {
				text += "\nPenalties applied:\n"
				for _, p := range f.Score.Penalties {
					text += fmt.Sprintf("  %s (×%.1f)\n", p.Reason, p.Factor)
				}
			}

			return s.toolResult(id, text)
		}
	}

	return s.toolError(id, fmt.Sprintf("finding %q not found in scan %q", args.FindingID, args.ScanID))
}

type compareRegionsArgs struct {
	FileA      string `json:"file_a"`
	StartLineA int    `json:"start_line_a"`
	EndLineA   int    `json:"end_line_a"`
	FileB      string `json:"file_b"`
	StartLineB int    `json:"start_line_b"`
	EndLineB   int    `json:"end_line_b"`
}

func (s *Server) handleCompareRegions(id any, raw json.RawMessage) *jsonrpcResponse {
	var args compareRegionsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return s.respondError(id, -32602, "invalid arguments", nil)
	}

	// Read both files and extract the specified regions.
	textA, err := readLines(args.FileA, args.StartLineA, args.EndLineA)
	if err != nil {
		return s.toolError(id, fmt.Sprintf("read %s: %v", args.FileA, err))
	}
	textB, err := readLines(args.FileB, args.StartLineB, args.EndLineB)
	if err != nil {
		return s.toolError(id, fmt.Sprintf("read %s: %v", args.FileB, err))
	}

	text := fmt.Sprintf("Region A: %s:%d-%d\n%s\n\nRegion B: %s:%d-%d\n%s\n",
		args.FileA, args.StartLineA, args.EndLineA, textA,
		args.FileB, args.StartLineB, args.EndLineB, textB)

	return s.toolResult(id, text)
}

func readLines(path string, startLine, endLine int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := splitLines(data)
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	var result string
	for i := startLine - 1; i < endLine; i++ {
		result += lines[i] + "\n"
	}
	return result, nil
}

func splitLines(data []byte) []string {
	var lines []string
	start := 0
	for i, b := range data {
		if b == '\n' {
			lines = append(lines, string(data[start:i]))
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, string(data[start:]))
	}
	return lines
}

// --- Tool definitions ---

func (s *Server) toolDefinitions() []toolDef {
	return []toolDef{
		{
			Name:        "scan",
			Description: "Scan directories for code clones. Supports Go, JavaScript/TypeScript, and Ruby. Returns a summary of findings with a scan_id for further queries.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"paths": {
						"type": "array",
						"items": {"type": "string"},
						"description": "Directories or files to scan. Defaults to current directory."
					},
					"min_score": {
						"type": "number",
						"description": "Minimum confidence score (0.0-1.0). Default: 0.15"
					},
					"max_results": {
						"type": "integer",
						"description": "Maximum findings to return. 0 = no limit."
					}
				}
			}`),
		},
		{
			Name:        "list_findings",
			Description: "List clone findings from a previous scan. Supports pagination and filtering by minimum score.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"scan_id": {
						"type": "string",
						"description": "Scan ID returned by the scan tool"
					},
					"min_score": {
						"type": "number",
						"description": "Filter by minimum score"
					},
					"limit": {
						"type": "integer",
						"description": "Maximum findings to return (default: 20)"
					},
					"offset": {
						"type": "integer",
						"description": "Skip first N findings (for pagination)"
					}
				},
				"required": ["scan_id"]
			}`),
		},
		{
			Name:        "explain_finding",
			Description: "Get detailed explanation of a specific clone finding, including normalized form, scores, and refactoring suggestions.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"scan_id": {
						"type": "string",
						"description": "Scan ID from a previous scan"
					},
					"finding_id": {
						"type": "string",
						"description": "Finding ID (e.g., F-a1b2c3d4e5)"
					}
				},
				"required": ["scan_id", "finding_id"]
			}`),
		},
		{
			Name:        "compare_regions",
			Description: "Compare two code regions side by side. Shows the source code from both regions for manual comparison.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_a":       {"type": "string", "description": "Path to first file"},
					"start_line_a": {"type": "integer", "description": "Start line in first file"},
					"end_line_a":   {"type": "integer", "description": "End line in first file"},
					"file_b":       {"type": "string", "description": "Path to second file"},
					"start_line_b": {"type": "integer", "description": "Start line in second file"},
					"end_line_b":   {"type": "integer", "description": "End line in second file"}
				},
				"required": ["file_a", "start_line_a", "end_line_a", "file_b", "start_line_b", "end_line_b"]
			}`),
		},
	}
}

// --- Response helpers ---

func (s *Server) respond(id any, result any) *jsonrpcResponse {
	return &jsonrpcResponse{JSONRPC: "2.0", ID: id, Result: result}
}

func (s *Server) respondError(id any, code int, message string, data any) *jsonrpcResponse {
	return &jsonrpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: code, Message: message, Data: data}}
}

func (s *Server) toolResult(id any, text string) *jsonrpcResponse {
	return s.respond(id, callToolResult{Content: []contentBlock{{Type: "text", Text: text}}})
}

func (s *Server) toolError(id any, msg string) *jsonrpcResponse {
	return s.respond(id, callToolResult{Content: []contentBlock{{Type: "text", Text: "Error: " + msg}}, IsError: true})
}
