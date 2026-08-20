package compat_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/blairforce1/kustofmt/format"
	"github.com/blairforce1/kustofmt/internal/compat"
)

// fixture is the shape the real matrix has after the historical replay.
func fixture() *compat.Matrix {
	return &compat.Matrix{
		Floor: "5.6.0",
		Releases: []compat.Release{
			{Kustofmt: "0.1.0", Kyaml: "v0.19.0", Kustomize: []string{"5.6.0"}},
			{Kustofmt: "0.1.1", Kyaml: "v0.20.0", Kustomize: []string{"5.7.0"}},
			{Kustofmt: "0.1.2", Kyaml: "v0.20.1", Kustomize: []string{"5.7.1"}},
		},
	}
}

func TestDecide(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		kustomize  string
		kyaml      string
		wantAction compat.Action
		wantTarget string
	}{
		{
			name:       "already recorded",
			kustomize:  "5.7.1",
			kyaml:      "v0.20.1",
			wantAction: compat.ActionNone,
			wantTarget: "0.1.2",
		},
		{
			name:       "new release shipping a kyaml we already build against",
			kustomize:  "5.7.2",
			kyaml:      "v0.20.1",
			wantAction: compat.ActionMatrixOnly,
			wantTarget: "0.1.2",
		},
		{
			name:       "new release shipping a new kyaml",
			kustomize:  "5.8.0",
			kyaml:      "v0.21.0",
			wantAction: compat.ActionRebuild,
			wantTarget: "0.1.3",
		},
		{
			// Upstream shipping a *lower* kyaml than the newest release is not
			// hypothetical: kustomize v5.4.1 shipped the same kyaml as v5.4.0.
			// It must join the existing row, not mint a version for a library
			// kustofmt has already been built against.
			name:       "new release downgrading to an older recorded kyaml",
			kustomize:  "5.7.3",
			kyaml:      "v0.19.0",
			wantAction: compat.ActionMatrixOnly,
			wantTarget: "0.1.0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := fixture().Decide(tc.kustomize, tc.kyaml)
			if got.Action != tc.wantAction {
				t.Errorf("action = %q, want %q", got.Action, tc.wantAction)
			}
			if got.Target != tc.wantTarget {
				t.Errorf("target = %q, want %q", got.Target, tc.wantTarget)
			}
		})
	}
}

func TestNextVersion(t *testing.T) {
	t.Parallel()
	if got := compat.NextPatch("0.1.2"); got != "0.1.3" {
		t.Errorf("NextPatch = %q, want 0.1.3", got)
	}
	if got := compat.NextMinor("0.1.2"); got != "0.2.0" {
		t.Errorf("NextMinor = %q, want 0.2.0", got)
	}
	if got := compat.NextPatch("0.1.9"); got != "0.1.10" {
		t.Errorf("NextPatch = %q, want 0.1.10", got)
	}
}

// TestCompareVersions: sorting these as strings puts 5.10.0 before 5.9.0, which
// is how a matrix ends up quietly in the wrong order.
func TestCompareVersions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		a, b string
		want int
	}{
		{"5.9.0", "5.10.0", -1},
		{"5.10.0", "5.9.0", 1},
		{"5.7.1", "5.7.1", 0},
		{"v0.20.1", "v0.21.0", -1},
		{"0.1.2", "0.1.10", -1},
	}
	for _, tc := range tests {
		if got := compat.CompareVersions(tc.a, tc.b); got != tc.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}

