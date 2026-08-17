// Package objc implements the Language interface for Objective-C source files.
// Handles .m and .mm files with a built-in tokenizer that understands ObjC method
// syntax (- / + prefixed methods), @"string" literals, message passing brackets,
// #import directives, and block syntax.
package objc

import (
	"go/token"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/vinq1911/amimica/internal/config"
	"github.com/vinq1911/amimica/internal/lang"
	"github.com/vinq1911/amimica/internal/model"
)

type Lang struct{}

func New() *Lang { return &Lang{} }

func (l *Lang) Name() string         { return "objc" }
func (l *Lang) Extensions() []string  { return []string{".m", ".mm"} }

func (l *Lang) IsTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "Tests.m") ||
		strings.HasSuffix(base, "Test.m") ||
		strings.HasSuffix(base, "Tests.mm") ||
		strings.Contains(path, "/Tests/") ||
		strings.Contains(path, "/XCTests/")
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

type ocToken struct {
	typ  int
	val  string
	line int
}

const (
	otIdent   = 0
	otKeyword = 1
	otNumber  = 2
	otString  = 3
	otPunct   = 4
	otOp      = 5
)

func tokenize(src []byte) []ocToken {
	var tokens []ocToken
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

		// Preprocessor lines (#import, #pragma, #include, #define, etc.)
		if ch == '#' {
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

		// NSString @"..." or @-keyword.
		if ch == '@' && i+1 < len(src) {
			if src[i+1] == '"' {
				// NSString literal.
				i++ // skip @
				start := i
				i++ // skip "
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
					i++ // closing "
				}
				tokens = append(tokens, ocToken{typ: otString, val: string(src[start:i]), line: line})
				continue
			}
			// @keyword (@interface, @implementation, @end, @protocol, etc.)
			if isIdentStart(src[i+1]) {
				start := i
				i++ // skip @
				for i < len(src) && isIdentPart(src[i]) {
					i++
				}
				tokens = append(tokens, ocToken{typ: otKeyword, val: string(src[start:i]), line: line})
				continue
			}
		}

		// String literals.
		if ch == '"' || ch == '\'' {
			start := i
			quote := ch
			i++
			for i < len(src) && src[i] != quote {
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
			tokens = append(tokens, ocToken{typ: otString, val: string(src[start:i]), line: line})
			continue
		}

		// Numbers.
		if ch >= '0' && ch <= '9' {
			start := i
			i++
			for i < len(src) && isNumPart(src[i]) {
				i++
			}
			// Skip suffixes like f, l, u, etc.
			if i < len(src) && (src[i] == 'f' || src[i] == 'F' || src[i] == 'l' || src[i] == 'L' || src[i] == 'u' || src[i] == 'U') {
				i++
			}
			tokens = append(tokens, ocToken{typ: otNumber, val: string(src[start:i]), line: line})
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
			if isObjCKeyword(word) {
				tokens = append(tokens, ocToken{typ: otKeyword, val: word, line: line})
			} else {
				tokens = append(tokens, ocToken{typ: otIdent, val: word, line: line})
			}
			continue
		}

		// Multi-char operators.
		if i+1 < len(src) {
			bi := string(src[i : i+2])
			if bi == "==" || bi == "!=" || bi == "<=" || bi == ">=" || bi == "&&" || bi == "||" ||
				bi == "++" || bi == "--" || bi == "+=" || bi == "-=" || bi == "*=" || bi == "/=" ||
				bi == "->" || bi == ">>" || bi == "<<" {
				tokens = append(tokens, ocToken{typ: otOp, val: bi, line: line})
				i += 2
				continue
			}
		}

		// Single-char punctuation.
		tokens = append(tokens, ocToken{typ: otPunct, val: string(ch), line: line})
		i++
	}
	return tokens
}

// --- Function finder ---

type funcSpan struct {
	name       string
	receiver   string
	bodyTokens []ocToken
	startLine  int
	endLine    int
}

