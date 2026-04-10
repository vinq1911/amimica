// Package csharp implements the Language interface for C# source files.
// Handles .cs files with a built-in tokenizer that understands C# syntax including
// verbatim strings (@"..."), interpolated strings ($"...{expr}..."), properties,
// lambdas, async/await, nullable operators (?., ??), and LINQ expressions.
package csharp

import (
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/user/amimica/internal/config"
	"github.com/user/amimica/internal/lang"
	"github.com/user/amimica/internal/model"
)

type Lang struct{}

func New() *Lang { return &Lang{} }

func (l *Lang) Name() string         { return "csharp" }
func (l *Lang) Extensions() []string { return []string{".cs"} }

func (l *Lang) IsTestFile(path string) bool {
	base := filepath.Base(path)
	dir := filepath.Dir(path)
	return strings.HasSuffix(base, "Test.cs") ||
		strings.HasSuffix(base, "Tests.cs") ||
		strings.HasSuffix(base, "_test.cs") ||
		strings.Contains(dir, ".Tests") ||
		strings.Contains(dir, ".Test") ||
		strings.Contains(path, "/Tests/") ||
		strings.Contains(path, "/test/")
}

func (l *Lang) IsGeneratedFile(content []byte) bool {
	return lang.IsGeneratedContent(content)
}

// amimica-ignore: language-specific entry point; shared structure is intentional
func (l *Lang) ParseAndExtract(sf model.SourceFile, cfg *config.Config, level model.NormalizationLevel, log *slog.Logger) ([]model.NormalizedUnit, error) {
	content, err := os.ReadFile(sf.Path)
	if err != nil {
		return nil, err
	}
	rawTokens := tokenize(content)
	if len(rawTokens) == 0 {
		return nil, nil
	}
	funcs := findFunctions(rawTokens, content)
	var units []model.NormalizedUnit

	for _, fn := range funcs {
		if lang.HasIgnoreComment(content, fn.startLine) {
			continue
		}
		normBody := normalizeTokens(fn.bodyTokens, level)
		stmtCount := estimateStatements(normBody)
		if stmtCount < cfg.Analysis.MinStatements {
			continue
		}
		lines := fn.endLine - fn.startLine + 1
		if lines < cfg.Analysis.MinLines {
			continue
		}
		region := model.SourceRegion{
			File:      sf.RelPath,
			StartLine: fn.startLine,
			EndLine:   fn.endLine,
			FuncName:  fn.name,
			Receiver:  fn.receiver,
		}
		units = append(units, lang.MakeUnit(normBody, region, model.UnitFunction, level, stmtCount))

		if stmtCount >= cfg.Analysis.WindowMinFunctionSize {
			stmts := lang.SplitByDelimiters(normBody, []string{";", "}"})
			winSize := cfg.Analysis.WindowSize
			for i := 0; i <= len(stmts)-winSize; i++ {
				var winTokens []model.NormToken
				for _, s := range stmts[i : i+winSize] {
					winTokens = append(winTokens, s...)
				}
				if len(winTokens) < 5 {
					continue
				}
				wr := model.SourceRegion{
					File:      sf.RelPath,
					StartLine: fn.startLine + i,
					EndLine:   fn.startLine + i + winSize,
					FuncName:  fn.name,
				}
				units = append(units, lang.MakeUnit(winTokens, wr, model.UnitWindow, level, winSize))
			}
		}
	}
	return units, nil
}

// --- Tokenizer ---

type csToken struct {
	typ  int
	val  string
	line int
}

const (
	ctIdent   = 0
	ctKeyword = 1
	ctNumber  = 2
	ctString  = 3
	ctPunct   = 4
	ctOp      = 5
)

