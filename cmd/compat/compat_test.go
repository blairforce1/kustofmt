package main

// These tests drive the compat CLI in-process, against a throwaway repository
// and a substituted upstream. Running the real thing is not a test: its answers
// depend on what kustomize has published today, and the cases that matter --
// a release that downgrades kyaml, one whose emitter moves the golden corpus,
// a matrix that has drifted from go.mod -- either have not happened yet or
// cannot be made to happen on demand.
//
// They are in package main rather than main_test because substituting those
// seams is the whole point.
//
// Three things here are deliberately not covered: main, which exists only to
// call run and exit, and readGoModKyaml and runGoCmd, which drive the Go
// toolchain against a populated module. Exercising those means depending on a
// module cache and a network, which is the dependency this file removes.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"

	"github.com/blairforce1/kustofmt/internal/compat"
)

// fakeUpstream stands in for the module proxy, GitHub's tag list and the Go
// toolchain, and records what the tool asked the toolchain to do -- which is
// how the tests below check that a refused style change really did leave go.mod
// bumped for inspection, as its error message promises.
type fakeUpstream struct {
	kyaml     map[string]string // kustomize version -> the kyaml it links
	published []string          // what upstream has released, oldest first
	goModPin  string            // what go.mod currently links
	// goldensFail makes the golden-corpus run report failure, which is how
	// applyRebuild learns that the emitted style moved.
	goldensFail bool

	goCalls []string // every go invocation, in order
}

func (f *fakeUpstream) install(t *testing.T) {
	t.Helper()
	kyamlOrig, publishedOrig, goModOrig, goOrig := kyamlFor, publishedReleases, goModKyaml, goCmd
	t.Cleanup(func() {
		kyamlFor, publishedReleases, goModKyaml, goCmd = kyamlOrig, publishedOrig, goModOrig, goOrig
	})

	kyamlFor = func(version string) (string, error) {
		ky, ok := f.kyaml[strings.TrimPrefix(version, "v")]
		if !ok {
			return "", fmt.Errorf("no kyaml recorded for kustomize %s", version)
		}
		return ky, nil
	}
	publishedReleases = func(string) ([]string, error) { return f.published, nil }
	goModKyaml = func() (string, error) { return f.goModPin, nil }
	goCmd = func(args ...string) error {
		call := strings.Join(args, " ")
		f.goCalls = append(f.goCalls, call)
		if f.goldensFail && strings.Contains(call, "TestGolden") {
			return errors.New("golden corpus differs")
		}
		return nil
	}
}

func (f *fakeUpstream) ranGo(substr string) bool {
	for _, c := range f.goCalls {
		if strings.Contains(c, substr) {
			return true
		}
	}
	return false
}

// baseMatrix is the shape the real matrix has partway through the historical
// replay, with the version level with the newest row.
func baseMatrix() *compat.Matrix {
	return &compat.Matrix{
		Version: "0.1.2",
		Floor:   "5.6.0",
		Releases: []compat.Release{
			{Kustofmt: "0.1.0", Kyaml: "v0.19.0", Kustomize: []string{"5.6.0"}},
			{Kustofmt: "0.1.1", Kyaml: "v0.20.0", Kustomize: []string{"5.7.0"}},
			{Kustofmt: "0.1.2", Kyaml: "v0.20.1", Kustomize: []string{"5.7.1"}},
		},
	}
}

// knownKyaml matches baseMatrix, so `check` re-deriving every row agrees.
func knownKyaml() map[string]string {
	return map[string]string{"5.6.0": "v0.19.0", "5.7.0": "v0.20.0", "5.7.1": "v0.20.1"}
}

