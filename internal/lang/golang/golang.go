// Package golang implements the Language interface for Go source files.
// It wraps the existing parser, normalizer, and extractor packages.
package golang

import (
	"bufio"
	"log/slog"
	"regexp"
	"strings"

	"github.com/user/amimica/internal/config"
	"github.com/user/amimica/internal/extract"
	"github.com/user/amimica/internal/model"
	"github.com/user/amimica/internal/parser"
)

var generatedMarker = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// Lang implements lang.Language for Go.
type Lang struct{}

func New() *Lang { return &Lang{} }

func (l *Lang) Name() string         { return "go" }
func (l *Lang) Extensions() []string { return []string{".go"} }

func (l *Lang) IsTestFile(path string) bool {
	return strings.HasSuffix(path, "_test.go")
}

func (l *Lang) IsGeneratedFile(content []byte) bool {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for i := 0; i < 10 && scanner.Scan(); i++ {
		if generatedMarker.MatchString(scanner.Text()) {
			return true
		}
	}
	return false
}

func (l *Lang) ParseAndExtract(sf model.SourceFile, cfg *config.Config, level model.NormalizationLevel, log *slog.Logger) ([]model.NormalizedUnit, error) {
	pf, err := parser.ParseFile(sf, log)
	if err != nil {
		return nil, err
	}
	return extract.Extract(pf, cfg, level), nil
}
