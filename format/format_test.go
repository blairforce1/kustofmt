package format_test

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairforce1/kustofmt/format"
)

var update = flag.Bool("update", false, "regenerate .golden.yaml files from .input.yaml")

// goldenCases returns the base name of every <case>.input.yaml in testdata.
func goldenCases(t *testing.T) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join("testdata", "*.input.yaml"))
	if err != nil {
		t.Fatalf("glob testdata: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("no test fixtures found in testdata")
	}
	names := make([]string, 0, len(matches))
	for _, m := range matches {
		names = append(names, strings.TrimSuffix(filepath.Base(m), ".input.yaml"))
	}
	return names
}

// TestGolden pins the style contract. Every fixture is a documented decision:
// if one of these files changes, the output style changed, and per semver that
// is a breaking change.
func TestGolden(t *testing.T) {
	for _, name := range goldenCases(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			inPath := filepath.Join("testdata", name+".input.yaml")
			goldenPath := filepath.Join("testdata", name+".golden.yaml")

			in, err := os.ReadFile(inPath)
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			got, err := format.Format(in)
			if err != nil {
				t.Fatalf("Format(%s): %v", name, err)
			}
			if *update {
				if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
					t.Fatalf("write golden: %v", err)
				}
				return
			}
			want, err := os.ReadFile(goldenPath)
			if err != nil {
				t.Fatalf("read golden (run `go test ./... -update` to create): %v", err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("output differs from golden\n--- want ---\n%s\n--- got ---\n%s", want, got)
			}
		})
	}
}

// TestIdempotent is the formatter's core guarantee: fmt(fmt(x)) == fmt(x),
// byte for byte. Without it, -l flaps and a pre-commit hook loops forever.
func TestIdempotent(t *testing.T) {
	for _, name := range goldenCases(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in, err := os.ReadFile(filepath.Join("testdata", name+".input.yaml"))
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			once, err := format.Format(in)
			if err != nil {
				t.Fatalf("first pass: %v", err)
			}
			twice, err := format.Format(once)
			if err != nil {
				t.Fatalf("second pass: %v", err)
			}
			if !bytes.Equal(once, twice) {
				t.Errorf("not idempotent\n--- first ---\n%s\n--- second ---\n%s", once, twice)
			}
		})
	}
}

// TestZeroDiff is the compatibility guarantee: YAML freshly emitted by the
// kustomize ecosystem is already in house style and must come back untouched.
// If this fails, kustofmt disagrees with the tool it claims to imitate.
func TestZeroDiff(t *testing.T) {
	for _, name := range []string{"kustomize-output", "flux-kustomization"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in, err := os.ReadFile(filepath.Join("testdata", name+".input.yaml"))
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			got, err := format.Format(in)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			if !bytes.Equal(in, got) {
				t.Errorf("ecosystem output was modified\n--- in ---\n%s\n--- out ---\n%s", in, got)
			}
		})
	}
}

// TestPassthrough covers inputs that parse to zero documents. The YAML object
// model cannot hold a document with no content, so re-emitting these files
// would silently empty them. Returning the input verbatim is the only safe
// answer.
func TestPassthrough(t *testing.T) {
	for _, name := range []string{"comments-only", "empty-file", "whitespace-only"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in, err := os.ReadFile(filepath.Join("testdata", name+".input.yaml"))
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			got, err := format.Format(in)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			if !bytes.Equal(in, got) {
				t.Errorf("input with no documents was altered\nin:  %q\ngot: %q", in, got)
			}
		})
	}
}

// TestLeadingSeparatorRoundTrip is the requirement yamlfmt cannot meet: the
// decision is per file, not global. Flux exports open with ---, kustomize
// output does not, and both must survive a format pass unchanged.
func TestLeadingSeparatorRoundTrip(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"flux style", "---\napiVersion: v1\nkind: ConfigMap\n", true},
		{"kustomize style", "apiVersion: v1\nkind: ConfigMap\n", false},
		{"separator after comments", "# header\n---\na: 1\n", true},
		{"separator after blank lines", "\n\n---\na: 1\n", true},
		{"content on separator line", "--- !tag\na: 1\n", false},
		{"separator only between docs", "a: 1\n---\nb: 2\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := format.Format([]byte(tc.in))
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			if has := bytes.HasPrefix(got, []byte("---\n")); has != tc.want {
				t.Errorf("leading separator = %v, want %v\ngot:\n%s", has, tc.want, got)
			}
		})
	}
}

