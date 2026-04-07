package fsguard_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/user/amimica/internal/fsguard"
)

// setupTempRoot creates a temporary directory structure for testing.
// Returns the root dir and a cleanup function.
func setupTempRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// Create a subdirectory
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	// Create a file inside
	f := filepath.Join(sub, "file.go")
	if err := os.WriteFile(f, []byte("package main\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return root
}

func TestNewGuardRejectsNonexistentRoot(t *testing.T) {
	_, err := fsguard.New([]string{"/nonexistent/path/xyz"}, 1<<20, false)
	if err == nil {
		t.Error("New should reject non-existent root directory")
	}
}

func TestValidatePathAcceptsPathInRoot(t *testing.T) {
	root := setupTempRoot(t)
	g, err := fsguard.New([]string{root}, 1<<20, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	innerPath := filepath.Join(root, "sub", "file.go")
	if err := g.ValidatePath(innerPath); err != nil {
		t.Errorf("ValidatePath should accept path inside root: %v", err)
	}
}

func TestValidatePathRejectsPathOutsideRoot(t *testing.T) {
	root := setupTempRoot(t)
	g, err := fsguard.New([]string{root}, 1<<20, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.ValidatePath("/etc/passwd"); err == nil {
		t.Error("ValidatePath should reject path outside root")
	}
}

func TestValidatePathRejectsTraversal(t *testing.T) {
	root := setupTempRoot(t)
	g, err := fsguard.New([]string{root}, 1<<20, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Attempt path traversal — the absolute resolved path will be outside root
	traversal := filepath.Join(root, "sub", "..", "..", "etc", "passwd")
	if err := g.ValidatePath(traversal); err == nil {
		t.Error("ValidatePath should reject path traversal that resolves outside root")
	}
}

func TestValidateSymlinkRejectsSymlinksWhenNotFollowing(t *testing.T) {
	root := setupTempRoot(t)
	// Create a symlink inside root pointing elsewhere
	linkPath := filepath.Join(root, "link")
	target := filepath.Join(root, "sub", "file.go")
	if err := os.Symlink(target, linkPath); err != nil {
		t.Fatalf("Symlink: %v", err)
	}

	g, err := fsguard.New([]string{root}, 1<<20, false) // followSymlinks=false
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.ValidateSymlink(linkPath); err == nil {
		t.Error("ValidateSymlink should reject symlinks when followSymlinks=false")
	}
}

func TestValidateSymlinkAcceptsNonSymlink(t *testing.T) {
	root := setupTempRoot(t)
	g, err := fsguard.New([]string{root}, 1<<20, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	regularFile := filepath.Join(root, "sub", "file.go")
	if err := g.ValidateSymlink(regularFile); err != nil {
		t.Errorf("ValidateSymlink should accept regular file: %v", err)
	}
}

func TestNewGuardMultipleRootsAllowsAllPaths(t *testing.T) {
	root1 := t.TempDir()
	root2 := t.TempDir()

	file1 := filepath.Join(root1, "a.go")
	file2 := filepath.Join(root2, "b.go")
	if err := os.WriteFile(file1, []byte("package a\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if err := os.WriteFile(file2, []byte("package b\n"), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	g, err := fsguard.New([]string{root1, root2}, 1<<20, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.ValidatePath(file1); err != nil {
		t.Errorf("ValidatePath root1 file: %v", err)
	}
	if err := g.ValidatePath(file2); err != nil {
		t.Errorf("ValidatePath root2 file: %v", err)
	}
}

func TestValidateFileSizeRejectsOversizedFile(t *testing.T) {
	root := setupTempRoot(t)
	g, err := fsguard.New([]string{root}, 100, false) // maxSize=100 bytes
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.ValidateFileSize(200); err == nil {
		t.Error("ValidateFileSize should reject size > maxSize")
	}
}

func TestValidateFileSizeAcceptsSmallFile(t *testing.T) {
	root := setupTempRoot(t)
	g, err := fsguard.New([]string{root}, 1<<20, false)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := g.ValidateFileSize(100); err != nil {
		t.Errorf("ValidateFileSize should accept small file: %v", err)
	}
}
