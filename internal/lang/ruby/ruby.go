// Package ruby implements the Language interface for Ruby source files.
// Uses a token-based approach similar to the JavaScript implementation.
package ruby

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

func (l *Lang) Name() string         { return "ruby" }
func (l *Lang) Extensions() []string { return []string{".rb", ".rake", ".gemspec"} }

func (l *Lang) IsTestFile(path string) bool {
	base := filepath.Base(path)
	return strings.HasSuffix(base, "_test.rb") ||
		strings.HasSuffix(base, "_spec.rb") ||
		strings.HasPrefix(base, "test_") ||
		strings.Contains(path, "/test/") ||
		strings.Contains(path, "/tests/") ||
		strings.Contains(path, "/spec/")
}

func (l *Lang) IsGeneratedFile(content []byte) bool {
	return lang.IsGeneratedContent(content)
}

func (l *Lang) ParseAndExtract(sf model.SourceFile, cfg *config.Config, level model.NormalizationLevel, log *slog.Logger) ([]model.NormalizedUnit, error) {
	content, err := os.ReadFile(sf.Path)
	if err != nil {
		return nil, err
	}
	rawTokens := tokenize(content)
	if len(rawTokens) == 0 {
		return nil, nil
	}
	methods := findMethods(rawTokens, content)
	var units []model.NormalizedUnit

	for _, m := range methods {
		// Check for amimica-ignore directive.
		if lang.HasIgnoreComment(content, m.startLine) {
			continue
		}

		normBody := normalizeTokens(m.bodyTokens, level)
		stmtCount := estimateStatements(normBody)
		if stmtCount < cfg.Analysis.MinStatements {
			continue
		}
		lines := m.endLine - m.startLine + 1
		if lines < cfg.Analysis.MinLines {
			continue
		}
		region := model.SourceRegion{
			File:      sf.RelPath,
			StartLine: m.startLine,
			EndLine:   m.endLine,
			FuncName:  m.name,
			Receiver:  m.receiver,
		}
		units = append(units, lang.MakeUnit(normBody, region, model.UnitFunction, level, stmtCount))

		if stmtCount >= cfg.Analysis.WindowMinFunctionSize {
			stmts := lang.SplitByDelimiters(normBody, []string{"end", "\n"})
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
					StartLine: m.startLine + i,
					EndLine:   m.startLine + i + winSize,
					FuncName:  m.name,
				}
				units = append(units, lang.MakeUnit(winTokens, wr, model.UnitWindow, level, winSize))
			}
		}
	}
	return units, nil
}

// --- Tokenizer ---

type rbToken struct {
	typ  int
	val  string
	line int
}

const (
	rtIdent   = 0
	rtKeyword = 1
	rtNumber  = 2
	rtString  = 3
	rtSymbol  = 4
	rtPunct   = 5
	rtOp      = 6
)

