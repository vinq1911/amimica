// Package discovery walks repositories to find Go source files for analysis.
// It respects include/exclude patterns, vendor directories, generated file
// markers, symlink policies, and file size limits.
package discovery

import (
	"bufio"
	"crypto/sha256"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/user/amimica/internal/config"
	"github.com/user/amimica/internal/model"
)

var generatedMarker = regexp.MustCompile(`^// Code generated .* DO NOT EDIT\.$`)

// Walk discovers Go source files under the given roots, applying filtering
// rules from the config. It returns a slice of SourceFile descriptors.
// Files that fail to read or exceed size limits are logged and skipped.
func Walk(roots []string, cfg *config.Config, log *slog.Logger) ([]model.SourceFile, error) {
	var files []model.SourceFile

	for _, root := range roots {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("discovery: resolve root %q: %w", root, err)
		}

		err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				log.Warn("walk error", "path", path, "error", err)
				return nil // skip but continue
			}

			// Skip directories that should be excluded.
			if d.IsDir() {
				name := d.Name()
				if name == "vendor" && !cfg.Paths.IncludeVendor {
					return filepath.SkipDir
				}
				if name == ".git" || name == ".amimica" || name == "node_modules" {
					return filepath.SkipDir
				}
				if name == "testdata" {
					// walk into testdata but files within won't match *.go patterns typically
					return nil
				}
				return nil
			}

			// Only consider .go files.
			if !strings.HasSuffix(path, ".go") {
				return nil
			}

			// Skip symlinks unless configured to follow.
			if d.Type()&fs.ModeSymlink != 0 && !cfg.Paths.FollowSymlinks {
				log.Debug("skipping symlink", "path", path)
				return nil
			}

			relPath, err := filepath.Rel(absRoot, path)
			if err != nil {
				relPath = path
			}

			// Check exclude patterns.
			if matchesAny(relPath, cfg.Paths.Exclude) {
				log.Debug("excluded", "path", relPath)
				return nil
			}

			// Get file info for size check.
			info, err := d.Info()
			if err != nil {
				log.Warn("stat error", "path", path, "error", err)
				return nil
			}

			if info.Size() > cfg.Performance.MaxFileSize {
				log.Debug("skipping large file", "path", relPath, "size", info.Size())
				return nil
			}

			// Read file content for hashing and generated detection.
			content, err := os.ReadFile(path)
			if err != nil {
				log.Warn("read error", "path", path, "error", err)
				return nil
			}

			isTest := strings.HasSuffix(filepath.Base(path), "_test.go")
			if isTest && !cfg.Paths.IncludeTests {
				return nil
			}

			isGenerated := detectGenerated(content)
			if isGenerated && !cfg.Paths.IncludeGenerated {
				log.Debug("skipping generated", "path", relPath)
				return nil
			}

			hash := sha256.Sum256(content)

			sf := model.SourceFile{
				Path:        path,
				RelPath:     relPath,
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

// detectGenerated checks the first few lines of content for the canonical
// Go generated code marker: "// Code generated ... DO NOT EDIT."
func detectGenerated(content []byte) bool {
	scanner := bufio.NewScanner(strings.NewReader(string(content)))
	for i := 0; i < 10 && scanner.Scan(); i++ {
		if generatedMarker.MatchString(scanner.Text()) {
			return true
		}
	}
	return false
}

// matchesAny checks if relPath matches any of the glob patterns.
func matchesAny(relPath string, patterns []string) bool {
	for _, pattern := range patterns {
		// Try matching the full relative path.
		if matched, _ := filepath.Match(pattern, relPath); matched {
			return true
		}
		// Try matching just the filename against patterns like "*.pb.go".
		if matched, _ := filepath.Match(pattern, filepath.Base(relPath)); matched {
			return true
		}
		// Handle ** patterns by checking if any path segment matches.
		if strings.Contains(pattern, "**") {
			simplified := strings.ReplaceAll(pattern, "**/", "")
			if matched, _ := filepath.Match(simplified, filepath.Base(relPath)); matched {
				return true
			}
		}
	}
	return false
}