func tokenize(src []byte) []csToken {
	var tokens []csToken
	i := 0
	line := 1

	for i < len(src) {
		ch := src[i]

		if ch == '\n' {
			line++
			i++
			continue
		}
		if ch == ' ' || ch == '\t' || ch == '\r' {
			i++
			continue
		}

		// Preprocessor directives (#region, #pragma, #if, #endif, etc.)
		if ch == '#' && (i == 0 || src[i-1] == '\n') {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}

		// Single-line comment.
		if ch == '/' && i+1 < len(src) && src[i+1] == '/' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}

		// Multi-line comment.
		if ch == '/' && i+1 < len(src) && src[i+1] == '*' {
			i += 2
			for i+1 < len(src) {
				if src[i] == '\n' {
					line++
				}
				if src[i] == '*' && src[i+1] == '/' {
					i += 2
					break
				}
				i++
			}
			continue
		}

		// Verbatim string @"..."
		if ch == '@' && i+1 < len(src) && src[i+1] == '"' {
			start := i
			i += 2 // skip @"
			for i < len(src) {
				if src[i] == '\n' {
					line++
				}
				if src[i] == '"' {
					if i+1 < len(src) && src[i+1] == '"' {
						i += 2 // escaped ""
						continue
					}
					i++ // closing "
					break
				}
				i++
			}
			tokens = append(tokens, csToken{typ: ctString, val: string(src[start:i]), line: line})
			continue
		}

		// Interpolated string $"...{expr}..."
		if ch == '$' && i+1 < len(src) && src[i+1] == '"' {
			start := i
			i += 2 // skip $"
			depth := 0
			for i < len(src) {
				if src[i] == '\n' {
					line++
				}
				if src[i] == '\\' && i+1 < len(src) {
					i += 2
					continue
				}
				if src[i] == '{' {
					depth++
					i++
					continue
				}
				if src[i] == '}' && depth > 0 {
					depth--
					i++
					continue
				}
				if src[i] == '"' && depth == 0 {
					i++ // closing "
					break
				}
				i++
			}
			tokens = append(tokens, csToken{typ: ctString, val: string(src[start:i]), line: line})
			continue
		}

		// Raw string literals """..."""
		if ch == '"' && i+2 < len(src) && src[i+1] == '"' && src[i+2] == '"' {
			start := i
			i += 3
			for i+2 < len(src) {
				if src[i] == '\n' {
					line++
				}
				if src[i] == '"' && src[i+1] == '"' && src[i+2] == '"' {
					i += 3
					break
				}
				i++
			}
			tokens = append(tokens, csToken{typ: ctString, val: string(src[start:i]), line: line})
			continue
		}

		// Regular string.
		if ch == '"' {
			start := i
			i++
			for i < len(src) && src[i] != '"' {
				if src[i] == '\\' && i+1 < len(src) {
					i++
				}
				if src[i] == '\n' {
					line++
				}
				i++
			}
			if i < len(src) {
				i++
			}
			tokens = append(tokens, csToken{typ: ctString, val: string(src[start:i]), line: line})
			continue
		}

		// Char literal.
		if ch == '\'' {
			start := i
			i++
			for i < len(src) && src[i] != '\'' {
				if src[i] == '\\' && i+1 < len(src) {
					i++
				}
				i++
			}
			if i < len(src) {
				i++
			}
			tokens = append(tokens, csToken{typ: ctString, val: string(src[start:i]), line: line})
			continue
		}

		// Numbers.
		if ch >= '0' && ch <= '9' {
			start := i
			i++
			for i < len(src) && isNumPart(src[i]) {
				i++
			}
			// Skip suffixes (f, d, m, L, UL, etc.)
			for i < len(src) && (src[i] == 'f' || src[i] == 'F' || src[i] == 'd' || src[i] == 'D' ||
				src[i] == 'm' || src[i] == 'M' || src[i] == 'l' || src[i] == 'L' ||
				src[i] == 'u' || src[i] == 'U') {
				i++
			}
			tokens = append(tokens, csToken{typ: ctNumber, val: string(src[start:i]), line: line})
			continue
		}

		// Identifiers and keywords.
		if isIdentStart(ch) {
			start := i
			i++
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			word := string(src[start:i])
			if isCSharpKeyword(word) {
				tokens = append(tokens, csToken{typ: ctKeyword, val: word, line: line})
			} else {
				tokens = append(tokens, csToken{typ: ctIdent, val: word, line: line})
			}
			continue
		}

		// Multi-char operators.
		if i+2 < len(src) {
			tri := string(src[i : i+3])
			if tri == ">>=" || tri == "<<=" || tri == "??=" {
				tokens = append(tokens, csToken{typ: ctOp, val: tri, line: line})
				i += 3
				continue
			}
		}
		if i+1 < len(src) {
			bi := string(src[i : i+2])
			if bi == "==" || bi == "!=" || bi == "<=" || bi == ">=" || bi == "&&" || bi == "||" ||
				bi == "++" || bi == "--" || bi == "+=" || bi == "-=" || bi == "*=" || bi == "/=" ||
				bi == "=>" || bi == "?." || bi == "??" || bi == ">>" || bi == "<<" ||
				bi == "&=" || bi == "|=" || bi == "^=" {
				tokens = append(tokens, csToken{typ: ctOp, val: bi, line: line})
				i += 2
				continue
			}
		}

		// Single-char.
		tokens = append(tokens, csToken{typ: ctPunct, val: string(ch), line: line})
		i++
	}
	return tokens
}