func tokenize(src []byte) []rbToken {
	var tokens []rbToken
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

		// Single-line comment.
		if ch == '#' {
			for i < len(src) && src[i] != '\n' {
				i++
			}
			continue
		}

		// Multi-line =begin/=end.
		if ch == '=' && i == 0 || (i > 0 && src[i-1] == '\n') {
			if i+5 < len(src) && string(src[i:i+6]) == "=begin" {
				for i < len(src) {
					if src[i] == '\n' {
						line++
					}
					if i > 0 && src[i-1] == '\n' && i+3 < len(src) && string(src[i:i+4]) == "=end" {
						i += 4
						break
					}
					i++
				}
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
			tokens = append(tokens, rbToken{typ: rtString, val: string(src[start:i]), line: line})
			continue
		}

		// Heredoc (simplified: <<~IDENTIFIER ... IDENTIFIER).
		if ch == '<' && i+1 < len(src) && src[i+1] == '<' {
			// Skip heredocs, treat as a string.
			start := i
			i += 2
			if i < len(src) && (src[i] == '~' || src[i] == '-') {
				i++
			}
			// Read identifier.
			idStart := i
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			if i > idStart {
				heredocEnd := string(src[idStart:i])
				// Scan for closing identifier on its own line.
				for i < len(src) {
					if src[i] == '\n' {
						line++
						i++
						_ = i // lineStart
						for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
							i++
						}
						end := i
						for end < len(src) && src[end] != '\n' {
							end++
						}
						if strings.TrimSpace(string(src[i:end])) == heredocEnd {
							i = end
							break
						}
					} else {
						i++
					}
				}
				tokens = append(tokens, rbToken{typ: rtString, val: string(src[start:i]), line: line})
				continue
			}
			// Not a heredoc, back up.
			i = start
		}

		// Symbols.
		if ch == ':' && i+1 < len(src) && isIdentStart(src[i+1]) {
			start := i
			i++ // skip :
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			tokens = append(tokens, rbToken{typ: rtSymbol, val: string(src[start:i]), line: line})
			continue
		}

		// Numbers.
		if ch >= '0' && ch <= '9' {
			start := i
			i++
			for i < len(src) && (src[i] >= '0' && src[i] <= '9' || src[i] == '.' || src[i] == '_' || src[i] == 'x' || src[i] == 'e') {
				i++
			}
			tokens = append(tokens, rbToken{typ: rtNumber, val: string(src[start:i]), line: line})
			continue
		}

		// Identifiers and keywords.
		if isIdentStart(ch) {
			start := i
			i++
			for i < len(src) && (isIdentPart(src[i]) || src[i] == '?' || src[i] == '!') {
				i++
			}
			word := string(src[start:i])
			if isRubyKeyword(word) {
				tokens = append(tokens, rbToken{typ: rtKeyword, val: word, line: line})
			} else {
				tokens = append(tokens, rbToken{typ: rtIdent, val: word, line: line})
			}
			continue
		}

		// Instance/class variables.
		if ch == '@' {
			start := i
			i++
			if i < len(src) && src[i] == '@' {
				i++ // @@class_var
			}
			for i < len(src) && isIdentPart(src[i]) {
				i++
			}
			tokens = append(tokens, rbToken{typ: rtIdent, val: string(src[start:i]), line: line})
			continue
		}

		// Multi-char operators.
		if i+1 < len(src) {
			bi := string(src[i : i+2])
			if bi == "==" || bi == "!=" || bi == "<=" || bi == ">=" || bi == "&&" || bi == "||" ||
				bi == "+=" || bi == "-=" || bi == "*=" || bi == "/=" || bi == "**" || bi == "=>" ||
				bi == ".." || bi == "::" || bi == "<<" || bi == ">>" || bi == "<=>" {
				tokens = append(tokens, rbToken{typ: rtOp, val: bi, line: line})
				i += 2
				continue
			}
		}

		tokens = append(tokens, rbToken{typ: rtPunct, val: string(ch), line: line})
		i++
	}
	return tokens
}

// --- Method finder ---

type methodSpan struct {
	name       string
	receiver   string
	bodyTokens []rbToken
	startLine  int
	endLine    int
}

func findMethods(tokens []rbToken, src []byte) []methodSpan {
	var methods []methodSpan

	for i := 0; i < len(tokens); i++ {
		t := tokens[i]

		// Pattern: def [self.]name ... end
		if t.typ == rtKeyword && t.val == "def" {
			startLine := t.line
			i++
			if i >= len(tokens) {
				break
			}

			var name, receiver string
			if tokens[i].typ == rtIdent || tokens[i].typ == rtKeyword {
				name = tokens[i].val
				i++
				// Check for self.method.
				if i < len(tokens) && tokens[i].val == "." {
					receiver = name
					i++
					if i < len(tokens) {
						name = tokens[i].val
						i++
					}
				}
			}

			// Collect body tokens until matching "end".
			// Only count block-opening keywords when they appear at the start of a
			// logical line (not as trailing modifiers like `raise X unless cond`).
			depth := 1
			bodyStart := i
			for i < len(tokens) && depth > 0 {
				if tokens[i].typ == rtKeyword {
					switch tokens[i].val {
					case "def", "do", "class", "module", "begin", "case":
						depth++
					case "if", "unless", "while", "until", "for":
						// Only opens a block if it's not a modifier (i.e., not preceded
						// by an expression on the same line). Heuristic: if the previous
						// token on the same line is not a keyword/punct that starts a line,
						// it's a modifier.
						if i > bodyStart && tokens[i-1].line == tokens[i].line &&
							tokens[i-1].typ != rtKeyword && tokens[i-1].val != ";" {
							// Modifier form — don't increase depth.
						} else {
							depth++
						}
					case "end":
						depth--
					}
				}
				if depth > 0 {
					i++
				}
			}

			endLine := startLine
			if i < len(tokens) {
				endLine = tokens[i].line
			}

			body := tokens[bodyStart:i]
			if len(body) > 0 {
				methods = append(methods, methodSpan{
					name:       name,
					receiver:   receiver,
					bodyTokens: body,
					startLine:  startLine,
					endLine:    endLine,
				})
			}

			// Don't i++ here — the for-loop's own i++ will advance past "end".
		}
	}
	return methods
}

// --- Normalizer ---