func TestLoadRejectsImpossibleMatrices(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		yaml string
		want string
	}{
		{
			name: "two releases claiming the same kyaml",
			yaml: "floor: 5.6.0\nreleases:\n- kustofmt: 0.1.0\n  kyaml: v0.19.0\n  kustomize: [5.6.0]\n- kustofmt: 0.1.1\n  kyaml: v0.19.0\n  kustomize: [5.7.0]\n",
			want: "appears in both",
		},
		{
			name: "one kustomize release in two rows",
			yaml: "floor: 5.6.0\nreleases:\n- kustofmt: 0.1.0\n  kyaml: v0.19.0\n  kustomize: [5.6.0]\n- kustofmt: 0.1.1\n  kyaml: v0.20.0\n  kustomize: [5.6.0]\n",
			want: "appears in both",
		},
		{
			name: "no floor",
			yaml: "releases:\n- kustofmt: 0.1.0\n  kyaml: v0.19.0\n  kustomize: [5.6.0]\n",
			want: "floor is required",
		},
		{
			name: "kyaml without a leading v",
			yaml: "floor: 5.6.0\nreleases:\n- kustofmt: 0.1.0\n  kyaml: 0.19.0\n  kustomize: [5.6.0]\n",
			want: "leading v",
		},
		{
			name: "kustofmt with a leading v",
			yaml: "floor: 5.6.0\nreleases:\n- kustofmt: v0.1.0\n  kyaml: v0.19.0\n  kustomize: [5.6.0]\n",
			want: "should not carry a leading v",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "compatibility.yaml")
			if err := os.WriteFile(path, []byte(tc.yaml), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := compat.Load(path)
			if err == nil {
				t.Fatal("expected an error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %q, want it to mention %q", err, tc.want)
			}
		})
	}
}