// --- Function finder ---

type funcSpan struct {
	name       string
	receiver   string
	bodyTokens []csToken
	startLine  int
	endLine    int
}

func findFunctions(tokens []csToken, src []byte) []funcSpan {
	var funcs []funcSpan
	totalLines := countLines(src)

	i := 0
	for i < len(tokens) {
		// Skip access modifiers and other leading keywords to find the method name.
		// Pattern: [modifiers...] returnType Name ( ... ) { body }
		// Also: Name ( ... ) { body } (constructors)

		if tokens[i].typ == ctIdent && i+1 < len(tokens) && tokens[i+1].val == "(" {
			name := tokens[i].val
			startLine := tokens[i].line

			// Find closing paren.
			pClose := matchDelimiter(tokens, i+1, "(", ")")
			if pClose > 0 && pClose+1 < len(tokens) && tokens[pClose+1].val == "{" {
				braceEnd := matchDelimiter(tokens, pClose+1, "{", "}")
				if braceEnd > 0 {
					endLine := totalLines
					if braceEnd < len(tokens) {
						endLine = tokens[braceEnd].line
					}
					body := tokens[pClose+1 : braceEnd+1]
					if len(body) > 2 {
						funcs = append(funcs, funcSpan{
							name:       name,
							bodyTokens: body,
							startLine:  startLine,
							endLine:    endLine,
						})
					}
					i = braceEnd + 1
					continue
				}
			}
		}

		// Property accessors: get { } set { }
		if tokens[i].typ == ctKeyword && (tokens[i].val == "get" || tokens[i].val == "set") {
			if i+1 < len(tokens) && tokens[i+1].val == "{" {
				braceEnd := matchDelimiter(tokens, i+1, "{", "}")
				if braceEnd > 0 {
					endLine := totalLines
					if braceEnd < len(tokens) {
						endLine = tokens[braceEnd].line
					}
					body := tokens[i+1 : braceEnd+1]
					if len(body) > 4 { // non-trivial accessor
						funcs = append(funcs, funcSpan{
							name:       tokens[i].val,
							bodyTokens: body,
							startLine:  tokens[i].line,
							endLine:    endLine,
						})
					}
					i = braceEnd + 1
					continue
				}
			}
		}

		i++
	}
	return funcs
}

