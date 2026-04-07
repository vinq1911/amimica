// Package fsguard implements path sandboxing and security validation for file
// system access. It ensures that the analysis engine can only access files within
// explicitly permitted root directories, preventing path traversal attacks and
// accidental access to files outside the intended scan scope.
//
// The Guard type is the primary entry point. Create one with New(), then call
// ValidatePath() before reading any file, ValidateSymlink() to enforce symlink
// policy, and ValidateFileSize() to enforce size limits.
//
// Usage:
//
//	g, err := fsguard.New([]string{"/path/to/repo"}, 1<<20, false)
//	if err != nil { ... }
//
//	if err := g.ValidatePath(filePath); err != nil {
//	    // path is outside allowed roots
//	}
package fsguard

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Guard enforces filesystem access policies: root containment, symlink handling,
// and file size limits. All fields are immutable after construction.
type Guard struct {
	// roots contains the absolute, cleaned paths of all allowed root directories.
	roots []string

	// maxSize is the maximum permitted file size in bytes.
	maxSize int64

	// followSymlinks controls whether symbolic links are followed.
	// When false, encountering a symlink returns an error from ValidateSymlink.
	// When true, symlinks are resolved and the target is checked against roots.
	followSymlinks bool
}

// New creates a new Guard that restricts access to the given root directories.
//
// Each root path is resolved to an absolute path and verified to exist. If any
// root does not exist or cannot be resolved, New returns an error.
//
// Parameters:
//   - roots: one or more directory paths that define the allowed access zone.
//   - maxSize: maximum file size in bytes (files larger than this fail ValidateFileSize).
//   - followSymlinks: if false, any symlink causes ValidateSymlink to return an error.
func New(roots []string, maxSize int64, followSymlinks bool) (*Guard, error) {
	if len(roots) == 0 {
		return nil, fmt.Errorf("fsguard: at least one root directory must be provided")
	}

	resolved := make([]string, 0, len(roots))
	for _, root := range roots {
		abs, err := filepath.Abs(root)
		if err != nil {
			return nil, fmt.Errorf("fsguard: resolve root %q: %w", root, err)
		}
		abs = filepath.Clean(abs)

		// Verify the root exists.
		if _, err := os.Stat(abs); err != nil {
			return nil, fmt.Errorf("fsguard: root %q: %w", root, err)
		}

		resolved = append(resolved, abs)
	}

	return &Guard{
		roots:          resolved,
		maxSize:        maxSize,
		followSymlinks: followSymlinks,
	}, nil
}

// ValidatePath checks that path (after resolving to an absolute path) is contained
// within one of the Guard's allowed roots.
//
// It returns nil if the path is inside any root, or a descriptive error if the
// path is outside all roots. This protects against path traversal attacks because
// filepath.Abs + filepath.Clean collapse any ".." components.
func (g *Guard) ValidatePath(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("fsguard: resolve path %q: %w", path, err)
	}
	abs = filepath.Clean(abs)

	for _, root := range g.roots {
		// Accept paths that are exactly the root or descend from it.
		if abs == root || strings.HasPrefix(abs, root+string(filepath.Separator)) {
			return nil
		}
	}

	return fmt.Errorf("fsguard: path %q is outside all allowed roots", path)
}

// ValidateSymlink checks the symlink policy for the given path.
//
// If followSymlinks is false:
//   - If path is a symlink, returns an error (symlinks are not permitted).
//   - If path is not a symlink, returns nil.
//
// If followSymlinks is true:
//   - Resolves the symlink target with filepath.EvalSymlinks.
//   - Checks the resolved target against allowed roots via ValidatePath.
//   - Returns an error if the target is outside all roots.
func (g *Guard) ValidateSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("fsguard: lstat %q: %w", path, err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		// Not a symlink — no policy concern.
		return nil
	}

	// path is a symlink.
	if !g.followSymlinks {
		return fmt.Errorf("fsguard: symlink not followed (followSymlinks=false): %q", path)
	}

	// followSymlinks=true: resolve and validate target.
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("fsguard: resolve symlink %q: %w", path, err)
	}
	return g.ValidatePath(resolved)
}

// ValidateFileSize checks that size does not exceed the Guard's maxSize limit.
// Returns nil if size <= maxSize, or an error describing the violation.
func (g *Guard) ValidateFileSize(size int64) error {
	if size > g.maxSize {
		return fmt.Errorf("fsguard: file size %d bytes exceeds limit of %d bytes", size, g.maxSize)
	}
	return nil
}
