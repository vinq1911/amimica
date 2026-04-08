// Package normalize transforms Go AST nodes into normalized token sequences
// at multiple levels of abstraction. Higher normalization levels abstract away
// more details (identifiers, literals, types), enabling detection of
// structurally similar but superficially different code clones.
package normalize

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"

	"github.com/user/amimica/internal/model"
)

// Normalizer transforms Go AST nodes into normalized token sequences.
type Normalizer struct {
	level model.NormalizationLevel
	fset  *token.FileSet
}

// New creates a Normalizer at the specified normalization level.
func New(level model.NormalizationLevel, fset *token.FileSet) *Normalizer {
	return &Normalizer{level: level, fset: fset}
}

// NormalizeFunc normalizes a function declaration into a token sequence.
func (n *Normalizer) NormalizeFunc(fn *ast.FuncDecl) []model.NormToken {
	tracker := newIdentTracker()
	var tokens []model.NormToken

	// Normalize receiver if present.
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		tokens = append(tokens, n.tok(token.LPAREN, "("))
		recv := fn.Recv.List[0]
		if len(recv.Names) > 0 {
			tokens = append(tokens, n.identToken(recv.Names[0].Name, identReceiver, tracker))
		}
		tokens = append(tokens, n.typeTokens(recv.Type, tracker)...)
		tokens = append(tokens, n.tok(token.RPAREN, ")"))
	}

	// Function name — kept at raw/light, normalized at strong+.
	tokens = append(tokens, n.tok(token.FUNC, "func"))
	if n.level >= model.NormStrong {
		tokens = append(tokens, n.rawToken(token.IDENT, "$FUNC"))
	} else {
		tokens = append(tokens, n.rawToken(token.IDENT, fn.Name.Name))
	}

	// Parameters.
	if fn.Type.Params != nil {
		tokens = append(tokens, n.fieldListTokens(fn.Type.Params, identParam, tracker)...)
	}

	// Results.
	if fn.Type.Results != nil {
		tokens = append(tokens, n.fieldListTokens(fn.Type.Results, identResult, tracker)...)
	}

	// Body.
	if fn.Body != nil {
		tokens = append(tokens, n.blockTokens(fn.Body, tracker)...)
	}

	return tokens
}

// NormalizeBlock normalizes a block statement into a token sequence.
func (n *Normalizer) NormalizeBlock(block *ast.BlockStmt) []model.NormToken {
	tracker := newIdentTracker()
	return n.blockTokens(block, tracker)
}

// NormalizeStmts normalizes a sequence of statements.
func (n *Normalizer) NormalizeStmts(stmts []ast.Stmt) []model.NormToken {
	tracker := newIdentTracker()
	var tokens []model.NormToken
	for _, stmt := range stmts {
		tokens = append(tokens, n.stmtTokens(stmt, tracker)...)
	}
	return tokens
}

func (n *Normalizer) blockTokens(block *ast.BlockStmt, t *identTracker) []model.NormToken {
	tokens := []model.NormToken{n.tok(token.LBRACE, "{")}
	for _, stmt := range block.List {
		tokens = append(tokens, n.stmtTokens(stmt, t)...)
	}
	tokens = append(tokens, n.tok(token.RBRACE, "}"))
	return tokens
}