// repo writes a throwaway repository for the tool to operate on and chdirs into
// it: a go.mod so repoRoot finds the root, a matrix, and the two documents whose
// tables are generated. The documents start rendered, so a stale one in a test
// is stale because that test made it so.
func repo(t *testing.T, m *compat.Matrix) string {
	t.Helper()
	dir := t.TempDir()
	write(t, dir, "go.mod", "module example.com/fixture\n\ngo 1.26.0\n")
	write(t, dir, readmeFile, "# fixture\n\n"+compat.MarkerBegin+"\n"+compat.MarkerEnd+"\n")
	write(t, dir, changelogFile,
		"# Changelog\n\n## [Unreleased]\n\n## Compatibility\n\n"+compat.MarkerBegin+"\n"+compat.MarkerEnd+"\n")
	if err := compat.Save(filepath.Join(dir, matrixFile), m); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{readmeFile, changelogFile} {
		if _, err := compat.UpdateDoc(filepath.Join(dir, f), m); err != nil {
			t.Fatal(err)
		}
	}
	// t.Chdir restores the previous directory afterwards and forbids t.Parallel,
	// which is correct here: the tool chdirs to the repository root itself, and
	// the substituted seams are package-level.
	t.Chdir(dir)
	return dir
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// runCLI drives the tool exactly as main does, capturing both streams.
func runCLI(t *testing.T, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errs bytes.Buffer
	err = run(args, &out, &errs)
	return out.String(), errs.String(), err
}

// reload reads the matrix back from disk, which is the only way to tell whether
// a command actually persisted its decision.
func reload(t *testing.T) *compat.Matrix {
	t.Helper()
	m, err := compat.Load(matrixFile)
	if err != nil {
		t.Fatalf("matrix no longer loads: %v", err)
	}
	return m
}

// TestApplyMatrixOnly: a kustomize release shipping a kyaml some kustofmt
// release already links joins that row. Nothing is rebuilt and no version is
// cut, because the binary that already exists is provably the right one to pin.
func TestApplyMatrixOnly(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1"}
	up.kyaml["5.7.2"] = "v0.20.1"
	up.install(t)
	repo(t, baseMatrix())

	stdout, _, err := runCLI(t, "apply", "5.7.2")
	if err != nil {
		t.Fatalf("apply: %v", err)
	}

	m := reload(t)
	if m.Version != "0.1.2" {
		t.Errorf("version moved to %q; a matrix-only apply cuts no release", m.Version)
	}
	if len(m.Releases) != 3 {
		t.Errorf("got %d rows, want 3: no row should have been added", len(m.Releases))
	}
	r, ok := m.ByKyaml("v0.20.1")
	if !ok {
		t.Fatal("the v0.20.1 row vanished")
	}
	if got := strings.Join(r.Kustomize, ","); got != "5.7.1,5.7.2" {
		t.Errorf("row covers %q, want 5.7.1,5.7.2", got)
	}
	if len(up.goCalls) != 0 {
		t.Errorf("the toolchain was invoked for a matrix-only apply: %v", up.goCalls)
	}
	if !strings.Contains(read(t, readmeFile), "| 0.1.2 | v0.20.1 | 5.7.1, 5.7.2 |") {
		t.Error("README table was not regenerated")
	}
	if strings.Contains(read(t, changelogFile), "## [0.1.3]") {
		t.Error("a changelog entry was written for a release that was not cut")
	}
	if !strings.Contains(stdout, "no rebuild, no release") {
		t.Errorf("output does not say what happened:\n%s", stdout)
	}
}

// TestApplyRebuildTakesAPatch: a new kyaml that leaves the golden corpus alone
// changes nothing a user can observe, and the output style is this tool's public
// API -- so it is a patch.
func TestApplyRebuildTakesAPatch(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1"}
	up.kyaml["5.8.0"] = "v0.21.0"
	up.install(t)
	repo(t, baseMatrix())

	if _, _, err := runCLI(t, "apply", "5.8.0"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	m := reload(t)
	if m.Version != "0.1.3" {
		t.Errorf("version = %q, want 0.1.3", m.Version)
	}
	cur := m.Current()
	if cur.Kustofmt != "0.1.3" || cur.Kyaml != "v0.21.0" || strings.Join(cur.Kustomize, ",") != "5.8.0" {
		t.Errorf("new row = %+v, want 0.1.3 / v0.21.0 / [5.8.0]", cur)
	}
	if !up.ranGo("mod edit -require=" + kyamlModule + "@v0.21.0") {
		t.Errorf("go.mod was not repointed at the new kyaml: %v", up.goCalls)
	}
	if !up.ranGo("mod tidy") {
		t.Errorf("go mod tidy was not run: %v", up.goCalls)
	}
	if up.ranGo("-update") {
		t.Errorf("goldens were regenerated for an unchanged corpus: %v", up.goCalls)
	}

	log := read(t, changelogFile)
	if !regexp.MustCompile(`## \[0\.1\.3\] - \d{4}-\d{2}-\d{2}`).MatchString(log) {
		t.Errorf("no dated 0.1.3 section in the changelog:\n%s", log)
	}
	for _, want := range []string{"kyaml v0.21.0", "kustomize 5.8.0", "byte-identical output"} {
		if !strings.Contains(log, want) {
			t.Errorf("changelog entry does not mention %q", want)
		}
	}
	// The entry belongs directly below Unreleased, not at the end of the file.
	if strings.Index(log, "## [Unreleased]") > strings.Index(log, "## [0.1.3]") {
		t.Error("the new section was not inserted below Unreleased")
	}
}

// TestApplyRefusesAStyleChange: a kyaml whose emitter moves the golden corpus is
// a breaking change to kustofmt's output, and not something to decide
// automatically. The refusal must also leave the tree as its message claims.
func TestApplyRefusesAStyleChange(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1", goldensFail: true}
	up.kyaml["5.8.0"] = "v0.21.0"
	up.install(t)
	repo(t, baseMatrix())
	before := read(t, matrixFile)

	_, _, err := runCLI(t, "apply", "5.8.0")
	if err == nil {
		t.Fatal("apply accepted a style change without being asked to")
	}
	for _, want := range []string{"--allow-style-change", "breaking change", "make golden"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal does not mention %q:\n%v", want, err)
		}
	}
	if read(t, matrixFile) != before {
		t.Error("the matrix was written despite the refusal")
	}
	if strings.Contains(read(t, changelogFile), "## [0.1.3]") {
		t.Error("a changelog entry was written despite the refusal")
	}
	// The message tells the reader go.mod was left pointing at the new kyaml so
	// they can inspect the difference. That has to be true.
	if !up.ranGo("mod edit -require=" + kyamlModule + "@v0.21.0") {
		t.Errorf("go.mod was not left bumped, contradicting the error message: %v", up.goCalls)
	}
}

// TestApplyAcceptsAStyleChangeAsAMinor: told explicitly to accept it, the corpus
// is regenerated and the release takes a minor.
func TestApplyAcceptsAStyleChangeAsAMinor(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1", goldensFail: true}
	up.kyaml["5.8.0"] = "v0.21.0"
	up.install(t)
	repo(t, baseMatrix())

	if _, _, err := runCLI(t, "apply", "5.8.0", "--allow-style-change"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	m := reload(t)
	if m.Version != "0.2.0" {
		t.Errorf("version = %q, want 0.2.0: a style change is not a patch", m.Version)
	}
	if m.Current().Kustofmt != "0.2.0" {
		t.Errorf("new row = %q, want 0.2.0", m.Current().Kustofmt)
	}
	if !up.ranGo("-update") {
		t.Errorf("the golden corpus was not regenerated: %v", up.goCalls)
	}
	if !strings.Contains(read(t, changelogFile), "Output style changed") {
		t.Error("the changelog does not flag the style change")
	}
}

// TestApplyDowngradeJoinsTheOlderRow: upstream shipping a *lower* kyaml than the
// newest release is not hypothetical -- kustomize v5.4.1 shipped the same kyaml
// as v5.4.0. It must join the existing row rather than mint a version for a
// library kustofmt has already been built against.
func TestApplyDowngradeJoinsTheOlderRow(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1"}
	up.kyaml["5.9.0"] = "v0.19.0"
	up.install(t)
	repo(t, baseMatrix())

	if _, _, err := runCLI(t, "apply", "5.9.0"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	m := reload(t)
	if m.Version != "0.1.2" || len(m.Releases) != 3 {
		t.Errorf("a version was cut for an already-linked kyaml: version %q, %d rows", m.Version, len(m.Releases))
	}
	r, _ := m.ByKyaml("v0.19.0")
	if got := strings.Join(r.Kustomize, ","); got != "5.6.0,5.9.0" {
		t.Errorf("the v0.19.0 row covers %q, want 5.6.0,5.9.0", got)
	}
}

// TestApplyAlreadyRecorded: applying something already in the matrix touches
// nothing at all.
func TestApplyAlreadyRecorded(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1"}
	up.install(t)
	repo(t, baseMatrix())
	before := read(t, matrixFile)

	stdout, _, err := runCLI(t, "apply", "v5.7.1") // a leading v must be tolerated
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !strings.Contains(stdout, "nothing to do") {
		t.Errorf("output does not say it was a no-op:\n%s", stdout)
	}
	if read(t, matrixFile) != before {
		t.Error("the matrix was rewritten for a no-op")
	}
}

func TestApplyArgumentErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{"no version", []string{"apply"}, "needs a kustomize version"},
		{"two versions", []string{"apply", "5.8.0", "5.8.1"}, "exactly one"},
		{"unknown flag", []string{"apply", "5.8.0", "--force"}, `unknown flag "--force"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1"}
			up.install(t)
			repo(t, baseMatrix())
			_, _, err := runCLI(t, tc.args...)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error = %v, want it to mention %q", err, tc.want)
			}
		})
	}
}

// TestCheckPasses: the gate is quiet when go.mod, the documents and upstream all
// agree with the matrix.
func TestCheckPasses(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1", published: []string{"5.6.0", "5.7.0", "5.7.1"}}
	up.install(t)
	repo(t, baseMatrix())

	stdout, _, err := runCLI(t, "check")
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if !strings.Contains(stdout, "agrees with go.mod, the docs and upstream") {
		t.Errorf("unexpected output:\n%s", stdout)
	}
	if _, _, err := runCLI(t, "check", "--complete"); err != nil {
		t.Fatalf("check --complete: %v", err)
	}
}

// TestCheckDetectsProblems is the reason the seams exist: each of these is a way
// the matrix can be wrong, and none of them can be staged against real upstream.
func TestCheckDetectsProblems(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		arrange func(t *testing.T, up *fakeUpstream)
		want    string
	}{
		{
			name: "go.mod links a kyaml the matrix head does not claim",
			arrange: func(_ *testing.T, up *fakeUpstream) {
				up.goModPin = "v0.21.0"
			},
			want: "go.mod links kyaml v0.21.0",
		},
		{
			name: "a generated table is stale",
			arrange: func(t *testing.T, _ *fakeUpstream) {
				write(t, ".", readmeFile, "# fixture\n\n"+compat.MarkerBegin+"\nstale\n"+compat.MarkerEnd+"\n")
			},
			want: readmeFile + " is stale",
		},
		{
			name: "upstream contradicts a recorded row",
			arrange: func(_ *testing.T, up *fakeUpstream) {
				up.kyaml["5.7.1"] = "v0.20.2"
			},
			want: "matrix claims kustomize 5.7.1 ships kyaml v0.20.1, upstream says v0.20.2",
		},
		{
			name: "a published release is missing",
			args: []string{"--complete"},
			arrange: func(_ *testing.T, up *fakeUpstream) {
				up.published = append(up.published, "5.8.0")
			},
			want: "kustomize 5.8.0 is published but not recorded",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			up := &fakeUpstream{
				kyaml:     knownKyaml(),
				goModPin:  "v0.20.1",
				published: []string{"5.6.0", "5.7.0", "5.7.1"},
			}
			up.install(t)
			repo(t, baseMatrix())
			tc.arrange(t, up)

			_, stderr, err := runCLI(t, append([]string{"check"}, tc.args...)...)
			if err == nil {
				t.Fatal("check passed a matrix that is wrong")
			}
			if !strings.Contains(stderr, tc.want) {
				t.Errorf("the failure report does not mention %q:\n%s", tc.want, stderr)
			}
			if !strings.Contains(err.Error(), "compatibility problem") {
				t.Errorf("error = %v, want it to count the problems", err)
			}
		})
	}
}

func TestCheckRejectsUnknownFlags(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1"}
	up.install(t)
	repo(t, baseMatrix())
	if _, _, err := runCLI(t, "check", "--all"); err == nil || !strings.Contains(err.Error(), `unknown flag "--all"`) {
		t.Errorf("error = %v, want it to reject --all", err)
	}
}

// TestStatusForecastsInOrder: applying pending releases in order is what
// actually happens, so the forecast has to model that. Evaluating each against
// the unchanged matrix would predict the same version for all of them.
func TestStatusForecastsInOrder(t *testing.T) {
	up := &fakeUpstream{
		kyaml:     knownKyaml(),
		goModPin:  "v0.20.1",
		published: []string{"5.6.0", "5.7.0", "5.7.1", "5.8.0", "5.8.1"},
	}
	up.kyaml["5.8.0"] = "v0.21.0"
	up.kyaml["5.8.1"] = "v0.21.1"
	up.install(t)
	repo(t, baseMatrix())

	stdout, _, err := runCLI(t, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	for _, want := range []string{
		"release:      kustofmt 0.1.2",
		"unhandled kustomize releases (2)",
		"5.8.0",
		"kustofmt 0.1.3",
		"5.8.1",
		"kustofmt 0.1.4",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("status output is missing %q:\n%s", want, stdout)
		}
	}
}

// TestStatusFlagsDisagreement: the two lines a maintainer actually reads.
func TestStatusFlagsDisagreement(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.21.0", published: []string{"5.6.0", "5.7.0", "5.7.1"}}
	up.install(t)
	m := baseMatrix()
	m.Version = "0.1.3" // released for a reason kyaml did not cause
	repo(t, m)

	stdout, _, err := runCLI(t, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(stdout, "DISAGREES with the matrix head") {
		t.Errorf("a go.mod pin that contradicts the matrix was not flagged:\n%s", stdout)
	}
	if !strings.Contains(stdout, "ahead of the newest row") {
		t.Errorf("a version ahead of the rows was not explained:\n%s", stdout)
	}
	if !strings.Contains(stdout, "up to date: all 3 published") {
		t.Errorf("status did not report being up to date:\n%s", stdout)
	}
}

// TestNext prints one version and nothing else: the watcher consumes it without
// parsing human-facing output, and empty output with a zero exit means there is
// nothing to do.
func TestNext(t *testing.T) {
	up := &fakeUpstream{
		kyaml:     knownKyaml(),
		goModPin:  "v0.20.1",
		published: []string{"5.6.0", "5.7.0", "5.7.1", "5.8.0", "5.8.1"},
	}
	up.kyaml["5.8.0"] = "v0.21.0"
	up.kyaml["5.8.1"] = "v0.21.1"
	up.install(t)
	repo(t, baseMatrix())

	stdout, _, err := runCLI(t, "next")
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if stdout != "5.8.0\n" {
		t.Errorf("next printed %q, want exactly \"5.8.0\\n\" -- the watcher consumes this", stdout)
	}

	up.published = []string{"5.6.0", "5.7.0", "5.7.1"}
	stdout, _, err = runCLI(t, "next")
	if err != nil {
		t.Fatalf("next: %v", err)
	}
	if stdout != "" {
		t.Errorf("next printed %q with nothing outstanding, want nothing", stdout)
	}
}

// TestVersion: the release workflow tags whatever this prints.
func TestVersion(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1"}
	up.install(t)
	m := baseMatrix()
	m.Version = "0.1.5"
	repo(t, m)

	stdout, _, err := runCLI(t, "version")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	if stdout != "0.1.5\n" {
		t.Errorf("version printed %q, want \"0.1.5\\n\"", stdout)
	}
}

func TestDecide(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1"}
	up.kyaml["5.8.0"] = "v0.21.0"
	up.install(t)
	repo(t, baseMatrix())

	stdout, _, err := runCLI(t, "decide", "5.8.0")
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	for _, want := range []string{"kustomize 5.8.0 links kyaml v0.21.0", "action:   rebuild", "kustofmt: 0.1.3"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("decide output is missing %q:\n%s", want, stdout)
		}
	}
	// Deciding must not write anything: that is what apply is for.
	if m := reload(t); m.Version != "0.1.2" || len(m.Releases) != 3 {
		t.Error("decide modified the matrix")
	}
	if _, _, err := runCLI(t, "decide"); err == nil {
		t.Error("decide accepted no arguments")
	}
}

// TestRender regenerates the tables and reports honestly whether it changed
// anything, which is what makes running it twice safe.
func TestRender(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1"}
	up.install(t)
	repo(t, baseMatrix())
	write(t, ".", readmeFile, "# fixture\n\n"+compat.MarkerBegin+"\nstale\n"+compat.MarkerEnd+"\n")

	stdout, _, err := runCLI(t, "render")
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !regexp.MustCompile(`README\.md\s+updated`).MatchString(stdout) {
		t.Errorf("render did not report updating the README:\n%s", stdout)
	}
	if !regexp.MustCompile(`CHANGELOG\.md\s+already current`).MatchString(stdout) {
		t.Errorf("render did not report the changelog as current:\n%s", stdout)
	}
	if strings.Contains(read(t, readmeFile), "stale") {
		t.Error("the stale table survived")
	}

	stdout, _, err = runCLI(t, "render")
	if err != nil {
		t.Fatalf("second render: %v", err)
	}
	if strings.Contains(stdout, "updated") {
		t.Errorf("render is not idempotent:\n%s", stdout)
	}
}

func TestRunDispatch(t *testing.T) {
	up := &fakeUpstream{kyaml: knownKyaml(), goModPin: "v0.20.1"}
	up.install(t)
	repo(t, baseMatrix())

	t.Run("no arguments", func(t *testing.T) {
		_, stderr, err := runCLI(t)
		if err == nil || !strings.Contains(err.Error(), "subcommand is required") {
			t.Errorf("error = %v, want it to ask for a subcommand", err)
		}
		if !strings.Contains(stderr, "usage: compat") {
			t.Error("usage was not printed to stderr alongside the error")
		}
	})

	t.Run("unknown subcommand", func(t *testing.T) {
		_, stderr, err := runCLI(t, "publish")
		if err == nil || !strings.Contains(err.Error(), `unknown subcommand "publish"`) {
			t.Errorf("error = %v, want it to name the subcommand", err)
		}
		if !strings.Contains(stderr, "usage: compat") {
			t.Error("usage was not printed to stderr")
		}
	})

	t.Run("help goes to stdout so it can be piped", func(t *testing.T) {
		stdout, stderr, err := runCLI(t, "help")
		if err != nil {
			t.Fatalf("help: %v", err)
		}
		if !strings.Contains(stdout, "usage: compat") {
			t.Errorf("help did not print usage to stdout:\n%s", stdout)
		}
		if stderr != "" {
			t.Errorf("help wrote to stderr: %q", stderr)
		}
	})
}

// TestRepoRootRequiresAModule: the tool resolves every path from the module
// root, so it has to refuse when there is not one.
func TestRepoRootRequiresAModule(t *testing.T) {
	t.Chdir(t.TempDir())
	if _, err := repoRoot(); err == nil {
		t.Skip("the temporary directory sits inside a Go module; nothing to assert")
	} else if !strings.Contains(err.Error(), "not inside a Go module") {
		t.Errorf("error = %v, want it to say there is no module", err)
	}
}

// TestScriptLayer covers the boundary the other tests substitute. The helper
// scripts are the exactness-critical part of this tool, and when one of them
// fails the error has to name it -- a bare "exit status 1" from a scheduled
// workflow is not something anyone can act on.
func TestScriptLayer(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the helper scripts are shell scripts")
	}
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}
	fake := func(name, body string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "scripts", name), []byte("#!/bin/sh\n"+body), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	// Trailing whitespace on purpose: the real scripts echo, and callers compare
	// the result to a version string.
	fake("kyaml-for-kustomize.sh", "echo '  v0.20.1  '\n")
	fake("kustomize-releases.sh", "printf '5.6.0\\n5.7.0\\n5.7.1\\n'\n")
	fake("broken.sh", "echo 'upstream said no' >&2\nexit 3\n")
	t.Chdir(dir)

	t.Run("output is trimmed", func(t *testing.T) {
		got, err := resolveKyamlFor("5.7.1")
		if err != nil {
			t.Fatal(err)
		}
		if got != "v0.20.1" {
			t.Errorf("got %q, want v0.20.1 with no surrounding space", got)
		}
	})

	t.Run("a release list is split into versions", func(t *testing.T) {
		got, err := listPublishedReleases("5.6.0")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Join(got, ",") != "5.6.0,5.7.0,5.7.1" {
			t.Errorf("got %v, want the three versions", got)
		}
	})

	t.Run("a failing script is named in the error", func(t *testing.T) {
		_, err := script("broken.sh", "5.9.0")
		if err == nil {
			t.Fatal("a script exiting 3 was reported as success")
		}
		for _, want := range []string{"scripts/broken.sh", "5.9.0"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error does not mention %q: %v", want, err)
			}
		}
	})

	t.Run("a missing script fails rather than returning nothing", func(t *testing.T) {
		if _, err := script("absent.sh"); err == nil {
			t.Error("a script that does not exist was reported as success")
		}
	})

	// listPublishedReleases must propagate a failure rather than report an empty
	// world, which the watcher would read as "nothing to do".
	t.Run("a failure is not an empty list", func(t *testing.T) {
		if err := os.Remove(filepath.Join(dir, "scripts", "kustomize-releases.sh")); err != nil {
			t.Fatal(err)
		}
		got, err := listPublishedReleases("5.6.0")
		if err == nil {
			t.Fatalf("got %v with no error; an unreadable upstream must not look like an empty one", got)
		}
	})
}
