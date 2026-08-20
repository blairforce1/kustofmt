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
	"io"
	"strings"

	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// mergeTag is the resolved tag yaml.v3 assigns to a "<<" merge key.
const mergeTag = "!!merge"

// Format applies kustomize's house style to a YAML document stream.
//
// Input that parses to zero documents — an empty file, or one containing only
// comments — is returned unchanged. The YAML object model cannot represent a
// document that has no content, so re-emitting such a file would discard its
// comments entirely. A formatter that eats comments is a vandal.
func Format(in []byte) ([]byte, error) {
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
