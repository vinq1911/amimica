// Package mcp implements an MCP (Model Context Protocol) server that exposes
// Amimica's clone detection as tools for editor and agent integration.
// It uses the stdio transport (JSON-RPC over stdin/stdout).
//
// Output is optimized for token efficiency: compact format by default,
// abbreviated codes for clone types and refactor hints, common path
// prefixes stripped, normalized forms truncated.
package mcp

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/user/amimica/internal/config"
	"github.com/user/amimica/internal/engine"
	"github.com/user/amimica/internal/model"
	"github.com/user/amimica/internal/report"
)

// Clone type short codes (saves ~10 tokens per finding vs full names).
// Documented in tool descriptions so the LLM knows the mapping.
var typeCode = map[string]string{
	"exact":          "EX",
	"renamed":        "RN",
	"near_duplicate": "ND",
	"pattern":        "PT",
}

// Refactor hint short codes.
var hintCode = map[string]string{
	"extract_helper":    "EH",
	"generic_func":      "GF",
	"table_driven":      "TD",
	"shared_validator":  "SV",
	"adapter_mapper":    "AM",
	"interface_extract": "IE",
	"config_driven":     "CD",
}

// Server is the MCP server that handles JSON-RPC requests over stdio.
type Server struct {
	cfg     *config.Config
	log     *slog.Logger
	results map[string]*report.Result
	mu      sync.RWMutex
}

func NewServer(cfg *config.Config, log *slog.Logger) *Server {
	return &Server{cfg: cfg, log: log, results: make(map[string]*report.Result)}
}

// --- JSON-RPC types ---

type jsonrpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonrpcResponse struct {
	JSONRPC string    `json:"jsonrpc"`
	ID      any       `json:"id,omitempty"`
	Result  any       `json:"result,omitempty"`
	Error   *rpcError `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

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
				return nil
			}
			return fmt.Errorf("read stdin: %w", err)
		}

		var req jsonrpcRequest
		if err := json.Unmarshal(line, &req); err != nil {
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
		return nil
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
		if req.ID == nil {
			return nil
		}
		return s.respondError(req.ID, -32601, "unknown method: "+req.Method, nil)
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
		return s.respondError(id, -32601, "unknown tool: "+params.Name, nil)
	}
}

// --- Tool handlers ---

type scanArgs struct {
	Paths      []string `json:"paths"`
	MinScore   float64  `json:"min_score,omitempty"`
	MaxResults int      `json:"max_results,omitempty"`
}

func (s *Server) handleScan(id any, raw json.RawMessage) *jsonrpcResponse {
	var args scanArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return s.respondError(id, -32602, "invalid arguments: "+err.Error(), nil)
	}
	if len(args.Paths) == 0 {
		args.Paths = []string{"."}
	}

	cfg := *s.cfg
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

	scanID := fmt.Sprintf("scan-%d", len(s.results)+1)
	s.mu.Lock()
	s.results[scanID] = result
	s.mu.Unlock()

	// Count visible findings.
	visible := 0
	for _, f := range result.Findings {
		if !f.Suppressed {
			visible++
		}
	}

	// Find common path prefix to strip from region paths.
	prefix := commonPrefix(result.Findings)

	// Build compact summary.
	var b strings.Builder
	fmt.Fprintf(&b, "%d files %d funcs %dms | %d clones | sid:%s\n",
		result.FilesScanned, result.FuncsAnalyzed,
		result.Duration.Milliseconds(), visible, scanID)

	count := 0
	for _, f := range result.Findings {
		if f.Suppressed {
			continue
		}
		count++
		if count > 5 {
			fmt.Fprintf(&b, "+%d more → list_findings sid:%s\n", visible-5, scanID)
			break
		}
		tc := typeCode[f.Type.String()]
		fmt.Fprintf(&b, "#%d %.2f %s %dr", count, f.Score.Composite, tc, len(f.Regions))

		// Compact refactor hint.
		if len(f.RefactorHints) > 0 {
			hc := hintCode[f.RefactorHints[0].Category.String()]
			if hc != "" {
				fmt.Fprintf(&b, " →%s", hc)
			}
		}
		b.WriteByte('\n')

		for _, r := range f.Regions {
			path := stripPrefix(r.File, prefix)
			fn := r.FuncName
			if r.Receiver != "" {
				fn = r.Receiver + "." + fn
			}
			fmt.Fprintf(&b, "  %s:%d-%d %s\n", path, r.StartLine, r.EndLine, fn)
		}
	}

	return s.toolResult(id, b.String())
}

type listFindingsArgs struct {
	ScanID   string  `json:"scan_id"`
	MinScore float64 `json:"min_score,omitempty"`
	Limit    int     `json:"limit,omitempty"`
	Offset   int     `json:"offset,omitempty"`
}

// lookupScan retrieves a stored scan result or returns an error response.
func (s *Server) lookupScan(id any, scanID string) (*report.Result, *jsonrpcResponse) {
	s.mu.RLock()
	result, ok := s.results[scanID]
	s.mu.RUnlock()
	if !ok {
		return nil, s.toolError(id, "scan_id not found: "+scanID)
	}
	return result, nil
}

func (s *Server) handleListFindings(id any, raw json.RawMessage) *jsonrpcResponse {
	var args listFindingsArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return s.respondError(id, -32602, "invalid arguments", nil)
	}

	result, errResp := s.lookupScan(id, args.ScanID)
	if errResp != nil {
		return errResp
	}

	if args.Limit == 0 {
		args.Limit = 20
	}

	prefix := commonPrefix(result.Findings)

	var b strings.Builder
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
			fmt.Fprintf(&b, "+%d more (offset=%d)\n", count-args.Offset-shown, args.Offset+shown)
			break
		}
		shown++

		fid := "F-" + f.ID.String()[:10]
		tc := typeCode[f.Type.String()]
		fmt.Fprintf(&b, "#%d %s %.2f %s %dr", count, fid, f.Score.Composite, tc, len(f.Regions))
		if len(f.RefactorHints) > 0 {
			fmt.Fprintf(&b, " →%s", hintCode[f.RefactorHints[0].Category.String()])
		}
		b.WriteByte('\n')

		for _, r := range f.Regions {
			path := stripPrefix(r.File, prefix)
			fmt.Fprintf(&b, "  %s:%d-%d %s\n", path, r.StartLine, r.EndLine, r.FuncName)
		}
	}

	if b.Len() == 0 {
		b.WriteString("No findings match criteria.")
	}

	return s.toolResult(id, b.String())
}

type explainFindingArgs struct {
	ScanID    string `json:"scan_id"`
	FindingID string `json:"finding_id"`
	Verbose   bool   `json:"verbose,omitempty"`
}

func (s *Server) handleExplainFinding(id any, raw json.RawMessage) *jsonrpcResponse {
	var args explainFindingArgs
	if err := json.Unmarshal(raw, &args); err != nil {
		return s.respondError(id, -32602, "invalid arguments", nil)
	}

	result, errResp := s.lookupScan(id, args.ScanID)
	if errResp != nil {
		return errResp
	}

	for _, f := range result.Findings {
		fid := "F-" + f.ID.String()[:10]
		if fid != args.FindingID && f.ID.String() != args.FindingID {
			continue
		}

		var b strings.Builder
		tc := typeCode[f.Type.String()]
		fmt.Fprintf(&b, "%s %s %.2f conf=%.2f sim=%.2f imp=%.2f ref=%.2f\n",
			fid, tc, f.Score.Composite,
			f.Score.Confidence, f.Score.Similarity, f.Score.Impact, f.Score.Refactorability)

		for _, r := range f.Regions {
			fmt.Fprintf(&b, "  %s:%d-%d %s\n", r.File, r.StartLine, r.EndLine, r.FuncName)
		}

		// Normalized form: truncated unless verbose.
		if f.Evidence.MatchedNormForm != "" {
			norm := f.Evidence.MatchedNormForm
			if !args.Verbose && len(norm) > 200 {
				norm = norm[:200] + "... (set verbose:true for full)"
			}
			fmt.Fprintf(&b, "norm: %s\n", norm)
		}

		if len(f.RefactorHints) > 0 {
			for _, h := range f.RefactorHints {
				hc := hintCode[h.Category.String()]
				fmt.Fprintf(&b, "hint: %s %.0f%% %s\n", hc, h.Confidence*100, h.Description)
			}
		}

		if len(f.Score.Penalties) > 0 {
			var parts []string
			for _, p := range f.Score.Penalties {
				parts = append(parts, fmt.Sprintf("%s(×%.1f)", p.Reason, p.Factor))
			}
			fmt.Fprintf(&b, "penalties: %s\n", strings.Join(parts, " "))
		}

		return s.toolResult(id, b.String())
	}

	return s.toolError(id, "finding not found: "+args.FindingID)
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

	textA, err := readLines(args.FileA, args.StartLineA, args.EndLineA)
	if err != nil {
		return s.toolError(id, "read: "+err.Error())
	}
	textB, err := readLines(args.FileB, args.StartLineB, args.EndLineB)
	if err != nil {
		return s.toolError(id, "read: "+err.Error())
	}

	text := fmt.Sprintf("A %s:%d-%d\n%s\nB %s:%d-%d\n%s",
		filepath.Base(args.FileA), args.StartLineA, args.EndLineA, textA,
		filepath.Base(args.FileB), args.StartLineB, args.EndLineB, textB)

	return s.toolResult(id, text)
}

// --- Helpers ---

// commonPrefix finds the longest common directory prefix across all finding regions.
func commonPrefix(findings []model.Finding) string {
	if len(findings) == 0 {
		return ""
	}
	var paths []string
	for _, f := range findings {
		for _, r := range f.Regions {
			paths = append(paths, r.File)
		}
	}
	if len(paths) == 0 {
		return ""
	}

	prefix := filepath.Dir(paths[0])
	for _, p := range paths[1:] {
		dir := filepath.Dir(p)
		for !strings.HasPrefix(dir, prefix) && prefix != "" && prefix != "." {
			prefix = filepath.Dir(prefix)
		}
	}

	if prefix == "." || prefix == "" {
		return ""
	}
	return prefix + "/"
}

func stripPrefix(path, prefix string) string {
	if prefix == "" {
		return path
	}
	return strings.TrimPrefix(path, prefix)
}

func readLines(path string, startLine, endLine int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	if startLine < 1 {
		startLine = 1
	}
	if endLine > len(lines) {
		endLine = len(lines)
	}
	var result strings.Builder
	for i := startLine - 1; i < endLine; i++ {
		result.WriteString(lines[i])
		result.WriteByte('\n')
	}
	return result.String(), nil
}

// --- Tool definitions ---
// Tool descriptions include code legends so the LLM can decode compact output.

func (s *Server) toolDefinitions() []toolDef {
	return []toolDef{
		{
			Name: "scan",
			Description: `Scan directories for code clones (Go/JS/TS/Ruby). Returns compact summary + scan_id.
Output codes: EX=exact RN=renamed ND=near-duplicate PT=pattern | EH=extract-helper TD=table-driven IE=interface GF=generic-func
Example output line: "#1 0.77 ND 2r →EH" means finding #1, score 0.77, near-duplicate, 2 regions, suggest extract-helper.`,
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"paths": {"type": "array", "items": {"type": "string"}, "description": "Dirs to scan. Default: [\".\"]"},
					"min_score": {"type": "number", "description": "Min score 0.0-1.0. Default: 0.15"},
					"max_results": {"type": "integer", "description": "Max findings. 0=no limit"}
				}
			}`),
		},
		{
			Name: "list_findings",
			Description: `List findings from a scan. Paginated. Same compact codes as scan.
