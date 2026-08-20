package format

import (
	"fmt"
	"strings"
)

// contextLines is the number of unchanged lines shown either side of a change,
// matching the conventional `diff -u` default.
const contextLines = 3

// maxDiffCells bounds the LCS table. Beyond it the diff degrades to a whole
// file replacement, which is still correct output, just less readable. YAML
// files that large are generated, and nobody reads their diffs line by line.
const maxDiffCells = 4 << 20

// Diff renders a unified diff between two versions of a file.
//
// It returns the empty string when the inputs are identical, so callers can use
// it directly as a "did anything change" test.
func Diff(oldName, newName string, old, new []byte) string {
	if string(old) == string(new) {
		return ""
	}
	oldLines, newLines := splitLines(old), splitLines(new)
	hunks := hunks(oldLines, newLines)
	if len(hunks) == 0 {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- %s\n", oldName)
	fmt.Fprintf(&b, "+++ %s\n", newName)
	for _, h := range hunks {
		b.WriteString(h.String())
	}
	return b.String()
}

// splitLines splits into lines without a trailing empty element, so a file
// ending in a newline does not appear to have a final blank line.
func splitLines(b []byte) []string {
	s := string(b)
	if s == "" {
		return nil
	}
	s = strings.TrimSuffix(s, "\n")
	return strings.Split(s, "\n")
}

type opKind int

const (
	opEqual opKind = iota
	opDelete
	opInsert
)

type edit struct {
	kind opKind
	line string
}

type hunk struct {
	oldStart, oldCount int
	newStart, newCount int
	edits              []edit
}

func (h hunk) String() string {
	var b strings.Builder
	fmt.Fprintf(&b, "@@ -%s +%s @@\n", rng(h.oldStart, h.oldCount), rng(h.newStart, h.newCount))
	for _, e := range h.edits {
		switch e.kind {
		case opEqual:
			b.WriteString(" " + e.line + "\n")
		case opDelete:
			b.WriteString("-" + e.line + "\n")
		case opInsert:
			b.WriteString("+" + e.line + "\n")
		}
	}
	return b.String()
}

// rng formats a hunk range header. A single-line range omits the count, and an
// empty range is reported at the preceding line, both per the unified format.
func rng(start, count int) string {
	if count == 0 {
		return fmt.Sprintf("%d,0", start)
	}
	if count == 1 {
		return fmt.Sprintf("%d", start+1)
	}
	return fmt.Sprintf("%d,%d", start+1, count)
}

// hunks builds the edit script and groups it into hunks with surrounding
// context.
func hunks(old, new []string) []hunk {
	edits := diffLines(old, new)

	var out []hunk
	var cur *hunk
	oldLine, newLine := 0, 0
	// pending holds trailing context that may become leading context for the
	// next hunk, or be flushed if the hunks are far enough apart to split.
	var pending []edit

	flush := func() {
		if cur != nil {
			out = append(out, *cur)
			cur = nil
		}
		pending = nil
	}

	for _, e := range edits {
		if e.kind == opEqual {
			if cur == nil {
				pending = append(pending, e)
				if len(pending) > contextLines {
					pending = pending[1:]
				}
			} else {
				pending = append(pending, e)
				if len(pending) > 2*contextLines {
					// Far enough from the last change to close this hunk,
					// keeping only its trailing context.
					cur.edits = append(cur.edits, pending[:contextLines]...)
					cur.oldCount += contextLines
					cur.newCount += contextLines
					out = append(out, *cur)
					cur = nil
					pending = pending[len(pending)-contextLines:]
				}
			}
			oldLine++
			newLine++
			continue
		}

		if cur == nil {
			cur = &hunk{oldStart: oldLine - len(pending), newStart: newLine - len(pending)}
			cur.edits = append(cur.edits, pending...)
			cur.oldCount += len(pending)
			cur.newCount += len(pending)
		} else {
			cur.edits = append(cur.edits, pending...)
			cur.oldCount += len(pending)
			cur.newCount += len(pending)
		}
		pending = nil

		cur.edits = append(cur.edits, e)
		switch e.kind {
		case opDelete:
			cur.oldCount++
			oldLine++
		case opInsert:
			cur.newCount++
			newLine++
		}
	}

	if cur != nil {
		n := min(len(pending), contextLines)
		cur.edits = append(cur.edits, pending[:n]...)
		cur.oldCount += n
		cur.newCount += n
	}
	flush()
	return out
}

// diffLines produces an edit script via longest-common-subsequence, after
// trimming the common prefix and suffix. For a formatter the changes are
// usually a few scattered lines, so the trim does most of the work.
func diffLines(old, new []string) []edit {
	var pre, post []edit

	i := 0
	for i < len(old) && i < len(new) && old[i] == new[i] {
		pre = append(pre, edit{opEqual, old[i]})
		i++
	}
	old, new = old[i:], new[i:]

	j := 0
	for j < len(old) && j < len(new) && old[len(old)-1-j] == new[len(new)-1-j] {
		j++
	}
	for k := len(old) - j; k < len(old); k++ {
		post = append(post, edit{opEqual, old[k]})
	}
	old, new = old[:len(old)-j], new[:len(new)-j]

	var mid []edit
	if len(old)*len(new) > maxDiffCells {
		for _, l := range old {
			mid = append(mid, edit{opDelete, l})
		}
		for _, l := range new {
			mid = append(mid, edit{opInsert, l})
		}
	} else {
		mid = lcsEdits(old, new)
	}

	return append(append(pre, mid...), post...)
}

// lcsEdits walks a longest-common-subsequence table to build the edit script.
func lcsEdits(old, new []string) []edit {
	n, m := len(old), len(new)
	if n == 0 && m == 0 {
		return nil
	}
	// table[i][j] = LCS length of old[i:] and new[j:]
	table := make([][]int, n+1)
	for i := range table {
		table[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if old[i] == new[j] {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}

	var out []edit
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case old[i] == new[j]:
			out = append(out, edit{opEqual, old[i]})
			i++
			j++
		case table[i+1][j] >= table[i][j+1]:
			out = append(out, edit{opDelete, old[i]})
			i++
		default:
			out = append(out, edit{opInsert, new[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, edit{opDelete, old[i]})
	}
	for ; j < m; j++ {
		out = append(out, edit{opInsert, new[j]})
	}
	return out
}