func TestTable(t *testing.T) {
	t.Parallel()
	want := "| kustofmt | kyaml | kustomize CLI |\n" +
		"|----------|-------|---------------|\n" +
		"| 0.1.0 | v0.19.0 | 5.6.0 |\n" +
		"| 0.1.1 | v0.20.0 | 5.7.0 |\n" +
		"| 0.1.2 | v0.20.1 | 5.7.1 |\n"
	if got := fixture().Table(); got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestTableJoinsMultipleKustomizeVersions(t *testing.T) {
	t.Parallel()
	m := fixture()
	m.Releases[2].Kustomize = append(m.Releases[2].Kustomize, "5.7.2")
	if got := m.Table(); !strings.Contains(got, "| 0.1.2 | v0.20.1 | 5.7.1, 5.7.2 |") {
		t.Errorf("expected a joined list, got:\n%s", got)
	}
}

func TestUpdateAndCheckDoc(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	src := "# Title\n\nprose above\n\n" + compat.MarkerBegin + "\nstale table\n" + compat.MarkerEnd + "\n\nprose below\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	m := fixture()

	if ok, err := compat.CheckDoc(path, m); err != nil || ok {
		t.Fatalf("CheckDoc on a stale file = %v, %v; want false, nil", ok, err)
	}
	changed, err := compat.UpdateDoc(path, m)
	if err != nil || !changed {
		t.Fatalf("UpdateDoc = %v, %v; want true, nil", changed, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"prose above", "prose below", "| 0.1.0 | v0.19.0 | 5.6.0 |"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("output missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(string(got), "stale table") {
		t.Error("the stale table survived")
	}
	// Second run is a no-op: generation must be deterministic.
	if changed, err := compat.UpdateDoc(path, m); err != nil || changed {
		t.Errorf("second UpdateDoc = %v, %v; want false, nil", changed, err)
	}
	if ok, err := compat.CheckDoc(path, m); err != nil || !ok {
		t.Errorf("CheckDoc after update = %v, %v; want true, nil", ok, err)
	}
}

func TestUpdateDocWithoutMarkersFails(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(path, []byte("# no markers here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := compat.UpdateDoc(path, fixture()); err == nil {
		t.Fatal("expected an error when the markers are missing")
	}
}

// TestSaveWritesHouseStyle: the file the tool maintains must be a file the tool
// would not reformat, or the self-hosting check fails on its own output.
func TestSaveWritesHouseStyle(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "compatibility.yaml")
	if err := compat.Save(path, fixture()); err != nil {
		t.Fatal(err)
	}
	back, err := compat.Load(path)
	if err != nil {
		t.Fatalf("saved file does not load: %v", err)
	}
	if len(back.Releases) != 3 || back.Floor != "5.6.0" {
		t.Errorf("round-trip lost data: %+v", back)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	// The tool's own verdict is the definition of house style. Eyeballing the
	// indentation is not: a nested indentless sequence still carries leading
	// whitespace from its parent, so a naive check reads it as indented.
	formatted, err := format.Format(raw)
	if err != nil {
		t.Fatalf("saved file does not format: %v", err)
	}
	if !bytes.Equal(raw, formatted) {
		t.Errorf("Save produced a file kustofmt would reformat:\n%s", format.Diff("saved", "formatted", raw, formatted))
	}
}

func TestSaveSortsOutOfOrderInput(t *testing.T) {
	t.Parallel()
	m := &compat.Matrix{
		Floor: "5.6.0",
		Releases: []compat.Release{
			{Kustofmt: "0.1.10", Kyaml: "v0.22.0", Kustomize: []string{"5.10.0", "5.9.0"}},
			{Kustofmt: "0.1.2", Kyaml: "v0.20.1", Kustomize: []string{"5.7.1"}},
		},
	}
	path := filepath.Join(t.TempDir(), "compatibility.yaml")
	if err := compat.Save(path, m); err != nil {
		t.Fatal(err)
	}
	back, err := compat.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if back.Releases[0].Kustofmt != "0.1.2" || back.Releases[1].Kustofmt != "0.1.10" {
		t.Errorf("releases not in version order: %+v", back.Releases)
	}
	if got := back.Releases[1].Kustomize; got[0] != "5.9.0" || got[1] != "5.10.0" {
		t.Errorf("kustomize versions not in version order: %v", got)
	}
}

// TestRecordSequence pins the historical replay: applying kustomize 5.7.0
// through 5.8.1 in order must yield kustofmt 0.1.1 through 0.1.4. Deciding each
// against the *unchanged* matrix instead would predict 0.1.1 four times, which
// is the bug Record exists to prevent.
func TestRecordSequence(t *testing.T) {
	t.Parallel()
	m := &compat.Matrix{
		Floor: "5.6.0",
		Releases: []compat.Release{
			{Kustofmt: "0.1.0", Kyaml: "v0.19.0", Kustomize: []string{"5.6.0"}},
		},
	}
	steps := []struct{ kustomize, kyaml, wantVersion string }{
		{"5.7.0", "v0.20.0", "0.1.1"},
		{"5.7.1", "v0.20.1", "0.1.2"},
		{"5.8.0", "v0.21.0", "0.1.3"},
		{"5.8.1", "v0.21.1", "0.1.4"},
	}
	for _, s := range steps {
		d := m.Decide(s.kustomize, s.kyaml)
		if d.Action != compat.ActionRebuild {
			t.Fatalf("%s: action = %q, want rebuild", s.kustomize, d.Action)
		}
		if d.Target != s.wantVersion {
			t.Fatalf("%s: target = %q, want %q", s.kustomize, d.Target, s.wantVersion)
		}
		m.Record(d)
	}
	if got := m.Current().Kustofmt; got != "0.1.4" {
		t.Errorf("final version = %q, want 0.1.4", got)
	}
	if len(m.Releases) != 5 {
		t.Errorf("got %d releases, want 5", len(m.Releases))
	}
}

// TestRecordMatrixOnlyExtendsTheRightRow: a kustomize release shipping a kyaml
// we already build against joins that row and cuts no version.
func TestRecordMatrixOnlyExtendsTheRightRow(t *testing.T) {
	t.Parallel()
	m := fixture()
	before := len(m.Releases)
	d := m.Decide("5.7.2", "v0.20.1")
	if d.Action != compat.ActionMatrixOnly {
		t.Fatalf("action = %q, want matrix-only", d.Action)
	}
	m.Record(d)
	if len(m.Releases) != before {
		t.Errorf("a release was cut: %d rows, want %d", len(m.Releases), before)
	}
	r, ok := m.ByKyaml("v0.20.1")
	if !ok {
		t.Fatal("row vanished")
	}
	if len(r.Kustomize) != 2 || r.Kustomize[1] != "5.7.2" {
		t.Errorf("row not extended: %v", r.Kustomize)
	}
}
