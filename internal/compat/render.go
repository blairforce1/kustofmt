package compat

import (
	"bytes"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/blairforce1/kustofmt/format"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

// Marker delimits the generated table inside a documentation file. Regenerating
// between markers keeps the surrounding prose editable by hand while the table
// itself stays derived from the matrix.
const (
	MarkerBegin = "<!-- compat:begin -->"
	MarkerEnd   = "<!-- compat:end -->"
)

// Save writes the matrix back to disk in kustofmt's own house style, so the
// file the tool maintains is a file the tool would not reformat.
func Save(path string, m *Matrix) error {
	m.sort()
	if err := m.validate(); err != nil {
		return err
	}
	body, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	styled, err := format.Format(body)
	if err != nil {
		return fmt.Errorf("formatting %s: %w", path, err)
	}
	return os.WriteFile(path, append([]byte(fileHeader), styled...), 0o644)
}

const fileHeader = `# The kustomize compatibility matrix -- the source of truth for the tables in
# README.md and CHANGELOG.md, which are generated from this file.
#
# Each kustofmt release links exactly one kyaml, the library whose emitter
# defines the output style. ` + "`kustomize`" + ` lists every kustomize CLI release
# shipping that same kyaml, so a repository pinning a kustomize version can pin
# the kustofmt built from the same library.
#
# ` + "`version`" + ` is the release this tree publishes. It is usually the newest row,
# but a change with no kyaml behind it -- packaging, signing -- advances the
# version without adding a row, because one row per kyaml is an invariant.
#
# Maintained by ` + "`go run ./cmd/compat`" + `; every row is re-derived from upstream by
# ` + "`make compat-check`" + `. Tracking starts at ` + "`floor`" + ` -- older kustomize releases are
# deliberately absent rather than accidentally missing.
`

// sort puts releases and their kustomize lists in version order. Left unsorted,
// a matrix drifts into an order that reads as meaningful but is not.
func (m *Matrix) sort() {
	sort.SliceStable(m.Releases, func(i, j int) bool {
		return CompareVersions(m.Releases[i].Kustofmt, m.Releases[j].Kustofmt) < 0
	})
	for i := range m.Releases {
		sort.SliceStable(m.Releases[i].Kustomize, func(a, b int) bool {
			return CompareVersions(m.Releases[i].Kustomize[a], m.Releases[i].Kustomize[b]) < 0
		})
	}
}

// Table renders the matrix as Markdown, oldest release first -- the same order
// as the file, so there is one canonical ordering and nothing to reconcile.
//
// The line below the table names the current release, which is not always the
// newest row: a release with no kyaml behind it advances the version without
// adding one. It is emitted unconditionally rather than only when the two
// differ, because a conditional line is a branch to get wrong for no benefit.
func (m *Matrix) Table() string {
	var b strings.Builder
	b.WriteString("| kustofmt | kyaml | kustomize CLI |\n")
	b.WriteString("|----------|-------|---------------|\n")
	for _, r := range m.Releases {
		versions := "none yet"
		if len(r.Kustomize) > 0 {
			versions = strings.Join(r.Kustomize, ", ")
		}
		fmt.Fprintf(&b, "| %s | %s | %s |\n", r.Kustofmt, r.Kyaml, versions)
	}
	fmt.Fprintf(&b, "\nThe current release is **%s**, built against kyaml %s. Each row is the\n"+
		"release that first linked that kyaml; later releases linking the same kyaml\n"+
		"emit identical output.\n", m.Version, m.Current().Kyaml)
	return b.String()
}

// UpdateDoc replaces the marked region of a documentation file with the current
// table. It reports whether the file changed.
func UpdateDoc(path string, m *Matrix) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	out, err := replaceMarked(src, m.Table())
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	if bytes.Equal(src, out) {
		return false, nil
	}
	return true, os.WriteFile(path, out, 0o644)
}

// CheckDoc reports whether the marked region of a file already matches.
func CheckDoc(path string, m *Matrix) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}
	out, err := replaceMarked(src, m.Table())
	if err != nil {
		return false, fmt.Errorf("%s: %w", path, err)
	}
	return bytes.Equal(src, out), nil
}

func replaceMarked(src []byte, table string) ([]byte, error) {
	begin := bytes.Index(src, []byte(MarkerBegin))
	end := bytes.Index(src, []byte(MarkerEnd))
	if begin < 0 || end < 0 {
		return nil, fmt.Errorf("missing %s / %s markers", MarkerBegin, MarkerEnd)
	}
	if end < begin {
		return nil, fmt.Errorf("%s appears before %s", MarkerEnd, MarkerBegin)
	}
	var out bytes.Buffer
	out.Write(src[:begin])
	out.WriteString(MarkerBegin)
	out.WriteString("\n")
	out.WriteString(table)
	out.Write(src[end:])
	return out.Bytes(), nil
}