Codes: EX=exact RN=renamed ND=near-duplicate | EH=extract-helper TD=table-driven IE=interface`,
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"scan_id": {"type": "string", "description": "From scan result"},
					"min_score": {"type": "number"},
					"limit": {"type": "integer", "description": "Default: 20"},
					"offset": {"type": "integer"}
				},
				"required": ["scan_id"]
			}`),
		},
		{
			Name: "explain_finding",
			Description: `Detailed breakdown of a finding. Normalized form truncated by default; set verbose:true for full.
Codes: EX=exact RN=renamed ND=near-duplicate | EH=extract-helper TD=table-driven IE=interface`,
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"scan_id": {"type": "string"},
					"finding_id": {"type": "string", "description": "e.g. F-a1b2c3d4e5"},
					"verbose": {"type": "boolean", "description": "Include full normalized form. Default: false"}
				},
				"required": ["scan_id", "finding_id"]
			}`),
		},
		{
			Name: "compare_regions",
			Description: "Show source code from two regions side by side.",
			InputSchema: json.RawMessage(`{
				"type": "object",
				"properties": {
					"file_a": {"type": "string"},
					"start_line_a": {"type": "integer"},
					"end_line_a": {"type": "integer"},
					"file_b": {"type": "string"},
					"start_line_b": {"type": "integer"},
					"end_line_b": {"type": "integer"}
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
