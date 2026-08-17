// Package discovery walks repositories to find source files for analysis.
// It respects include/exclude patterns, vendor directories, generated file
// markers, symlink policies, and file size limits. Supports multiple languages.
package discovery

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/vinq1911/amimica/internal/config"
	"github.com/vinq1911/amimica/internal/lang"
	"github.com/vinq1911/amimica/internal/model"
)

// Walk discovers source files under the given roots, applying filtering
// rules from the config. Only files matching registered language extensions
// are included. Returns a slice of SourceFile descriptors.
func Walk(roots []string, cfg *config.Config, registry *lang.Registry, log *slog.Logger) ([]model.SourceFile, error) {
	var files []model.SourceFile

	for _, root := range roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("discovery: resolve root %q: %w", root, err)
		}

		err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				log.Warn("walk error", "path", path, "error", err)
				return nil
			}

			if d.IsDir() {
				name := d.Name()
				if name == "vendor" && !cfg.Paths.IncludeVendor {
					return filepath.SkipDir
				}
				if name == ".git" || name == ".amimica" || name == "node_modules" ||
					name == "__pycache__" || name == ".bundle" || name == ".next" ||
					name == "dist" || name == "build" ||
					name == "tmp" || name == "log" || name == "coverage" {
					return filepath.SkipDir
				}
				return nil
			}

			// Check if any registered language handles this file.
			language := registry.ForFile(path)
			if language == nil {
				return nil
			}

			if d.Type()&fs.ModeSymlink != 0 && !cfg.Paths.FollowSymlinks {
				return nil
			}

			relPath, err := filepath.Rel(absRoot, path)
			if err != nil {
				relPath = path
			}

			if matchesAny(relPath, cfg.Paths.Exclude) {
				return nil
			}

			info, err := d.Info()
			if err != nil {
				log.Warn("stat error", "path", path, "error", err)
				return nil
			}

			if info.Size() > cfg.Performance.MaxFileSize {
				log.Debug("skipping large file", "path", relPath, "size", info.Size())
				return nil
			}

			content, err := os.ReadFile(path)
			if err != nil {
				log.Warn("read error", "path", path, "error", err)
				return nil
			}

			isTest := language.IsTestFile(path)
			if isTest && !cfg.Paths.IncludeTests {
				return nil
			}

			isGenerated := language.IsGeneratedFile(content)
			if isGenerated && !cfg.Paths.IncludeGenerated {
				log.Debug("skipping generated", "path", relPath)
				return nil
			}

			// Skip minified files (avg line length > 200 chars).
			if isMinified(content) {
				log.Debug("skipping minified", "path", relPath)
				return nil
			}

			hash := sha256.Sum256(content)

			sf := model.SourceFile{
				Path:        path,
				RelPath:     relPath,
				Language:    language.Name(),
				ContentHash: hash,
				Size:        info.Size(),
				IsTest:      isTest,
				IsGenerated: isGenerated,
			}

			files = append(files, sf)
			return nil
		})

		if err != nil {
			return nil, fmt.Errorf("discovery: walk %q: %w", root, err)
		}
	}

	return files, nil
}

// isMinified detects minified JS/CSS files by checking average line length.
// Minified files have very long lines (often >1000 chars) because whitespace is stripped.
func isMinified(content []byte) bool {
	if len(content) < 500 {
		return false // too small to tell
	}
	lines := bytes.Count(content, []byte("\n"))
	if lines == 0 {
		lines = 1
	}
	avgLen := len(content) / lines
	return avgLen > 200
}

func matchesAny(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
		if matched, _ := filepath.Match(pattern, filepath.Base(relPath)); matched {
			return true
		}
		if strings.Contains(pattern, "**") {
			simplified := strings.ReplaceAll(pattern, "**/", "")
			if matched, _ := filepath.Match(simplified, filepath.Base(relPath)); matched {
				return true
			}
		}
	}
	return false
}