func (n *Normalizer) stmtTokens(stmt ast.Stmt, t *identTracker) []model.NormToken {
	if stmt == nil {
		return nil
	}
	switch s := stmt.(type) {
	case *ast.ReturnStmt:
		tokens := []model.NormToken{n.tok(token.RETURN, "return")}
		for i, expr := range s.Results {
			if i > 0 {
				tokens = append(tokens, n.tok(token.COMMA, ","))
			}
			tokens = append(tokens, n.exprTokens(expr, t)...)
		}
		return tokens

	case *ast.AssignStmt:
		var tokens []model.NormToken
		for i, lhs := range s.Lhs {
			if i > 0 {
				tokens = append(tokens, n.tok(token.COMMA, ","))
			}
			tokens = append(tokens, n.exprTokens(lhs, t)...)
		}
		tokens = append(tokens, n.tok(s.Tok, s.Tok.String()))
		for i, rhs := range s.Rhs {
			if i > 0 {
				tokens = append(tokens, n.tok(token.COMMA, ","))
			}
			tokens = append(tokens, n.exprTokens(rhs, t)...)
		}
		return tokens

	case *ast.ExprStmt:
		return n.exprTokens(s.X, t)

	case *ast.IfStmt:
		tokens := []model.NormToken{n.tok(token.IF, "if")}
		if s.Init != nil {
			tokens = append(tokens, n.stmtTokens(s.Init, t)...)
			tokens = append(tokens, n.tok(token.SEMICOLON, ";"))
		}
		tokens = append(tokens, n.exprTokens(s.Cond, t)...)
		tokens = append(tokens, n.blockTokens(s.Body, t)...)
		if s.Else != nil {
			tokens = append(tokens, n.tok(token.ELSE, "else"))
			tokens = append(tokens, n.stmtTokens(s.Else, t)...)
		}
		return tokens

	case *ast.ForStmt:
		tokens := []model.NormToken{n.tok(token.FOR, "for")}
		if s.Init != nil {
			tokens = append(tokens, n.stmtTokens(s.Init, t)...)
			tokens = append(tokens, n.tok(token.SEMICOLON, ";"))
		}
		if s.Cond != nil {
			tokens = append(tokens, n.exprTokens(s.Cond, t)...)
			tokens = append(tokens, n.tok(token.SEMICOLON, ";"))
		}
		if s.Post != nil {
			tokens = append(tokens, n.stmtTokens(s.Post, t)...)
		}
		tokens = append(tokens, n.blockTokens(s.Body, t)...)
		return tokens

	case *ast.RangeStmt:
		tokens := []model.NormToken{n.tok(token.FOR, "for")}
		if s.Key != nil {
			tokens = append(tokens, n.exprTokens(s.Key, t)...)
		}
		if s.Value != nil {
			tokens = append(tokens, n.tok(token.COMMA, ","))
			tokens = append(tokens, n.exprTokens(s.Value, t)...)
		}
		tokens = append(tokens, n.tok(s.Tok, s.Tok.String()))
		tokens = append(tokens, n.tok(token.RANGE, "range"))
		tokens = append(tokens, n.exprTokens(s.X, t)...)
		tokens = append(tokens, n.blockTokens(s.Body, t)...)
		return tokens

	case *ast.BlockStmt:
		return n.blockTokens(s, t)

	case *ast.DeclStmt:
		return n.declTokens(s.Decl, t)

	case *ast.IncDecStmt:
		tokens := n.exprTokens(s.X, t)
		tokens = append(tokens, n.tok(s.Tok, s.Tok.String()))
		return tokens

	case *ast.SwitchStmt:
		tokens := []model.NormToken{n.tok(token.SWITCH, "switch")}
		if s.Init != nil {
			tokens = append(tokens, n.stmtTokens(s.Init, t)...)
			tokens = append(tokens, n.tok(token.SEMICOLON, ";"))
		}
		if s.Tag != nil {
			tokens = append(tokens, n.exprTokens(s.Tag, t)...)
		}
		tokens = append(tokens, n.blockTokens(s.Body, t)...)
		return tokens

	case *ast.TypeSwitchStmt:
		tokens := []model.NormToken{n.tok(token.SWITCH, "switch")}
		if s.Init != nil {
			tokens = append(tokens, n.stmtTokens(s.Init, t)...)
			tokens = append(tokens, n.tok(token.SEMICOLON, ";"))
		}
		tokens = append(tokens, n.stmtTokens(s.Assign, t)...)
		tokens = append(tokens, n.blockTokens(s.Body, t)...)
		return tokens

	case *ast.CaseClause:
		tokens := []model.NormToken{n.tok(token.CASE, "case")}
		for i, expr := range s.List {
			if i > 0 {
				tokens = append(tokens, n.tok(token.COMMA, ","))
			}
			tokens = append(tokens, n.exprTokens(expr, t)...)
		}
		tokens = append(tokens, n.tok(token.COLON, ":"))
		for _, stmt := range s.Body {
			tokens = append(tokens, n.stmtTokens(stmt, t)...)
		}
		return tokens

	case *ast.DeferStmt:
		tokens := []model.NormToken{n.tok(token.DEFER, "defer")}
		tokens = append(tokens, n.exprTokens(s.Call, t)...)
		return tokens

	case *ast.GoStmt:
		tokens := []model.NormToken{n.tok(token.GO, "go")}
		tokens = append(tokens, n.exprTokens(s.Call, t)...)
		return tokens

	case *ast.SendStmt:
		tokens := n.exprTokens(s.Chan, t)
		tokens = append(tokens, n.tok(token.ARROW, "<-"))
		tokens = append(tokens, n.exprTokens(s.Value, t)...)
		return tokens

	case *ast.BranchStmt:
		return []model.NormToken{n.tok(s.Tok, s.Tok.String())}

	case *ast.LabeledStmt:
		tokens := []model.NormToken{n.identToken(s.Label.Name, identLabel, t)}
		tokens = append(tokens, n.tok(token.COLON, ":"))
		tokens = append(tokens, n.stmtTokens(s.Stmt, t)...)
		return tokens

	case *ast.SelectStmt:
		tokens := []model.NormToken{n.tok(token.SELECT, "select")}
		tokens = append(tokens, n.blockTokens(s.Body, t)...)
		return tokens

	case *ast.CommClause:
		tokens := []model.NormToken{n.tok(token.CASE, "case")}
		if s.Comm != nil {
			tokens = append(tokens, n.stmtTokens(s.Comm, t)...)
		}
		tokens = append(tokens, n.tok(token.COLON, ":"))
		for _, stmt := range s.Body {
			tokens = append(tokens, n.stmtTokens(stmt, t)...)
		}
		return tokens

	case *ast.EmptyStmt:
		return nil

	default:
		// Fallback: emit a placeholder for unhandled statement types.
		return []model.NormToken{n.rawToken(token.IDENT, fmt.Sprintf("$STMT_%T", stmt))}
	}
}

