package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// exercise runs the CLI in-process. Keeping run() free of globals means every
// mode is testable without building a binary or spawning a subprocess.
func exercise(t *testing.T, stdin string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	var out, errBuf bytes.Buffer
	code = run(args, strings.NewReader(stdin), &out, &errBuf)
	return code, out.String(), errBuf.String()
}

const messy = "spec:\n  containers:\n    - name: app\n      resources:\n        requests: {cpu: 50m}\n"
const tidy = "spec:\n  containers:\n  - name: app\n    resources:\n      requests:\n        cpu: 50m\n"

// writeTree creates files under a temp dir and returns its path.
func writeTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestStdinFilter(t *testing.T) {
	code, stdout, stderr := exercise(t, messy)
	if code != exitOK {
		t.Errorf("exit = %d, want %d (stderr: %s)", code, exitOK, stderr)
	}
	if stdout != tidy {
		t.Errorf("stdout =\n%s\nwant:\n%s", stdout, tidy)
	}
}

func TestStdinRejectsWrite(t *testing.T) {
	code, _, stderr := exercise(t, messy, "-w")
	if code != exitError {
		t.Errorf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "cannot use -w with standard input") {
		t.Errorf("stderr = %q, want an explanation", stderr)
	}
}

func TestListExitsOneWhenFilesDiffer(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": messy, "b.yaml": tidy})
	code, stdout, _ := exercise(t, "", "-l", dir)
	if code != exitDiffers {
		t.Errorf("exit = %d, want %d", code, exitDiffers)
	}
	if !strings.Contains(stdout, "a.yaml") {
		t.Errorf("stdout = %q, want a.yaml listed", stdout)
	}
	if strings.Contains(stdout, "b.yaml") {
		t.Errorf("stdout = %q, b.yaml is already formatted and must not be listed", stdout)
	}
}