// TestParseErrorsAreReported: malformed YAML must fail loudly, never silently
// produce truncated output that a -w run would then write to disk.
func TestParseError(t *testing.T) {
	t.Parallel()
	if _, err := format.Format([]byte("a: [1, 2\nb: {\n")); err == nil {
		t.Fatal("expected a parse error, got nil")
	}
}

// TestFlowCommentsStayAttached is a regression test for a real bug: yaml.v3
// stores the trailing comment of a flow collection on the flow node itself, so
// converting that node to block style made the encoder flush the comment onto
// the *next* key. Silently reattaching a comment to a different field is the
// worst thing a formatter can do, because the diff looks harmless.
func TestFlowCommentsStayAttached(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "map value keeps its comment on the key",
			in:   "labels: {a: b} # note\nafter: 1\n",
			want: "labels: # note\n  a: b\nafter: 1\n",
		},
		{
			name: "sequence value keeps its comment on the key",
			in:   "args: [x, y] # note\nafter: 1\n",
			want: "args: # note\n- x\n- y\nafter: 1\n",
		},
		{
			name: "sequence element comment moves above the item",
			in:   "list:\n  - {k: v} # note\nafter: 1\n",
			want: "list:\n# note\n- k: v\nafter: 1\n",
		},
		{
			name: "empty flow collections keep their comment in place",
			in:   "empty: {} # note\nafter: 1\n",
			want: "empty: {} # note\nafter: 1\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := format.Format([]byte(tc.in))
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			if string(got) != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tc.want)
			}
			if !strings.Contains(string(got), "# note") {
				t.Error("comment was lost entirely")
			}
		})
	}
}

// TestNoCommentIsLost checks the weaker but broader invariant across the whole
// corpus: every comment present in the input is still present in the output.
// Placement is pinned by the goldens; this catches outright disappearance.
func TestNoCommentIsLost(t *testing.T) {
	for _, name := range goldenCases(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in, err := os.ReadFile(filepath.Join("testdata", name+".input.yaml"))
			if err != nil {
				t.Fatalf("read input: %v", err)
			}
			got, err := format.Format(in)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			for _, want := range commentTexts(string(in)) {
				if !strings.Contains(string(got), want) {
					t.Errorf("comment %q vanished from output:\n%s", want, got)
				}
			}
		})
	}
}

// commentTexts extracts whole-line comments, which are unambiguous to find.
// Inline comments are skipped here because a "#" inside a quoted scalar is not
// a comment; the goldens pin those cases instead.
func commentTexts(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if t := strings.TrimSpace(line); strings.HasPrefix(t, "#") {
			out = append(out, t)
		}
	}
	return out
}

// TestMergeKeyStaysPlain guards against the encoder writing the explicit
// !!merge tag it resolves "<<" to. The tagged form is equivalent but churns
// every overlay that uses a merge key, which in GitOps repos is many of them.
func TestMergeKeyStaysPlain(t *testing.T) {
	t.Parallel()
	in := []byte("base: &base\n  a: 1\nchild:\n  <<: *base\n  b: 2\n")
	got, err := format.Format(in)
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if bytes.Contains(got, []byte("!!merge")) {
		t.Errorf("explicit !!merge tag was emitted:\n%s", got)
	}
	if !bytes.Contains(got, []byte("<<: *base")) {
		t.Errorf("merge key was altered:\n%s", got)
	}
}

// TestCRLFNormalisedToLF documents the choice: mixed line endings in a repo are
// a formatting inconsistency like any other, and LF is what the ecosystem's
// emitters produce.
func TestCRLFNormalisedToLF(t *testing.T) {
	t.Parallel()
	got, err := format.Format([]byte("a: 1\r\nlist:\r\n  - x\r\n"))
	if err != nil {
		t.Fatalf("Format: %v", err)
	}
	if bytes.Contains(got, []byte("\r")) {
		t.Errorf("carriage returns survived: %q", got)
	}
}
