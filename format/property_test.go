package format_test

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/blairforce1/kustofmt/format"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// decodeAll reduces a stream to plain Go values, discarding everything the
// formatter is allowed to change (style, indentation, comments) and keeping
// everything it must not (keys, values, types, structure).
func decodeAll(b []byte) ([]any, error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	var out []any
	for {
		var v any
		err := dec.Decode(&v)
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
}

// requireSameSemantics is the property that makes the tool safe to run over a
// production GitOps repository: formatting changes bytes, never meaning.
func requireSameSemantics(t *testing.T, in, out []byte) {
	t.Helper()
	before, err := decodeAll(in)
	if err != nil {
		return // unparseable input is not a semantics question
	}
	after, err := decodeAll(out)
	if err != nil {
		t.Fatalf("formatted output no longer parses: %v\n%s", err, out)
	}
	if !reflect.DeepEqual(before, after) {
		t.Errorf("semantics changed\nbefore: %#v\nafter:  %#v\ninput:\n%s\noutput:\n%s", before, after, in, out)
	}
}

// TestPropertySemanticsPreserved runs the property over every fixture.
func TestPropertySemanticsPreserved(t *testing.T) {
	if testing.Short() {
		t.Skip("property test")
	}
	for _, name := range goldenCases(t) {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			in, err := os.ReadFile(filepath.Join("testdata", name+".input.yaml"))
			if err != nil {
				t.Fatalf("read: %v", err)
			}
			out, err := format.Format(in)
			if err != nil {
				t.Fatalf("Format: %v", err)
			}
			requireSameSemantics(t, in, out)
		})
	}
}

// TestPropertyGenerated exercises the invariants against randomly generated
// documents. The generator is seeded deterministically so a failure is
// reproducible from the test log alone.
func TestPropertyGenerated(t *testing.T) {
	if testing.Short() {
		t.Skip("property test")
	}
	const cases = 500
	for seed := range cases {
		t.Run(fmt.Sprintf("seed-%d", seed), func(t *testing.T) {
			t.Parallel()
			src := generateYAML(rand.New(rand.NewSource(int64(seed))))
			out, err := format.Format([]byte(src))
			if err != nil {
				t.Fatalf("Format failed on generated input: %v\n%s", err, src)
			}
			requireSameSemantics(t, []byte(src), out)

			twice, err := format.Format(out)
			if err != nil {
				t.Fatalf("second pass failed: %v", err)
			}
			if !bytes.Equal(out, twice) {
				t.Errorf("not idempotent\ninput:\n%s\nfirst:\n%s\nsecond:\n%s", src, out, twice)
			}

			if hasFlowCollection(t, out) {
				t.Errorf("non-empty flow collection survived formatting:\n%s", out)
			}
		})
	}
}

// hasFlowCollection reports whether any non-empty collection is still in flow
// style, which would mean the core style rule was not applied.
func hasFlowCollection(t *testing.T, b []byte) bool {
	t.Helper()
	dec := yaml.NewDecoder(bytes.NewReader(b))
	for {
		var n yaml.Node
		err := dec.Decode(&n)
		if errors.Is(err, io.EOF) {
			return false
		}
		if err != nil {
			t.Fatalf("output does not parse: %v", err)
		}
		if found := walkForFlow(&n); found {
			return true
		}
	}
}

func walkForFlow(n *yaml.Node) bool {
	if n == nil {
		return false
	}
	isColl := n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode
	if isColl && len(n.Content) > 0 && n.Style&yaml.FlowStyle != 0 {
		return true
	}
	for _, c := range n.Content {
		if walkForFlow(c) {
			return true
		}
	}
	return false
}

// generateYAML builds a random but valid document, biased towards the shapes
// this formatter cares about: flow collections, empty collections, comments,
// quoted scalars and nesting.
func generateYAML(r *rand.Rand) string {
	var b strings.Builder
	if r.Intn(4) == 0 {
		b.WriteString("---\n")
	}
	if r.Intn(3) == 0 {
		b.WriteString("# generated header\n")
	}
	docs := 1 + r.Intn(2)
	for d := range docs {
		if d > 0 {
			b.WriteString("---\n")
		}
		writeMapping(&b, r, 0, 0)
	}
	return b.String()
}

func writeMapping(b *strings.Builder, r *rand.Rand, depth, indent int) {
	pad := strings.Repeat(" ", indent)
	for i, n := 0, 1+r.Intn(4); i < n; i++ {
		key := fmt.Sprintf("key%d", i)
		if r.Intn(6) == 0 {
			fmt.Fprintf(b, "%s# comment on %s\n", pad, key)
		}
		switch pick := r.Intn(8); {
		case pick == 0 && depth < 3:
			fmt.Fprintf(b, "%s%s:\n", pad, key)
			writeMapping(b, r, depth+1, indent+2)
		case pick == 1:
			fmt.Fprintf(b, "%s%s: {}\n", pad, key)
		case pick == 2:
			fmt.Fprintf(b, "%s%s: []\n", pad, key)
		case pick == 3:
			fmt.Fprintf(b, "%s%s: {a: 1, b: \"two\"}\n", pad, key)
		case pick == 4:
			fmt.Fprintf(b, "%s%s: [1, \"two\", three]\n", pad, key)
		case pick == 5:
			fmt.Fprintf(b, "%s%s:\n%s  - one\n%s  - two\n", pad, key, pad, pad)
		case pick == 6:
			fmt.Fprintf(b, "%s%s: %q\n", pad, key, "1.36")
		default:
			fmt.Fprintf(b, "%s%s: value%d\n", pad, key, r.Intn(100))
		}
	}
}