func (n *Normalizer) exprTokens(expr ast.Expr, t *identTracker) []model.NormToken {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.Ident:
		if e.Name == "_" {
			return []model.NormToken{n.rawToken(token.IDENT, "_")}
		}
		if e.Name == "nil" || e.Name == "true" || e.Name == "false" ||
			e.Name == "iota" || e.Name == "err" && n.level < model.NormStrong {
			return []model.NormToken{n.rawToken(token.IDENT, e.Name)}
		}
		return []model.NormToken{n.identToken(e.Name, identLocal, t)}

	case *ast.BasicLit:
		return []model.NormToken{n.literalToken(e)}

	case *ast.BinaryExpr:
		tokens := n.exprTokens(e.X, t)
		tokens = append(tokens, n.tok(e.Op, e.Op.String()))
		tokens = append(tokens, n.exprTokens(e.Y, t)...)
		return tokens

	case *ast.UnaryExpr:
		tokens := []model.NormToken{n.tok(e.Op, e.Op.String())}
		tokens = append(tokens, n.exprTokens(e.X, t)...)
		return tokens

	case *ast.CallExpr:
		tokens := n.exprTokens(e.Fun, t)
		tokens = append(tokens, n.tok(token.LPAREN, "("))
		for i, arg := range e.Args {
			if i > 0 {
				tokens = append(tokens, n.tok(token.COMMA, ","))
			}
			tokens = append(tokens, n.exprTokens(arg, t)...)
		}
		if e.Ellipsis.IsValid() {
			tokens = append(tokens, n.tok(token.ELLIPSIS, "..."))
		}
		tokens = append(tokens, n.tok(token.RPAREN, ")"))
		return tokens

	case *ast.SelectorExpr:
		tokens := n.exprTokens(e.X, t)
		tokens = append(tokens, n.tok(token.PERIOD, "."))
		// Selector names (method/field) are preserved at all levels.
		tokens = append(tokens, n.rawToken(token.IDENT, e.Sel.Name))
		return tokens

	case *ast.IndexExpr:
		tokens := n.exprTokens(e.X, t)
		tokens = append(tokens, n.tok(token.LBRACK, "["))
		tokens = append(tokens, n.exprTokens(e.Index, t)...)
		tokens = append(tokens, n.tok(token.RBRACK, "]"))
		return tokens

	case *ast.SliceExpr:
		tokens := n.exprTokens(e.X, t)
		tokens = append(tokens, n.tok(token.LBRACK, "["))
		if e.Low != nil {
			tokens = append(tokens, n.exprTokens(e.Low, t)...)
		}
		tokens = append(tokens, n.tok(token.COLON, ":"))
		if e.High != nil {
			tokens = append(tokens, n.exprTokens(e.High, t)...)
		}
		if e.Max != nil {
			tokens = append(tokens, n.tok(token.COLON, ":"))
			tokens = append(tokens, n.exprTokens(e.Max, t)...)
		}
		tokens = append(tokens, n.tok(token.RBRACK, "]"))
		return tokens

	case *ast.CompositeLit:
		var tokens []model.NormToken
		if e.Type != nil {
			tokens = append(tokens, n.typeTokens(e.Type, t)...)
		}
		tokens = append(tokens, n.tok(token.LBRACE, "{"))
		for i, elt := range e.Elts {
			if i > 0 {
				tokens = append(tokens, n.tok(token.COMMA, ","))
			}
			tokens = append(tokens, n.exprTokens(elt, t)...)
		}
		tokens = append(tokens, n.tok(token.RBRACE, "}"))
		return tokens

	case *ast.KeyValueExpr:
		tokens := n.exprTokens(e.Key, t)
		tokens = append(tokens, n.tok(token.COLON, ":"))
		tokens = append(tokens, n.exprTokens(e.Value, t)...)
		return tokens

	case *ast.ParenExpr:
		tokens := []model.NormToken{n.tok(token.LPAREN, "(")}
		tokens = append(tokens, n.exprTokens(e.X, t)...)
		tokens = append(tokens, n.tok(token.RPAREN, ")"))
		return tokens

	case *ast.StarExpr:
		tokens := []model.NormToken{n.tok(token.MUL, "*")}
		tokens = append(tokens, n.exprTokens(e.X, t)...)
		return tokens

	case *ast.TypeAssertExpr:
		tokens := n.exprTokens(e.X, t)
		tokens = append(tokens, n.tok(token.PERIOD, "."))
		tokens = append(tokens, n.tok(token.LPAREN, "("))
		if e.Type != nil {
			tokens = append(tokens, n.typeTokens(e.Type, t)...)
		} else {
			tokens = append(tokens, n.rawToken(token.IDENT, "type"))
		}
		tokens = append(tokens, n.tok(token.RPAREN, ")"))
		return tokens

	case *ast.FuncLit:
		tokens := []model.NormToken{n.tok(token.FUNC, "func")}
		if e.Type.Params != nil {
			tokens = append(tokens, n.fieldListTokens(e.Type.Params, identParam, t)...)
		}
		if e.Type.Results != nil {
			tokens = append(tokens, n.fieldListTokens(e.Type.Results, identResult, t)...)
		}
		if e.Body != nil {
			tokens = append(tokens, n.blockTokens(e.Body, t)...)
		}
		return tokens

	case *ast.Ellipsis:
		tokens := []model.NormToken{n.tok(token.ELLIPSIS, "...")}
		if e.Elt != nil {
			tokens = append(tokens, n.typeTokens(e.Elt, t)...)
		}
		return tokens

	case *ast.ArrayType:
		return n.typeTokens(e, t)

	case *ast.MapType:
		return n.typeTokens(e, t)

	case *ast.ChanType:
		return n.typeTokens(e, t)

	case *ast.InterfaceType:
		return n.typeTokens(e, t)

	case *ast.StructType:
		return n.typeTokens(e, t)

	default:
		return []model.NormToken{n.rawToken(token.IDENT, fmt.Sprintf("$EXPR_%T", expr))}
	}
}