func findFunctions(tokens []ocToken, src []byte) []funcSpan {
	var funcs []funcSpan
	totalLines := countLines(src)

	i := 0
	for i < len(tokens) {
		t := tokens[i]

		// ObjC method: - (type)name... { body } or + (type)name... { body }
		if (t.val == "-" || t.val == "+") && t.typ == otPunct {
			methodType := t.val
			startLine := t.line
			_ = methodType

			// Scan forward for the opening brace.
			name := ""
			j := i + 1
			for j < len(tokens) && tokens[j].val != "{" && tokens[j].val != ";" {
				if tokens[j].typ == otIdent && name == "" {
					name = tokens[j].val
				}
				j++
			}

			if j < len(tokens) && tokens[j].val == "{" {
				braceEnd := matchBraces(tokens, j)
				if braceEnd > 0 {
					endLine := totalLines
					if braceEnd < len(tokens) {
						endLine = tokens[braceEnd].line
					}
					body := tokens[j : braceEnd+1]
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

		// C-style function: type name( ... ) { body }
		if t.typ == otIdent && i+1 < len(tokens) && tokens[i+1].val == "(" {
			name := t.val
			startLine := t.line

			// Find closing paren.
			pClose := matchDelimiter(tokens, i+1, "(", ")")
			if pClose > 0 && pClose+1 < len(tokens) && tokens[pClose+1].val == "{" {
				braceEnd := matchBraces(tokens, pClose+1)
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

		i++
	}
	return funcs
}

func matchBraces(tokens []ocToken, start int) int {
	return matchDelimiter(tokens, start, "{", "}")
}

func matchDelimiter(tokens []ocToken, start int, open, close string) int {
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

func normalizeTokens(tokens []ocToken, level model.NormalizationLevel) []model.NormToken {
	var result []model.NormToken
	tracker := lang.NewIdentTracker()

	for _, t := range tokens {
		switch t.typ {
		case otString:
			if level >= model.NormLight {
				result = append(result, model.NormToken{Kind: token.STRING, Norm: "$STR", OrigLit: t.val})
			} else {
				result = append(result, model.NormToken{Kind: token.STRING, Norm: t.val, OrigLit: t.val})
			}
		case otNumber:
			if level >= model.NormLight {
				result = append(result, model.NormToken{Kind: token.INT, Norm: "$NUM", OrigLit: t.val})
			} else {
				result = append(result, model.NormToken{Kind: token.INT, Norm: t.val, OrigLit: t.val})
			}
		case otKeyword:
			result = append(result, model.NormToken{Kind: token.IDENT, Norm: t.val, OrigLit: t.val})
		case otIdent:
			if level >= model.NormStrong && !isWellKnown(t.val) {
				result = append(result, model.NormToken{Kind: token.IDENT, Norm: tracker.Get(t.val), OrigLit: t.val})
			} else {
				result = append(result, model.NormToken{Kind: token.IDENT, Norm: t.val, OrigLit: t.val})
			}
		case otPunct, otOp:
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

func isIdentStart(ch byte) bool { return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' }
func isIdentPart(ch byte) bool  { return isIdentStart(ch) || ch >= '0' && ch <= '9' }
func isNumPart(ch byte) bool {
	return ch >= '0' && ch <= '9' || ch == '.' || ch == 'x' || ch == 'X' ||
		ch >= 'a' && ch <= 'f' || ch >= 'A' && ch <= 'F' || ch == '_' || ch == 'e' || ch == 'E'
}

func isObjCKeyword(word string) bool {
	switch word {
	case "auto", "break", "case", "char", "const", "continue", "default", "do",
		"double", "else", "enum", "extern", "float", "for", "goto", "if",
		"int", "long", "register", "return", "short", "signed", "sizeof", "static",
		"struct", "switch", "typedef", "union", "unsigned", "void", "volatile", "while",
		// ObjC additions
		"id", "nil", "Nil", "BOOL", "YES", "NO", "SEL", "IMP", "Class",
		"self", "super", "in", "out", "inout", "bycopy", "byref", "oneway",
		"strong", "weak", "copy", "assign", "retain", "nonatomic", "atomic",
		"readonly", "readwrite", "nonnull", "nullable",
		"instancetype", "typeof", "block":
		return true
	}
	return false
}

func isWellKnown(name string) bool {
	switch name {
	case "self", "super", "nil", "Nil", "YES", "NO", "NULL",
		"NSLog", "NSString", "NSMutableString", "NSArray", "NSMutableArray",
		"NSDictionary", "NSMutableDictionary", "NSSet", "NSMutableSet",
		"NSNumber", "NSInteger", "NSUInteger", "CGFloat",
		"NSError", "NSException", "NSData", "NSDate", "NSURL",
		"NSObject", "NSNotificationCenter", "NSUserDefaults",
		"UIView", "UIViewController", "UITableView", "UICollectionView",
		"UILabel", "UIButton", "UIImage", "UIImageView",
		"dispatch_async", "dispatch_sync", "dispatch_get_main_queue",
		"dispatch_queue_t", "dispatch_once",
		"CGRectMake", "CGSizeMake", "CGPointMake",
		"NSLocalizedString",
		"true", "false", "TRUE", "FALSE":
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
	default:   return token.IDENT
	}
}
