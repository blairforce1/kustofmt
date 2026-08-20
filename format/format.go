// Package format implements kustomize's house YAML style as a formatter.
//
// The style is not invented here: it is whatever sigs.k8s.io/kustomize/kyaml
// emits, which is what kustomize and Flux write into GitOps repositories.
// kustofmt exists to close the gap between that emitted style and the styles
// produced by the tools people edit YAML with.
//
// The contract, in full:
//
//   - Two-space map indentation.
//   - Sequence items are not indented relative to their key ("compact" or
//     "indentless" sequences).
//   - Block style everywhere, except genuinely empty collections ({} and []),
//     which have no block form and stay flow.
//   - Comments, scalar quoting, anchors, aliases and block scalars are the
//     encoder's business and are preserved.
//   - Key order is preserved. kustofmt is a style formatter, not a linter;
//     it never reorders your fields.
//   - A leading document separator round-trips: files that have one keep it,
//     files that do not stay bare.
package format

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// mergeTag is the resolved tag yaml.v3 assigns to a "<<" merge key.
const mergeTag = "!!merge"

// maxPasses bounds the convergence loop in Format.
const maxPasses = 4

// ErrNotConverged is returned when repeated formatting never reaches a fixed
// point. It should not happen; it exists so that it cannot happen silently.
var ErrNotConverged = errors.New("formatting did not converge")

// ErrSemanticsChanged is returned when formatting would alter what the document
// means. Callers should leave the file alone and report it.
var ErrSemanticsChanged = errors.New("formatting would change the document's meaning")

// Format applies kustomize's house style to a YAML document stream.
//
// Input that parses to zero documents -- an empty file, or one containing only
// comments -- is returned unchanged. The YAML object model cannot represent a
// document that has no content, so re-emitting such a file would discard its
// comments entirely. A formatter that eats comments is a vandal.
//
// The result is checked before it is returned: it must be a fixed point, and it
// must decode to exactly the same values as the input. Both checks defend
// against defects in the underlying YAML emitter, which are not hypothetical --
// see verifySemantics. A formatter that cannot vouch for its output should
// refuse to produce it rather than hand back a file that looks fine.
func Format(in []byte) ([]byte, error) {
	out, err := formatOnce(in)
	if err != nil {
		return nil, err
	}
	// Check meaning before stability. A semantics change is both the more
	// serious failure and the more specific diagnosis, so it should be the
	// error the user sees when a document manages to trigger both.
	if err := verifySemantics(in, out); err != nil {
		return nil, err
	}

	// Converge. The emitter is not perfectly stable for every comment
	// placement: a foot comment can gain a blank line on the pass after it is
	// first written. Iterating to a fixed point makes Format idempotent by
	// construction, which is what -l and pre-commit hooks depend on.
	for pass := 1; ; pass++ {
		next, err := formatOnce(out)
		if err != nil {
			return nil, err
		}
		if err := verifySemantics(in, next); err != nil {
			return nil, err
		}
		if bytes.Equal(next, out) {
			return out, nil
		}
		if pass >= maxPasses {
			return nil, fmt.Errorf("%w after %d passes", ErrNotConverged, maxPasses)
		}
		out = next
	}
}