func (n *Normalizer) typeTokens(expr ast.Expr, t *identTracker) []model.NormToken {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.Ident:
		// Type names are kept as-is at all levels (they are part of the signature).
		return []model.NormToken{n.rawToken(token.IDENT, e.Name)}

	case *ast.StarExpr:
		tokens := []model.NormToken{n.tok(token.MUL, "*")}
		tokens = append(tokens, n.typeTokens(e.X, t)...)
		return tokens

	case *ast.ArrayType:
		tokens := []model.NormToken{n.tok(token.LBRACK, "[")}
		if e.Len != nil {
			tokens = append(tokens, n.exprTokens(e.Len, t)...)
		}
		tokens = append(tokens, n.tok(token.RBRACK, "]"))
		tokens = append(tokens, n.typeTokens(e.Elt, t)...)
		return tokens

	case *ast.MapType:
		tokens := []model.NormToken{n.rawToken(token.IDENT, "map")}
		tokens = append(tokens, n.tok(token.LBRACK, "["))
		tokens = append(tokens, n.typeTokens(e.Key, t)...)
		tokens = append(tokens, n.tok(token.RBRACK, "]"))
		tokens = append(tokens, n.typeTokens(e.Value, t)...)
		return tokens

	case *ast.SelectorExpr:
		tokens := n.exprTokens(e.X, t)
		tokens = append(tokens, n.tok(token.PERIOD, "."))
		tokens = append(tokens, n.rawToken(token.IDENT, e.Sel.Name))
		return tokens

	case *ast.ChanType:
		tokens := []model.NormToken{n.rawToken(token.IDENT, "chan")}
		tokens = append(tokens, n.typeTokens(e.Value, t)...)
		return tokens

	case *ast.FuncType:
		tokens := []model.NormToken{n.tok(token.FUNC, "func")}
		if e.Params != nil {
			tokens = append(tokens, n.fieldListTokens(e.Params, identParam, t)...)
		}
		if e.Results != nil {
			tokens = append(tokens, n.fieldListTokens(e.Results, identResult, t)...)
		}
		return tokens

	case *ast.InterfaceType:
		return []model.NormToken{n.rawToken(token.IDENT, "interface{}")}

	case *ast.StructType:
		return []model.NormToken{n.rawToken(token.IDENT, "struct{}")}

	case *ast.Ellipsis:
		tokens := []model.NormToken{n.tok(token.ELLIPSIS, "...")}
		if e.Elt != nil {
			tokens = append(tokens, n.typeTokens(e.Elt, t)...)
		}
		return tokens

	default:
		return []model.NormToken{n.rawToken(token.IDENT, fmt.Sprintf("$TYPE_%T", expr))}
	}
}

