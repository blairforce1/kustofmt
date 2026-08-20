//go:build unix

package main

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

type owner struct{ uid, gid int }

func ownerOf(t *testing.T, path string) owner {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		t.Skip("this platform does not report a numeric owner")
	}
	return owner{int(st.Uid), int(st.Gid)}
}

// TestWritePreservesOwnership covers the ordinary case, where the person
// formatting a file already owns it. It cannot fail today for the wrong
// reasons -- the replacement would inherit the same owner anyway -- but it
// pins the property against a future change of write strategy.
func TestWritePreservesOwnership(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": messy})
	path := filepath.Join(dir, "a.yaml")
	before := ownerOf(t, path)

	if code, _, stderr := exercise(t, "", "-w", path); code != exitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if after := ownerOf(t, path); after != before {
		t.Errorf("owner = %+v, want %+v", after, before)
	}
}

// TestWritePreservesForeignOwnership is the test that actually bites: a file
// owned by somebody other than the user doing the formatting. That is the
// `sudo kustofmt -w .` case, where an atomic replacement would silently hand
// every manifest to root. Giving a file away needs privilege, so this runs
// only where the suite has it.
func TestWritePreservesForeignOwnership(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("needs root: only a privileged process can give a file away")
	}
	const nobody = 65534

	dir := writeTree(t, map[string]string{"a.yaml": messy})
	path := filepath.Join(dir, "a.yaml")
	if err := os.Chown(path, nobody, nobody); err != nil {
		t.Fatalf("chown: %v", err)
	}

	if code, _, stderr := exercise(t, "", "-w", path); code != exitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	if got := ownerOf(t, path); got.uid != nobody || got.gid != nobody {
		t.Errorf("owner = %+v, want uid and gid %d: the file was reassigned to the caller", got, nobody)
	}
	// The formatting still has to have happened.
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != tidy {
		t.Errorf("file =\n%s\nwant:\n%s", content, tidy)
	}
}
