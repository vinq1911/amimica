// Package lang defines the Language interface that abstracts language-specific
// parsing, normalization, and extraction. Each supported language implements
// this interface, allowing the engine to analyze multi-language codebases
// through a unified pipeline.
//
// The fingerprint, match, score, and report layers operate on []NormToken
// and are completely language-agnostic.
package lang

import (
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/user/amimica/internal/config"
	"github.com/user/amimica/internal/model"
)

// Language encapsulates all language-specific logic for clone detection.
type Language interface {
	// Name returns the language identifier (e.g., "go", "javascript", "ruby").
	Name() string

	// Extensions returns the file extensions this language handles.
	Extensions() []string

	// ParseAndExtract reads a source file and produces normalized units.
	// It combines parsing, normalization, and extraction into one step
	// because these are tightly coupled per language.
	ParseAndExtract(sf model.SourceFile, cfg *config.Config, level model.NormalizationLevel, log *slog.Logger) ([]model.NormalizedUnit, error)

	// IsTestFile returns true if the file is a test file for this language.
	IsTestFile(path string) bool

	// IsGeneratedFile checks the file content for generated-code markers.
	IsGeneratedFile(content []byte) bool
}

// Registry maps file extensions to Language implementations.
type Registry struct {
	byExt map[string]Language
	langs []Language
}

// NewRegistry creates an empty language registry.
func NewRegistry() *Registry {
	return &Registry{byExt: make(map[string]Language)}
}

// Register adds a language to the registry.
func (r *Registry) Register(l Language) {
	r.langs = append(r.langs, l)
	for _, ext := range l.Extensions() {
		r.byExt[ext] = l
	}
}

// ForFile returns the Language that handles the given file path, or nil.
func (r *Registry) ForFile(path string) Language {
	ext := strings.ToLower(filepath.Ext(path))
	return r.byExt[ext]
}

// SupportedExtensions returns all registered file extensions.
func (r *Registry) SupportedExtensions() []string {
	exts := make([]string, 0, len(r.byExt))
	for ext := range r.byExt {
		exts = append(exts, ext)
	}
	return exts
}

// Languages returns all registered languages.
func (r *Registry) Languages() []Language { return r.langs }

// IsSupportedFile returns true if the file extension is registered.
func (r *Registry) IsSupportedFile(path string) bool { return r.ForFile(path) != nil }