func (n *Normalizer) fieldListTokens(fl *ast.FieldList, kind identKind, t *identTracker) []model.NormToken {
	tokens := []model.NormToken{n.tok(token.LPAREN, "(")}
	for i, field := range fl.List {
		if i > 0 {
			tokens = append(tokens, n.tok(token.COMMA, ","))
		}
		for j, name := range field.Names {
			if j > 0 {
				tokens = append(tokens, n.tok(token.COMMA, ","))
			}
			tokens = append(tokens, n.identToken(name.Name, kind, t))
		}
		tokens = append(tokens, n.typeTokens(field.Type, t)...)
	}
	tokens = append(tokens, n.tok(token.RPAREN, ")"))
	return tokens
}

func (n *Normalizer) declTokens(decl ast.Decl, t *identTracker) []model.NormToken {
	switch d := decl.(type) {
	case *ast.GenDecl:
		var tokens []model.NormToken
		tokens = append(tokens, n.tok(d.Tok, d.Tok.String()))
		for _, spec := range d.Specs {
			switch s := spec.(type) {
			case *ast.ValueSpec:
				for i, name := range s.Names {
					if i > 0 {
						tokens = append(tokens, n.tok(token.COMMA, ","))
					}
					tokens = append(tokens, n.identToken(name.Name, identLocal, t))
				}
				if s.Type != nil {
					tokens = append(tokens, n.typeTokens(s.Type, t)...)
				}
				for _, val := range s.Values {
					tokens = append(tokens, n.tok(token.ASSIGN, "="))
					tokens = append(tokens, n.exprTokens(val, t)...)
				}
			case *ast.TypeSpec:
				tokens = append(tokens, n.identToken(s.Name.Name, identLocal, t))
				tokens = append(tokens, n.typeTokens(s.Type, t)...)
			}
		}
		return tokens
	default:
		return nil
	}
}