func normalizeTokens(tokens []rbToken, level model.NormalizationLevel) []model.NormToken {
	var result []model.NormToken
	tracker := lang.NewIdentTracker()

	for _, t := range tokens {
		switch t.typ {
		case rtString:
			if level >= model.NormLight {
				result = append(result, model.NormToken{Kind: token.STRING, Norm: "$STR", OrigLit: t.val})
			} else {
				result = append(result, model.NormToken{Kind: token.STRING, Norm: t.val, OrigLit: t.val})
			}
		case rtSymbol:
			if level >= model.NormLight {
				result = append(result, model.NormToken{Kind: token.STRING, Norm: "$SYM", OrigLit: t.val})
			} else {
				result = append(result, model.NormToken{Kind: token.STRING, Norm: t.val, OrigLit: t.val})
			}
		case rtNumber:
			if level >= model.NormLight {
				result = append(result, model.NormToken{Kind: token.INT, Norm: "$NUM", OrigLit: t.val})
			} else {
				result = append(result, model.NormToken{Kind: token.INT, Norm: t.val, OrigLit: t.val})
			}
		case rtKeyword:
			result = append(result, model.NormToken{Kind: token.IDENT, Norm: t.val, OrigLit: t.val})
		case rtIdent:
			if level >= model.NormStrong && !isRubyWellKnown(t.val) {
				result = append(result, model.NormToken{Kind: token.IDENT, Norm: tracker.Get(t.val), OrigLit: t.val})
			} else {
				result = append(result, model.NormToken{Kind: token.IDENT, Norm: t.val, OrigLit: t.val})
			}
		case rtPunct, rtOp:
			result = append(result, model.NormToken{Kind: punctToKind(t.val), Norm: t.val, OrigLit: t.val})
		}
	}
	return result
}

// --- Helpers ---

func estimateStatements(tokens []model.NormToken) int {
	// In Ruby, statements are separated by newlines and keywords like raise, return, if, unless.
	// Count tokens that typically start statements.
	count := 0
	for _, t := range tokens {
		switch t.Norm {
		case "end", ";", "raise", "return", "if", "unless", "while", "until", "yield":
			count++
		case "=":
			count++ // assignment is a statement
		}
	}
	if count == 0 && len(tokens) > 0 {
		count = len(tokens) / 4
	}
	return count
}

func isIdentStart(ch byte) bool { return ch >= 'a' && ch <= 'z' || ch >= 'A' && ch <= 'Z' || ch == '_' }
func isIdentPart(ch byte) bool  { return isIdentStart(ch) || ch >= '0' && ch <= '9' }

func isRubyKeyword(word string) bool {
	switch word {
	case "BEGIN", "END", "alias", "and", "begin", "break", "case", "class",
		"def", "defined?", "do", "else", "elsif", "end", "ensure", "false",
		"for", "if", "in", "module", "next", "nil", "not", "or", "redo",
		"rescue", "retry", "return", "self", "super", "then", "true",
		"undef", "unless", "until", "when", "while", "yield",
		"raise", "require", "require_relative", "include", "extend", "prepend",
		"attr_reader", "attr_writer", "attr_accessor", "private", "protected", "public":
		return true
	}
	return false
}

func isRubyWellKnown(name string) bool {
	switch name {
	case "puts", "print", "p", "pp", "raise", "require", "require_relative",
		"include", "extend", "prepend", "attr_reader", "attr_writer", "attr_accessor",
		"nil", "true", "false", "self", "ARGV", "STDIN", "STDOUT", "STDERR",
		"Kernel", "Object", "Class", "Module", "Enumerable", "Comparable",
		"Array", "Hash", "String", "Integer", "Float", "Symbol", "Proc", "Lambda",
		"File", "IO", "Dir", "Regexp", "Range", "Time", "Struct", "OpenStruct":
		return true
	}
	return false
}

func punctToKind(val string) token.Token {
	switch val {
	case "(":
		return token.LPAREN
	case ")":
		return token.RPAREN
	case "{":
		return token.LBRACE
	case "}":
		return token.RBRACE
	case "[":
		return token.LBRACK
	case "]":
		return token.RBRACK
	case ";":
		return token.SEMICOLON
	case ",":
		return token.COMMA
	case ".":
		return token.PERIOD
	case ":":
		return token.COLON
	case "=":
		return token.ASSIGN
	case "+":
		return token.ADD
	case "-":
		return token.SUB
	case "*":
		return token.MUL
	case "/":
		return token.QUO
	case "<":
		return token.LSS
	case ">":
		return token.GTR
	case "!":
		return token.NOT
	case "|":
		return token.OR
	case "&":
		return token.AND
	default:
		return token.IDENT
	}
}
