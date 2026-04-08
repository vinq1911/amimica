// Package parser wraps Go's go/parser to produce ASTs from source files with
// error tolerance. Partial ASTs are returned even when files contain syntax errors.
package parser

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/scanner"
	"go/token"
	"log/slog"
	"os"

	"github.com/user/amimica/internal/model"
)

// ParsedFile holds the AST and metadata for a single parsed Go source file.
type ParsedFile struct {
	File    *ast.File
	Fset    *token.FileSet
	Source  model.SourceFile
	Content []byte
}

// ParseFile parses a single Go source file. It tolerates syntax errors and
// returns partial ASTs when possible. If the file fails to parse entirely,
// it returns nil with an error.
func ParseFile(sf model.SourceFile, log *slog.Logger) (*ParsedFile, error) {
	content, err := os.ReadFile(sf.Path)
	if err != nil {
		return nil, fmt.Errorf("parser: read %q: %w", sf.Path, err)
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, sf.Path, content, parser.ParseComments|parser.AllErrors)

	if err != nil {
		// Collect parse errors but continue with partial AST if available.
		if errList, ok := err.(scanner.ErrorList); ok {
			for _, e := range errList {
				sf.ParseErrors = append(sf.ParseErrors, model.ParseError{
					Line:   e.Pos.Line,
					Column: e.Pos.Column,
					Msg:    e.Msg,
				})
			}
			log.Debug("parse errors (partial AST available)", "path", sf.RelPath, "errors", len(errList))
		}
	}

	if f == nil {
		return nil, fmt.Errorf("parser: failed to parse %q: %w", sf.RelPath, err)
	}

	// Extract package name from the parsed file.
	sf.Package = f.Name.Name

	return &ParsedFile{
		File:    f,
		Fset:    fset,
		Source:  sf,
		Content: content,
	}, nil
}

// ParseFiles parses all source files and returns successfully parsed files.
// Files that fail to parse are logged and skipped.
func ParseFiles(files []model.SourceFile, log *slog.Logger) []*ParsedFile {
	var parsed []*ParsedFile
	for _, sf := range files {
		pf, err := ParseFile(sf, log)
		if err != nil {
			log.Warn("skipping file", "path", sf.RelPath, "error", err)
			continue
		}
		parsed = append(parsed, pf)
	}
	return parsed
}