// Token helpers

func (n *Normalizer) tok(kind token.Token, norm string) model.NormToken {
	return model.NormToken{Kind: kind, Norm: norm, OrigLit: norm}
}

func (n *Normalizer) rawToken(kind token.Token, lit string) model.NormToken {
	return model.NormToken{Kind: kind, Norm: lit, OrigLit: lit}
}

func (n *Normalizer) literalToken(lit *ast.BasicLit) model.NormToken {
	if n.level >= model.NormLight {
		switch lit.Kind {
		case token.INT:
			return model.NormToken{Kind: token.INT, Norm: "$INT", OrigLit: lit.Value}
		case token.FLOAT:
			return model.NormToken{Kind: token.FLOAT, Norm: "$FLOAT", OrigLit: lit.Value}
		case token.STRING:
			return model.NormToken{Kind: token.STRING, Norm: "$STR", OrigLit: lit.Value}
		case token.CHAR:
			return model.NormToken{Kind: token.CHAR, Norm: "$RUNE", OrigLit: lit.Value}
		}
	}
	return model.NormToken{Kind: lit.Kind, Norm: lit.Value, OrigLit: lit.Value}
}

func (n *Normalizer) identToken(name string, kind identKind, t *identTracker) model.NormToken {
	if n.level < model.NormStrong {
		return model.NormToken{Kind: token.IDENT, Norm: name, OrigLit: name}
	}
	norm := t.normalize(name, kind)
	return model.NormToken{Kind: token.IDENT, Norm: norm, OrigLit: name}
}

// identKind classifies an identifier for normalization purposes.
type identKind int

const (
	identLocal    identKind = iota // Local variable
	identParam                     // Function parameter
	identReceiver                  // Method receiver
	identResult                    // Named return value
	identLabel                     // Label
)

// identTracker assigns positional placeholder names to identifiers.
type identTracker struct {
	seen     map[string]string
	counters map[identKind]int
}

func newIdentTracker() *identTracker {
	return &identTracker{
		seen:     make(map[string]string),
		counters: make(map[identKind]int),
	}
}

func (t *identTracker) normalize(name string, kind identKind) string {
	if existing, ok := t.seen[name]; ok {
		return existing
	}

	var prefix string
	switch kind {
	case identLocal:
		prefix = "$V"
	case identParam:
		prefix = "$P"
	case identReceiver:
		prefix = "$R"
		t.seen[name] = "$R"
		return "$R"
	case identResult:
		prefix = "$RET"
	case identLabel:
		prefix = "$LABEL"
	}

	idx := t.counters[kind]
	t.counters[kind]++
	norm := prefix + strconv.Itoa(idx)
	t.seen[name] = norm
	return norm
}