// formatOnce is a single formatting pass.
func formatOnce(in []byte) ([]byte, error) {
	docs, err := parse(in)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return in, nil
	}

	for _, doc := range docs {
		blockify(doc)
	}

	var buf bytes.Buffer
	if hasLeadingSeparator(in) {
		buf.WriteString("---\n")
	}
	enc := yaml.NewEncoder(&buf)
	// Both calls are kyaml's own encoder defaults (yaml/alias.go). Setting them
	// explicitly is the point: this pair *is* the style contract, and a silent
	// upstream default change should break a golden test, not our output.
	enc.SetIndent(yaml.DefaultIndent)
	enc.CompactSeqIndent()
	for _, doc := range docs {
		if err := enc.Encode(doc); err != nil {
			return nil, err
		}
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// verifySemantics checks that formatting changed only presentation.
//
// This is not defensive programming for its own sake. The underlying emitter
// corrupts folded scalars whose content contains an indented line: given
//
//	key: >
//	  one
//	   two
//
// it re-emits a value with an extra newline in it. Detecting that and refusing
// is the difference between a formatter and a silent data-loss bug, and the
// cost is one extra parse of a file we have already parsed twice.
func verifySemantics(in, out []byte) error {
	before, err := decodeValues(in)
	if err != nil {
		// The input does not reduce to plain values (duplicate keys, say).
		// There is nothing to compare against, so do not invent a failure.
		return nil //nolint:nilerr // deliberate: no comparison is possible
	}
	after, err := decodeValues(out)
	if err != nil {
		return fmt.Errorf("formatted output does not parse: %w", err)
	}
	if !reflect.DeepEqual(before, after) {
		return ErrSemanticsChanged
	}
	return nil
}

// decodeValues reduces a stream to plain Go values, discarding everything the
// formatter is allowed to change and keeping everything it must not.
func decodeValues(b []byte) ([]any, error) {
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

// parse decodes every document in the stream. A stream of only comments or
// whitespace yields no documents and no error.
func parse(in []byte) ([]*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(in))
	var docs []*yaml.Node
	for {
		var doc yaml.Node
		err := dec.Decode(&doc)
		if errors.Is(err, io.EOF) {
			return docs, nil
		}
		if err != nil {
			return nil, err
		}
		docs = append(docs, &doc)
	}
}

// blockify recursively forces block style on every collection that has
// content. Empty collections are left alone: {} and [] have no block
// representation, so clearing their style would produce a bare key with a
// null value, which is a different document.
func blockify(n *yaml.Node) {
	if n == nil {
		return
	}
	if isCollection(n) && len(n.Content) > 0 {
		n.Style = 0
	}
	switch n.Kind {
	case yaml.MappingNode:
		// Content alternates key, value, key, value...
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i], n.Content[i+1]
			unquoteMergeKey(key)
			relocateLineComment(val, key)
			blockify(key)
			blockify(val)
		}
	case yaml.SequenceNode:
		for _, el := range n.Content {
			relocateLineComment(el, nil)
			blockify(el)
		}
	default:
		for _, c := range n.Content {
			relocateLineComment(c, nil)
			blockify(c)
		}
	}
}

// unquoteMergeKey strips the explicit !!merge tag from a merge key.
//
// The parser tags "<<" as !!merge, and the encoder then writes that tag out
// explicitly, turning "<<: *defaults" into "!!merge <<: *defaults". The two are
// equivalent -- "<<" resolves to !!merge implicitly on the way back in -- but
// the tagged form is noise, and merge keys are common enough in GitOps
// overlays that emitting it would churn every file that uses one.
func unquoteMergeKey(key *yaml.Node) {
	if key.Kind == yaml.ScalarNode && key.Value == "<<" && key.Tag == mergeTag {
		key.Tag = ""
	}
}

func isCollection(n *yaml.Node) bool {
	return n.Kind == yaml.MappingNode || n.Kind == yaml.SequenceNode
}

// relocateLineComment rescues the trailing comment of a flow collection before
// that collection is converted to block style.
//
// yaml.v3 stores "key: {a: b} # note" as a LineComment on the flow node. Once
// the node spans several lines the comment can no longer sit at its end, so the
// encoder flushes it onto whatever line comes next -- silently reattaching it
// to the following key. Moving it first keeps it on the line it was written on:
//
//	labels: {a: b} # note     becomes     labels: # note
//	                                        a: b
//
// A sequence element has no key to hold the comment, so it moves above the item
// rather than being lost or misplaced.
func relocateLineComment(n, key *yaml.Node) {
	if n == nil || n.LineComment == "" || !isCollection(n) || len(n.Content) == 0 {
		return
	}
	if n.Style&yaml.FlowStyle == 0 {
		return // already block: the comment is already on its own line
	}
	comment := n.LineComment
	n.LineComment = ""
	switch {
	case key == nil:
		n.HeadComment = joinComments(n.HeadComment, comment)
	case key.LineComment == "":
		key.LineComment = comment
	default:
		key.LineComment = joinComments(key.LineComment, comment)
	}
}

func joinComments(existing, added string) string {
	if existing == "" {
		return added
	}
	return existing + "\n" + added
}

// hasLeadingSeparator reports whether the stream opens with a bare "---".
//
// The encoder writes separators between documents but never before the first,
// so a leading separator is dropped on round-trip unless we put it back. Flux
// exports carry one; kustomize output does not. Both must survive.
//
// Only a bare separator counts. "--- foo" and "--- !tag" carry content or a
// tag on the same line and are the encoder's business, not ours.
func hasLeadingSeparator(in []byte) bool {
	for _, line := range strings.Split(string(in), "\n") {
		trimmed := strings.TrimRight(line, " \t\r")
		switch {
		case trimmed == "":
			continue // leading blank lines
		case strings.HasPrefix(trimmed, "#"):
			continue // comments may precede the separator
		default:
			return trimmed == "---"
		}
	}
	return false
}
