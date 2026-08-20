//go:build unix

package main

import (
	"os"
	"path/filepath"
	"testing"
)

// Windows expresses permissions as an access-control list, not a mode, and
// os.Chmod there only toggles the read-only bit -- so these assertions are
// meaningful on Unix and meaningless everywhere else.

// TestWritePreservesFileMode: the atomic write creates a new file, so the mode
// has to be copied across deliberately. Without it, every formatted manifest
// would silently become 0644.
func TestWritePreservesFileMode(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": messy})
	path := filepath.Join(dir, "a.yaml")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := exercise(t, "", "-w", path); code != exitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640 preserved", info.Mode().Perm())
	}
}

// TestWriteLeavesModeAloneOnCleanFiles: an unchanged file is not rewritten at
// all, so its mode cannot drift either.
func TestWriteLeavesModeAloneOnCleanFiles(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": tidy})
	path := filepath.Join(dir, "a.yaml")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := exercise(t, "", "-w", path); code != exitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 preserved", info.Mode().Perm())
	}
}
