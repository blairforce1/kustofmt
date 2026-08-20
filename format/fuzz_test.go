package format_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/blairforce1/kustofmt/format"
)

// seedFromTestdata adds every fixture as a fuzz seed, so the fuzzer starts from
// real YAML rather than having to discover the grammar from scratch.
func seedFromTestdata(f *testing.F) {
	f.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "*.yaml"))
	if err != nil {
		f.Fatalf("glob: %v", err)
	}
	for _, m := range matches {
		b, err := os.ReadFile(m)
		if err != nil {
			f.Fatalf("read %s: %v", m, err)
		}
		f.Add(b)
	}
	// A few shapes worth pinning even if no fixture happens to cover them.
	for _, s := range []string{
		"", "---\n", "a: 1\n", "{}\n", "[]\n", "# only a comment\n",
		"a: {b: [c, {d: e}]}\n", "a: &x 1\nb: *x\n", "? complex\n: key\n",
	} {
		f.Add([]byte(s))
	}
}

// FuzzIdempotent: fmt(fmt(x)) == fmt(x), byte for byte. A formatter that fails
// this makes -l flap and sends a pre-commit hook into a loop.
func FuzzIdempotent(f *testing.F) {
	seedFromTestdata(f)
	f.Fuzz(func(t *testing.T, in []byte) {
		once, err := format.Format(in)
		if err != nil {
			t.Skip() // invalid YAML is rejected, which is correct behaviour
		}
		twice, err := format.Format(once)
		if err != nil {
			t.Fatalf("formatted output no longer formats: %v\ninput: %q\nfirst: %q", err, in, once)
		}
		if !bytes.Equal(once, twice) {
			t.Fatalf("not idempotent\ninput:  %q\nfirst:  %q\nsecond: %q", in, once, twice)
		}
	})
}

// FuzzSemanticsPreserved: the bytes may change, the meaning may not.
func FuzzSemanticsPreserved(f *testing.F) {
	seedFromTestdata(f)
	f.Fuzz(func(t *testing.T, in []byte) {
		out, err := format.Format(in)
		if err != nil {
			t.Skip()
		}
		requireSameSemantics(t, in, out)
	})
}

// FuzzIsSOPSNeverPanics: detection runs on every file the tool touches,
// including malformed ones, and must always return an answer.
func FuzzIsSOPSNeverPanics(f *testing.F) {
	seedFromTestdata(f)
	f.Fuzz(func(t *testing.T, in []byte) {
		_ = format.IsSOPS(in)
	})
}

// FuzzDiffApplies: whatever the inputs, a generated diff must reconstruct the
// second file from the first.
func FuzzDiffApplies(f *testing.F) {
	f.Add([]byte("a: 1\n"), []byte("a: 2\n"))
	f.Add([]byte(""), []byte("a: 1\n"))
	f.Add([]byte("a: 1\nb: 2\nc: 3\n"), []byte("a: 1\nc: 3\n"))
	f.Fuzz(func(t *testing.T, a, b []byte) {
		d := format.Diff("old", "new", a, b)
		if d == "" {
			if !bytes.Equal(a, b) {
				t.Fatalf("inputs differ but diff was empty\na=%q\nb=%q", a, b)
			}
			return
		}
		got, err := applyUnified(string(a), d)
		if err != nil {
			t.Fatalf("apply failed: %v\na=%q\nb=%q\ndiff:\n%s", err, a, b, d)
		}
		want := string(b)
		if got != want {
			t.Fatalf("apply mismatch\na=%q\nb=%q\ngot=%q\ndiff:\n%s", a, b, got, d)
		}
	})
}
