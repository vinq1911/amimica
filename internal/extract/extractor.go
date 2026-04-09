// Package extract segments parsed and normalized Go ASTs into analysis units:
// whole functions, statement windows, and inner blocks. Each unit is a
// NormalizedUnit suitable for fingerprinting and comparison.
package extract

import (
	"crypto/sha256"
	"go/ast"
	"go/token"
	"strings"

	"github.com/user/amimica/internal/config"
	"github.com/user/amimica/internal/lang"
	"github.com/user/amimica/internal/model"
	"github.com/user/amimica/internal/normalize"
	"github.com/user/amimica/internal/parser"
)

// Extract produces NormalizedUnits from a parsed file at the given normalization level.
// It extracts function-level units and optionally statement windows.
// Functions annotated with "amimica-ignore" in their doc comment are skipped.
func Extract(pf *parser.ParsedFile, cfg *config.Config, level model.NormalizationLevel) []model.NormalizedUnit {
	norm := normalize.New(level, pf.Fset)
	var units []model.NormalizedUnit

	for _, decl := range pf.File.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}

		// Check for amimica-ignore directive in doc comment or line comment.
		if hasIgnoreDirective(fn) {
			continue
		}

		stmtCount := len(fn.Body.List)
		if stmtCount < cfg.Analysis.MinStatements {
			continue
		}

		// Extract function-level unit.
		tokens := norm.NormalizeFunc(fn)
		if len(tokens) == 0 {
			continue
		}

		region := funcRegion(pf, fn)
		lines := region.EndLine - region.StartLine + 1
		if lines < cfg.Analysis.MinLines {
			continue
		}

		unit := makeUnit(tokens, region, model.UnitFunction, level, stmtCount, countNodes(fn))
		units = append(units, unit)

		// Extract statement windows if function is large enough.
		if stmtCount >= cfg.Analysis.WindowMinFunctionSize {
			winSize := cfg.Analysis.WindowSize
			for i := 0; i <= stmtCount-winSize; i++ {
				window := fn.Body.List[i : i+winSize]
				winTokens := norm.NormalizeStmts(window)
				if len(winTokens) == 0 {
					continue
				}
				winRegion := stmtRegion(pf, window, fn)
				winUnit := makeUnit(winTokens, winRegion, model.UnitWindow, level, winSize, countStmtNodes(window))
				units = append(units, winUnit)
			}
		}
	}

	return units
}

// hasIgnoreDirective checks whether a function declaration has an "amimica-ignore"
// directive in its doc comment or associated comment group.
func hasIgnoreDirective(fn *ast.FuncDecl) bool {
	if fn.Doc != nil {
		for _, c := range fn.Doc.List {
			if strings.Contains(c.Text, "amimica-ignore") {
				return true
			}
		}
	}
	return false
}

// amimica-ignore: differs from lang.MakeUnit by accepting nodeCount from Go AST (not just len(tokens))
func makeUnit(tokens []model.NormToken, region model.SourceRegion, kind model.UnitKind, level model.NormalizationLevel, stmtCount, nodeCount int) model.NormalizedUnit {
	tokenHash := lang.HashTokens(tokens)
	id := sha256.Sum256(append(tokenHash[:], []byte(region.File)...))

	return model.NormalizedUnit{
		ID:         id,
		Source:     region,
		Kind:       kind,
		NormTokens: tokens,
		NormLevel:  level,
		TokenHash:  tokenHash,
		ASTHash:    tokenHash,
		StmtCount:  stmtCount,
		NodeCount:  nodeCount,
	}
}

func funcRegion(pf *parser.ParsedFile, fn *ast.FuncDecl) model.SourceRegion {
	start := pf.Fset.Position(fn.Pos())
	end := pf.Fset.Position(fn.End())

	region := model.SourceRegion{
		File:      pf.Source.RelPath,
		StartLine: start.Line,
		EndLine:   end.Line,
		StartCol:  start.Column,
		EndCol:    end.Column,
		FuncName:  fn.Name.Name,
	}

	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		region.Receiver = typeString(fn.Recv.List[0].Type)
	}

	return region
}

func stmtRegion(pf *parser.ParsedFile, stmts []ast.Stmt, fn *ast.FuncDecl) model.SourceRegion {
	if len(stmts) == 0 {
		return model.SourceRegion{File: pf.Source.RelPath}
	}
	start := pf.Fset.Position(stmts[0].Pos())
	end := pf.Fset.Position(stmts[len(stmts)-1].End())

	return model.SourceRegion{
		File:      pf.Source.RelPath,
		StartLine: start.Line,
		EndLine:   end.Line,
		StartCol:  start.Column,
		EndCol:    end.Column,
		FuncName:  fn.Name.Name,
	}
}

func typeString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	default:
		return ""
	}
}

func countNodes(node ast.Node) int {
	count := 0
	ast.Inspect(node, func(n ast.Node) bool {
		if n != nil {
			count++
		}
		return true
	})
	return count
}

func countStmtNodes(stmts []ast.Stmt) int {
	count := 0
	for _, s := range stmts {
		ast.Inspect(s, func(n ast.Node) bool {
			if n != nil {
				count++
			}
			return true
		})
	}
	return count
}

// TokensToString renders a normalized token sequence as human-readable text.
func TokensToString(tokens []model.NormToken) string {
	var buf []byte
	prevKind := token.ILLEGAL
	for _, t := range tokens {
		// Add space between tokens (but not after { or before }).
		if len(buf) > 0 && prevKind != token.LBRACE && t.Kind != token.RBRACE &&
			prevKind != token.LPAREN && t.Kind != token.RPAREN &&
			t.Kind != token.COMMA && t.Kind != token.SEMICOLON &&
			t.Kind != token.PERIOD && prevKind != token.PERIOD &&
			t.Kind != token.COLON && prevKind != token.COLON {
			buf = append(buf, ' ')
		}
		buf = append(buf, t.Norm...)
		prevKind = t.Kind
	}
	return string(buf)
}