func TestListExitsZeroWhenClean(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": tidy})
	code, stdout, _ := exercise(t, "", "-l", dir)
	if code != exitOK {
		t.Errorf("exit = %d, want %d", code, exitOK)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
}

func TestWriteInPlace(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": messy})
	path := filepath.Join(dir, "a.yaml")
	code, stdout, stderr := exercise(t, "", "-w", path)
	if code != exitOK {
		t.Errorf("exit = %d, want %d (stderr: %s)", code, exitOK, stderr)
	}
	if stdout != "" {
		t.Errorf("-w must not print the file, got %q", stdout)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != tidy {
		t.Errorf("file =\n%s\nwant:\n%s", got, tidy)
	}
}

// TestWritePreservesModeAndSkipsUnchanged: rewriting a file that did not change
// would churn mtimes and wake every file watcher in the repository.
func TestWriteLeavesCleanFilesAlone(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": tidy})
	path := filepath.Join(dir, "a.yaml")
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := exercise(t, "", "-w", path); code != exitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if !before.ModTime().Equal(after.ModTime()) {
		t.Error("an already-formatted file was rewritten")
	}
	if after.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v, want 0600 preserved", after.Mode().Perm())
	}
}

func TestWritePreservesFileMode(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": messy})
	path := filepath.Join(dir, "a.yaml")
	if err := os.Chmod(path, 0o640); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := exercise(t, "", "-w", path); code != exitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o640 {
		t.Errorf("mode = %v, want 0640 preserved", info.Mode().Perm())
	}
}

// TestWriteReplacesSymlinkTargetNotTheLink: a repository that shares a manifest
// by symlinking it must still have a symlink afterwards. Renaming over the link
// would quietly turn it into a regular file and fork the two copies.
func TestWriteReplacesSymlinkTargetNotTheLink(t *testing.T) {
	dir := writeTree(t, map[string]string{"shared/values.yaml": messy})
	target := filepath.Join(dir, "shared", "values.yaml")
	link := filepath.Join(dir, "values.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if code, _, stderr := exercise(t, "", "-w", link); code != exitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("the symlink was replaced by a regular file")
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != tidy {
		t.Errorf("target =\n%s\nwant:\n%s", got, tidy)
	}
}

// TestWriteRefusesAReadOnlyFile: renaming a replacement into place needs write
// permission on the directory, not on the file, so the atomic write has to ask
// about the file explicitly. Without that, chmod -w stops protecting a manifest
// the moment the write strategy changes.
func TestWriteRefusesAReadOnlyFile(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": messy})
	path := filepath.Join(dir, "a.yaml")
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}

	code, _, stderr := exercise(t, "", "-w", path)
	if code != exitError {
		t.Errorf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "permission denied") {
		t.Errorf("stderr = %q, want a permission error", stderr)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != messy {
		t.Error("a read-only file was rewritten")
	}
}

// TestWriteLeavesNoTemporaryFiles: the write goes via a temporary alongside the
// target, and a formatter that litters a repository with them is worse than one
// that writes in place.
func TestWriteLeavesNoTemporaryFiles(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": messy, "b.yaml": messy})
	if code, _, stderr := exercise(t, "", "-w", dir); code != exitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 2 {
		t.Errorf("directory contains %v, want exactly the two YAML files", names)
	}
}

func TestDiffMode(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": messy})
	path := filepath.Join(dir, "a.yaml")
	code, stdout, _ := exercise(t, "", "-d", path)
	if code != exitOK {
		t.Errorf("exit = %d, want %d", code, exitOK)
	}
	for _, want := range []string{"--- " + path + ".orig", "+++ " + path, "@@", "-        requests: {cpu: 50m}"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("diff missing %q:\n%s", want, stdout)
		}
	}
	// -d must not modify anything.
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != messy {
		t.Error("-d modified the file")
	}
}

func TestSOPSSkippedByDefault(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "format", "testdata", "sops-secret.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := writeTree(t, map[string]string{"secret.yaml": string(src)})
	path := filepath.Join(dir, "secret.yaml")

	code, stdout, stderr := exercise(t, "", "-w", path)
	if code != exitOK {
		t.Errorf("exit = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stderr, "sops-encrypted, skipped") {
		t.Errorf("expected a skip notice, stderr = %q", stderr)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, src) {
		t.Fatal("a sops-encrypted file was rewritten; its MAC is now invalid")
	}
}

func TestSOPSIncludedOnRequest(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "format", "testdata", "sops-secret.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	dir := writeTree(t, map[string]string{"secret.yaml": string(src)})
	path := filepath.Join(dir, "secret.yaml")

	code, _, stderr := exercise(t, "", "-w", "--include-sops", path)
	if code != exitOK {
		t.Errorf("exit = %d, want %d (stderr: %s)", code, exitOK, stderr)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(got, src) {
		t.Error("--include-sops did not format the file")
	}
}

// TestSOPSOnStdinRespectsTheOutputChannel: -l promises a list of filenames and
// -d promises a diff. Echoing the skipped document down either of them puts a
// file body into a channel a caller is parsing. Filter mode is the one place
// stdout *is* the file, so that is the one place the passthrough belongs.
func TestSOPSOnStdinRespectsTheOutputChannel(t *testing.T) {
	src, err := os.ReadFile(filepath.Join("..", "..", "format", "testdata", "sops-secret.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	for _, mode := range []string{"-l", "-d"} {
		t.Run(mode, func(t *testing.T) {
			code, stdout, stderr := exercise(t, string(src), mode)
			if code != exitOK {
				t.Errorf("exit = %d, want %d", code, exitOK)
			}
			if stdout != "" {
				t.Errorf("%s emitted %d bytes of the document on stdout", mode, len(stdout))
			}
			if !strings.Contains(stderr, "sops-encrypted") {
				t.Errorf("stderr = %q, want a skip notice", stderr)
			}
		})
	}

	t.Run("filter mode still passes it through", func(t *testing.T) {
		code, stdout, _ := exercise(t, string(src))
		if code != exitOK {
			t.Errorf("exit = %d, want %d", code, exitOK)
		}
		if stdout != string(src) {
			t.Error("filter mode must emit the document unchanged")
		}
	})
}

func TestMultipleFilesRequireAMode(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": messy, "b.yaml": messy})
	code, stdout, stderr := exercise(t, "", dir)
	if code != exitError {
		t.Errorf("exit = %d, want %d", code, exitError)
	}
	if stdout != "" {
		t.Errorf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(stderr, "specify -l, -d or -w") {
		t.Errorf("stderr = %q, want guidance", stderr)
	}
}

func TestSingleFilePrintsToStdout(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": messy})
	code, stdout, _ := exercise(t, "", filepath.Join(dir, "a.yaml"))
	if code != exitOK {
		t.Errorf("exit = %d, want %d", code, exitOK)
	}
	if stdout != tidy {
		t.Errorf("stdout =\n%s\nwant:\n%s", stdout, tidy)
	}
}

// TestParseErrorDoesNotStopTheWalk: one malformed file in a large repository
// must not hide the formatting state of every file after it.
func TestParseErrorDoesNotStopTheWalk(t *testing.T) {
	dir := writeTree(t, map[string]string{
		"a-broken.yaml": "a: [1, 2\nb: {\n",
		"b-messy.yaml":  messy,
	})
	code, stdout, stderr := exercise(t, "", "-l", dir)
	if code != exitError {
		t.Errorf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "a-broken.yaml") {
		t.Errorf("stderr should name the bad file, got %q", stderr)
	}
	if !strings.Contains(stdout, "b-messy.yaml") {
		t.Errorf("the walk stopped early; stdout = %q", stdout)
	}
}

func TestSkipsGitAndVendorDirectories(t *testing.T) {
	dir := writeTree(t, map[string]string{
		".git/config.yaml":         messy,
		"vendor/dep/thing.yaml":    messy,
		"node_modules/pkg/an.yaml": messy,
		"real.yaml":                messy,
	})
	_, stdout, _ := exercise(t, "", "-l", dir)
	for _, unwanted := range []string{".git", "vendor", "node_modules"} {
		if strings.Contains(stdout, unwanted) {
			t.Errorf("%s should not be walked, stdout = %q", unwanted, stdout)
		}
	}
	if !strings.Contains(stdout, "real.yaml") {
		t.Errorf("real.yaml missing from %q", stdout)
	}
}

// TestSymlinkedDirectoryIsWalked: a check mode that examines nothing and exits
// 0 is indistinguishable from a check mode that passed. os.Stat follows the
// link and calls it a directory; filepath.WalkDir Lstats its root and calls it
// a file, so the walk found nothing and said so silently.
func TestSymlinkedDirectoryIsWalked(t *testing.T) {
	dir := writeTree(t, map[string]string{"real/a.yaml": messy})
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "real"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	code, stdout, stderr := exercise(t, "", "-l", link)
	if code != exitDiffers {
		t.Errorf("exit = %d, want %d (stderr: %s)", code, exitDiffers, stderr)
	}
	// Reported under the name the user typed, not the far side of the link.
	want := filepath.Join(link, "a.yaml")
	if !strings.Contains(stdout, want) {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestSymlinkedDirectoryIsWritableThroughItsLink: the rewritten path has to
// still address a real file, or -w reports success and writes nothing.
func TestSymlinkedDirectoryIsWritableThroughItsLink(t *testing.T) {
	dir := writeTree(t, map[string]string{"real/a.yaml": messy})
	link := filepath.Join(dir, "link")
	if err := os.Symlink(filepath.Join(dir, "real"), link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if code, _, stderr := exercise(t, "", "-w", link); code != exitOK {
		t.Fatalf("exit = %d (stderr: %s)", code, stderr)
	}
	got, err := os.ReadFile(filepath.Join(dir, "real", "a.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != tidy {
		t.Errorf("file =\n%s\nwant:\n%s", got, tidy)
	}
}

func TestNamedFileIsFormattedWhateverItsExtension(t *testing.T) {
	dir := writeTree(t, map[string]string{"Chart.lock": messy})
	code, stdout, _ := exercise(t, "", filepath.Join(dir, "Chart.lock"))
	if code != exitOK {
		t.Errorf("exit = %d, want %d", code, exitOK)
	}
	if stdout != tidy {
		t.Error("naming a file explicitly should format it regardless of extension")
	}
}

func TestBothYamlExtensionsAreWalked(t *testing.T) {
	dir := writeTree(t, map[string]string{"a.yaml": messy, "b.yml": messy, "c.txt": messy})
	_, stdout, _ := exercise(t, "", "-l", dir)
	for _, want := range []string{"a.yaml", "b.yml"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("%s not walked, stdout = %q", want, stdout)
		}
	}
	if strings.Contains(stdout, "c.txt") {
		t.Errorf("c.txt is not YAML, stdout = %q", stdout)
	}
}

func TestMissingPathIsAnError(t *testing.T) {
	code, _, stderr := exercise(t, "", "-l", filepath.Join(t.TempDir(), "nope"))
	if code != exitError {
		t.Errorf("exit = %d, want %d", code, exitError)
	}
	if stderr == "" {
		t.Error("expected an error message")
	}
}

func TestVersion(t *testing.T) {
	code, stdout, _ := exercise(t, "", "-version")
	if code != exitOK {
		t.Errorf("exit = %d, want %d", code, exitOK)
	}
	if !strings.HasPrefix(stdout, "kustofmt ") {
		t.Errorf("stdout = %q", stdout)
	}
	// The style contract is kyaml's emission, so the binary must say which
	// kyaml it was built against.
	if !strings.Contains(stdout, "kyaml ") {
		t.Errorf("version output should report the kyaml version, got %q", stdout)
	}
}

func TestUnknownFlagFails(t *testing.T) {
	code, _, stderr := exercise(t, "", "-nope")
	if code != exitError {
		t.Errorf("exit = %d, want %d", code, exitError)
	}
	if stderr == "" {
		t.Error("expected usage output on stderr")
	}
}

// TestSemanticsRefusalLeavesFileAlone: when the formatter cannot vouch for its
// own output it must not write. The message has to say what to do about it,
// because "would change the document's meaning" alone is not actionable.
func TestSemanticsRefusalLeavesFileAlone(t *testing.T) {
	const folded = "key: >\n  one\n   two\n"
	dir := writeTree(t, map[string]string{"a.yaml": folded})
	path := filepath.Join(dir, "a.yaml")

	code, _, stderr := exercise(t, "", "-w", path)
	if code != exitError {
		t.Errorf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, "left unchanged") || !strings.Contains(stderr, "literal (|)") {
		t.Errorf("stderr should explain the fix, got: %s", stderr)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != folded {
		t.Error("the file was rewritten despite the refusal")
	}
}

// TestParseErrorNamesFileAndPosition: the filename is the part a user needs to
// act on, and it must always be present. A position is included too, straight
// from the parser -- see the note in processFile about its accuracy.
func TestParseErrorNamesFileAndPosition(t *testing.T) {
	dir := writeTree(t, map[string]string{"broken.yaml": "a: 1\n  b: 2\n"})
	path := filepath.Join(dir, "broken.yaml")
	code, _, stderr := exercise(t, "", "-l", path)
	if code != exitError {
		t.Errorf("exit = %d, want %d", code, exitError)
	}
	if !strings.Contains(stderr, path) {
		t.Errorf("stderr must name the file, got: %s", stderr)
	}
	if !strings.Contains(stderr, "line ") {
		t.Errorf("stderr should carry a position, got: %s", stderr)
	}
}
