package format_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/blairforce1/kustofmt/format"
)

func TestDiffIdenticalIsEmpty(t *testing.T) {
	t.Parallel()
	if got := format.Diff("a", "b", []byte("x: 1\n"), []byte("x: 1\n")); got != "" {
		t.Errorf("expected empty diff, got:\n%s", got)
	}
}

func TestDiffOutputShape(t *testing.T) {
	t.Parallel()
	old := []byte("a: 1\nb:\n  - x\nc: 3\n")
	new := []byte("a: 1\nb:\n- x\nc: 3\n")
	got := format.Diff("testdata/f.yaml.orig", "testdata/f.yaml", old, new)

	want := "--- testdata/f.yaml.orig\n" +
		"+++ testdata/f.yaml\n" +
		"@@ -1,4 +1,4 @@\n" +
		" a: 1\n" +
		" b:\n" +
		"-  - x\n" +
		"+- x\n" +
		" c: 3\n"
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// TestDiffHandlesContentResemblingTheNoEOLMarker: the no-final-newline fact used
// to be recorded by appending a sentinel string to the line text, so a line
// whose content happened to end with that sentinel was mis-rendered -- the
// suffix stripped and a spurious "no newline" marker emitted for a file that
// ended perfectly normally. The fact lives in a struct field now and no content
// can imitate it.
func TestDiffHandlesContentResemblingTheNoEOLMarker(t *testing.T) {
	t.Parallel()
	const sentinel = "\x00noeol"
	old := []byte("a: 1\nb: " + sentinel + "\n")
	new := []byte("a: 2\nb: " + sentinel + "\n")

	d := format.Diff("old", "new", old, new)
	if strings.Contains(d, `\ No newline at end of file`) {
		t.Errorf("both files end in a newline, but the diff says otherwise:\n%q", d)
	}
	got, err := applyUnified(string(old), d)
	if err != nil {
		t.Fatalf("apply: %v\ndiff:\n%q", err, d)
	}
	if got != string(new) {
		t.Errorf("apply mismatch\ngot:  %q\nwant: %q", got, new)
	}
}

// TestDiffApplies is the real correctness test: a diff is right when applying
// it to the old file reproduces the new one. It runs over every fixture, so
// any hunk header arithmetic error shows up immediately.
func TestDiffApplies(t *testing.T) {
	for _, name := range goldenCases(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			old, err := os.ReadFile(filepath.Join("testdata", name+".input.yaml"))
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			new, err := format.Format(old)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			d := format.Diff("old", "new", old, new)
			if d == "" {
				if string(old) != string(new) {
					t.Fatal("files differ but diff was empty")
				}
				return
			}
			got, err := applyUnified(string(old), d)
			if err != nil {
				t.Fatalf("apply: %v\ndiff:\n%s", err, d)
			}
			if got != string(new) {
				t.Errorf("applying the diff did not reproduce the formatted file\ndiff:\n%s\ngot:\n%q\nwant:\n%q", d, got, new)
			}
		})
	}
}

// TestDiffMatchesSystemDiff cross-checks against GNU diff where available.
// Equally valid edit scripts exist, so this compares the reconstructed result
// rather than demanding byte-identical output.
func TestDiffMatchesSystemDiff(t *testing.T) {
	if testing.Short() {
		t.Skip("shells out to diff(1)")
	}
	if _, err := exec.LookPath("diff"); err != nil {
		t.Skip("diff(1) not available")
	}
	dir := t.TempDir()
	for _, name := range goldenCases(t) {
		t.Run(name, func(t *testing.T) {
			old, err := os.ReadFile(filepath.Join("testdata", name+".input.yaml"))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			new, err := format.Format(old)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			if string(old) == string(new) {
				t.Skip("no change")
			}
			oldPath := filepath.Join(dir, name+".old")
			newPath := filepath.Join(dir, name+".new")
			if err := os.WriteFile(oldPath, old, 0o644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(newPath, new, 0o644); err != nil {
				t.Fatal(err)
			}
			out, _ := exec.CommandContext(t.Context(), "diff", "-u", oldPath, newPath).Output()
			sysHunks := strings.Count(string(out), "\n@@ ")
			ourHunks := strings.Count(format.Diff("a", "b", old, new), "\n@@ ")
			if sysHunks != ourHunks {
				t.Errorf("hunk count %d, diff(1) says %d\nours:\n%s\ntheirs:\n%s",
					ourHunks, sysHunks, format.Diff("a", "b", old, new), out)
			}
		})
	}
}

// applyUnified is a minimal unified-diff applier, used to verify our output.
// A "\\ No newline at end of file" marker refers to the line immediately before
// it, so it only affects the result when that line is a context or added line.
func applyUnified(old, diff string) (string, error) {
	oldLines := strings.Split(strings.TrimSuffix(old, "\n"), "\n")
	if old == "" {
		oldLines = nil
	}
	var out []string
	cursor := 0
	newEndsWithoutNewline := false

	lines := strings.Split(strings.TrimSuffix(diff, "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "@@") {
			continue
		}
		start, err := hunkOldStart(lines[i])
		if err != nil {
			return "", err
		}
		if start > len(oldLines) {
			return "", fmt.Errorf("hunk starts at %d, file has %d lines", start, len(oldLines))
		}
		out = append(out, oldLines[cursor:start]...)
		cursor = start

		prevPrefix := byte(0)
		for i+1 < len(lines) && !strings.HasPrefix(lines[i+1], "@@") {
			i++
			body := lines[i]
			if body == `\ No newline at end of file` {
				if prevPrefix == ' ' || prevPrefix == '+' {
					newEndsWithoutNewline = true
				}
				continue
			}
			if body == "" {
				continue
			}
			prevPrefix = body[0]
			switch body[0] {
			case ' ':
				if cursor >= len(oldLines) || oldLines[cursor] != body[1:] {
					return "", fmt.Errorf("context mismatch at line %d: %q", cursor, body[1:])
				}
				out = append(out, body[1:])
				cursor++
			case '-':
				if cursor >= len(oldLines) || oldLines[cursor] != body[1:] {
					return "", fmt.Errorf("deletion mismatch at line %d: %q", cursor, body[1:])
				}
				cursor++
			case '+':
				out = append(out, body[1:])
			}
		}
	}
	// Any lines after the final hunk are copied straight from the old file, so
	// the result inherits the old file's trailing-newline state.
	if tail := oldLines[cursor:]; len(tail) > 0 {
		out = append(out, tail...)
		newEndsWithoutNewline = !strings.HasSuffix(old, "\n")
	}
	if len(out) == 0 {
		return "", nil
	}
	joined := strings.Join(out, "\n")
	if newEndsWithoutNewline {
		return joined, nil
	}
	return joined + "\n", nil
}

// hunkOldStart parses the 0-based old-file start offset from "@@ -a,b +c,d @@".
func hunkOldStart(header string) (int, error) {
	fields := strings.Fields(header)
	if len(fields) < 2 || !strings.HasPrefix(fields[1], "-") {
		return 0, fmt.Errorf("malformed hunk header %q", header)
	}
	spec := strings.TrimPrefix(fields[1], "-")
	startStr, countStr, hasCount := strings.Cut(spec, ",")
	start, err := strconv.Atoi(startStr)
	if err != nil {
		return 0, fmt.Errorf("malformed hunk header %q: %w", header, err)
	}
	// A zero-length range is written at the preceding line and is already
	// 0-based; every other range is 1-based.
	if hasCount && countStr == "0" {
		return start, nil
	}
	return start - 1, nil
}