func matchDelimiter(tokens []csToken, start int, open, close string) int {
	if start >= len(tokens) || tokens[start].val != open {
		return -1
	}
	depth := 1
	for j := start + 1; j < len(tokens); j++ {
		if tokens[j].val == open {
			depth++
		}
		if tokens[j].val == close {
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return -1
}

// --- Normalizer ---

func normalizeTokens(tokens []csToken, level model.NormalizationLevel) []model.NormToken {
	var result []model.NormToken
	tracker := lang.NewIdentTracker()

	for _, t := range tokens {
		switch t.typ {
		case ctString:
			if level >= model.NormLight {
				result = append(result, model.NormToken{Kind: token.STRING, Norm: "$STR", OrigLit: t.val})
			} else {
				result = append(result, model.NormToken{Kind: token.STRING, Norm: t.val, OrigLit: t.val})
			}
		case ctNumber:
			if level >= model.NormLight {
				result = append(result, model.NormToken{Kind: token.INT, Norm: "$NUM", OrigLit: t.val})
			} else {
				result = append(result, model.NormToken{Kind: token.INT, Norm: t.val, OrigLit: t.val})
			}
		case ctKeyword:
			result = append(result, model.NormToken{Kind: token.IDENT, Norm: t.val, OrigLit: t.val})
		case ctIdent:
			if level >= model.NormStrong && !isWellKnown(t.val) {
				result = append(result, model.NormToken{Kind: token.IDENT, Norm: tracker.Get(t.val), OrigLit: t.val})
			} else {
				result = append(result, model.NormToken{Kind: token.IDENT, Norm: t.val, OrigLit: t.val})
			}
		case ctPunct, ctOp:
			result = append(result, model.NormToken{Kind: punctToKind(t.val), Norm: t.val, OrigLit: t.val})
		}
	}
	return result
}

// --- Helpers ---

func estimateStatements(tokens []model.NormToken) int {
	count := 0
	for _, t := range tokens {
		if t.Norm == ";" || t.Norm == "}" {
			count++
		}
	}
	if count == 0 && len(tokens) > 0 {
		count = len(tokens) / 5
	}
	return count
}

func countLines(src []byte) int {
	n := 1
	for _, b := range src {
		if b == '\n' {
			n++
		}
	}
	return n
}

func isIdentStart(ch byte) bool { return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' || ch == '@' }
func isIdentPart(ch byte) bool  { return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' || ch >= '0' && ch <= '9' }
func isNumPart(ch byte) bool {
	return ch >= '0' && ch <= '9' || ch == '.' || ch == 'x' || ch == 'X' || ch == 'b' || ch == 'B' ||
		ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F' || ch == '_' || ch == 'e' || ch == 'E'
}

func isCSharpKeyword(word string) bool {
	switch word {
	case "abstract", "as", "base", "bool", "break", "byte", "case", "catch",
		"char", "checked", "class", "const", "continue", "decimal", "default",
		"delegate", "do", "double", "else", "enum", "event", "explicit", "extern",
		"false", "finally", "fixed", "float", "for", "foreach", "goto", "if",
		"implicit", "in", "int", "interface", "internal", "is", "lock", "long",
		"namespace", "new", "null", "object", "operator", "out", "override",
		"params", "private", "protected", "public", "readonly", "ref", "return",
		"sbyte", "sealed", "short", "sizeof", "stackalloc", "static", "string",
		"struct", "switch", "this", "throw", "true", "try", "typeof", "uint",
		"ulong", "unchecked", "unsafe", "ushort", "using", "virtual", "void",
		"volatile", "while",
		// Contextual keywords commonly used
		"async", "await", "dynamic", "get", "set", "var", "value", "yield",
		"where", "nameof", "when", "init", "record", "required", "global":
		return true
	}
	return false
}

func isWellKnown(name string) bool {
	switch name {
	case "Console", "string", "int", "bool", "var", "object", "null", "true", "false",
		"void", "double", "float", "decimal", "long", "byte", "char", "short",
		"Task", "ValueTask", "List", "Dictionary", "HashSet", "Queue", "Stack",
		"IEnumerable", "IList", "IDictionary", "ICollection", "IDisposable",
		"ILogger", "IConfiguration", "IServiceProvider", "IHostedService",
		"Exception", "ArgumentException", "InvalidOperationException", "NullReferenceException",
		"StringBuilder", "DateTime", "DateTimeOffset", "TimeSpan", "Guid",
		"HttpClient", "HttpContext", "HttpRequest", "HttpResponse",
		"JsonSerializer", "JsonConvert",
		"String", "Int32", "Boolean", "Object", "Array",
		"Enumerable", "Queryable", "Math", "Convert", "Activator",
		"CancellationToken", "CancellationTokenSource",
		"Assert", "Fact", "Theory", "Test", "TestMethod":
		return true
	}
	return false
}

func punctToKind(val string) token.Token {
	switch val {
	case "(":  return token.LPAREN
	case ")":  return token.RPAREN
	case "{":  return token.LBRACE
	case "}":  return token.RBRACE
	case "[":  return token.LBRACK
	case "]":  return token.RBRACK
	case ";":  return token.SEMICOLON
	case ",":  return token.COMMA
	case ".":  return token.PERIOD
	case ":":  return token.COLON
	case "=":  return token.ASSIGN
	case "+":  return token.ADD
	case "-":  return token.SUB
	case "*":  return token.MUL
	case "/":  return token.QUO
	case "<":  return token.LSS
	case ">":  return token.GTR
	case "!":  return token.NOT
	case "&":  return token.AND
	case "|":  return token.OR
	case "^":  return token.XOR
	case "~":  return token.XOR
	case "?":  return token.IDENT
	default:   return token.IDENT
	}
}
