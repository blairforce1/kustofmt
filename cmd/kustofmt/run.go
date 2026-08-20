package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/blairforce1/kustofmt/format"
)

// Exit codes. These are the contract for hooks and CI, so they are named.
const (
	exitOK      = 0 // nothing to do, or the requested work succeeded
	exitDiffers = 1 // -l found files that need formatting
	exitError   = 2 // an operational failure: unreadable file, parse error
)

// version is set at build time with -ldflags "-X main.version=...".
// Left empty for `go install` builds, which fall back to the module's
// VCS stamp instead of reporting a misleading "dev".
var version string

const stdinName = "<standard input>"

const usage = `kustofmt formats YAML in kustomize's emitted style.

Usage:
	kustofmt [flags] [path ...]

With no paths, kustofmt reads standard input and writes to standard output.
Directories are walked recursively for *.yaml and *.yml.

Flags:
	-l              list files whose formatting differs; exit 1 if any
	-w              write the result back to the file
	-d              print unified diffs instead of the formatted file
	--include-sops  format sops-encrypted files instead of skipping them
	-version        print version information
	-h              print this message

Files encrypted with sops are skipped by default: sops computes a MAC over
the document structure, so reformatting one makes it undecryptable.

Exit codes: 0 success, 1 files need formatting (-l), 2 operational error.
`

type options struct {
	list        bool
	write       bool
	diff        bool
	includeSOPS bool
}

// mode reports whether any flag was given that makes formatting many files at
// once meaningful. Without one, printing several formatted files to stdout
// would concatenate them into nonsense.
func (o options) mode() bool { return o.list || o.write || o.diff }

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var (
		opts        options
		showVersion bool
	)
	fs := flag.NewFlagSet("kustofmt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	// flag calls Usage for -h and for a malformed flag alike. Those are
	// different events: one is a request that succeeded and belongs on stdout
	// where it can be piped or paged, the other is an error. Suppress the
	// built-in call and print it below, where the two can be told apart.
	fs.Usage = func() {}
	fs.BoolVar(&opts.list, "l", false, "list files whose formatting differs")
	fs.BoolVar(&opts.write, "w", false, "write result to (source) file instead of stdout")
	fs.BoolVar(&opts.diff, "d", false, "display diffs instead of rewriting files")
	fs.BoolVar(&opts.includeSOPS, "include-sops", false, "format sops-encrypted files")
	fs.BoolVar(&showVersion, "version", false, "print version information")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			printUsage(stdout)
			return exitOK
		}
		// flag has already named the offending argument on stderr.
		printUsage(stderr)
		return exitError
	}
	if showVersion {
		if _, err := fmt.Fprintln(stdout, versionString()); err != nil {
			return exitError
		}
		return exitOK
	}

	paths := fs.Args()
	if len(paths) == 0 {
		return runStdin(opts, stdin, stdout, stderr)
	}

	files, err := collect(paths)
	if err != nil {
		warnf(stderr, "kustofmt: %v\n", err)
		return exitError
	}
	if !opts.mode() && len(files) > 1 {
		warnf(stderr, "kustofmt: %d files match; specify -l, -d or -w\n", len(files))
		return exitError
	}

	code := exitOK
	for _, path := range files {
		switch processFile(path, opts, stdout, stderr) {
		case exitError:
			code = exitError
		case exitDiffers:
			if code == exitOK {
				code = exitDiffers
			}
		}
	}
	return code
}

// runStdin implements filter mode. -w has no file to write back to, so it is
// rejected rather than silently ignored.
func runStdin(opts options, stdin io.Reader, stdout, stderr io.Writer) int {
	if opts.write {
		warnf(stderr, "%s\n", "kustofmt: cannot use -w with standard input")
		return exitError
	}
	src, err := io.ReadAll(stdin)
	if err != nil {
		warnf(stderr, "kustofmt: reading standard input: %v\n", err)
		return exitError
	}
	if !opts.includeSOPS && format.IsSOPS(src) {
		warnf(stderr, "%s\n", "kustofmt: standard input looks sops-encrypted; skipped (use --include-sops to override)")
		// Filter mode has to emit the document: stdout *is* the output file, and
		// swallowing the input would truncate whatever the pipe feeds next. In
		// -l and -d mode stdout is a machine-readable channel -- a list of
		// names, or a diff -- and a file body has no place in it.
		if !opts.mode() {
			if _, err := stdout.Write(src); err != nil {
				return exitError
			}
		}
		return exitOK
	}
	out, err := format.Format(src)
	if err != nil {
		warnf(stderr, "kustofmt: %s: %v\n", stdinName, describe(err))
		return exitError
	}
	return reportResult(stdinName, opts, src, out, stdout, stderr)
}

func processFile(path string, opts options, stdout, stderr io.Writer) int {
	src, err := os.ReadFile(path)
	if err != nil {
		warnf(stderr, "kustofmt: %v\n", err)
		return exitError
	}
	if !opts.includeSOPS && format.IsSOPS(src) {
		warnf(stderr, "kustofmt: %s: sops-encrypted, skipped (use --include-sops to override)\n", path)
		return exitOK
	}
	out, err := format.Format(src)
	if err != nil {
		// A parse failure names the file and does not stop the walk: one bad
		// file in a large repository should not hide the state of the rest.
		//
		// The position comes from the YAML parser verbatim. It is accurate for
		// most error classes but reports one line early for unterminated flow
		// collections, and that offset is not consistent enough to correct
		// blindly -- adjusting it would break the classes that are already
		// right. Passing the parser's own words through is the honest option.
		warnf(stderr, "kustofmt: %s: %v\n", path, describe(err))
		return exitError
	}
	if opts.write {
		if err := writeFile(path, src, out); err != nil {
			warnf(stderr, "kustofmt: %v\n", err)
			return exitError
		}
	}
	return reportResult(path, opts, src, out, stdout, stderr)
}

// reportResult emits whatever the selected modes ask for and reports whether
// the file differed.
func reportResult(name string, opts options, src, out []byte, stdout, stderr io.Writer) int {
	changed := !bytesEqual(src, out)
	if opts.list && changed {
		if _, err := fmt.Fprintln(stdout, name); err != nil {
			warnf(stderr, "kustofmt: %v\n", err)
			return exitError
		}
	}
	if opts.diff && changed {
		if _, err := io.WriteString(stdout, format.Diff(name+".orig", name, src, out)); err != nil {
			warnf(stderr, "kustofmt: %v\n", err)
			return exitError
		}
	}
	if !opts.mode() {
		if _, err := stdout.Write(out); err != nil {
			warnf(stderr, "kustofmt: %v\n", err)
			return exitError
		}
	}
	if changed && opts.list {
		return exitDiffers
	}
	return exitOK
}

// writeFile replaces path only when the content actually changed, so a
// formatting run over a clean repository leaves every mtime alone and does not
// churn build caches or file watchers.
//
// The replacement is written to a temporary file alongside the target and
// renamed over it. Writing in place would mean opening the real file with
// O_TRUNC, which leaves a window where an interrupted run -- a Ctrl-C partway
// through a repository, a cancelled CI job, a full disk -- has emptied a
// manifest and not yet written it back. rename(2) is atomic within a
// filesystem, so a concurrent reader sees the old file or the new one and
// never half of either.
//
// The cost is that the file gets a new inode, which breaks any hard link to
// it. Hard-linked YAML is rare; a truncated manifest in a GitOps repository is
// expensive. Symlinks are followed rather than replaced, below, because those
// are not rare at all.
func writeFile(path string, src, out []byte) error {
	if bytesEqual(src, out) {
		return nil
	}
	// Replace what a link points at, not the link itself: repositories that
	// share a manifest by symlinking it expect the link to survive a format
	// run, which is what writing through the file descriptor used to do.
	target, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		return err
	}
	// Renaming over a file needs write permission on its *directory*, not on
	// the file, so the atomic path would happily replace a read-only manifest
	// that a plain write refused. Ask the question the old in-place write asked
	// implicitly, and refuse for the same reason it did.
	probe, err := os.OpenFile(target, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	if err := probe.Close(); err != nil {
		return err
	}

	// The temporary file has to share a directory with its target: rename is
	// only atomic within a filesystem, and the system temp directory is
	// routinely a different one. The leading dot keeps it out of the way of
	// anything watching the directory for manifests.
	tmp, err := os.CreateTemp(filepath.Dir(target), "."+filepath.Base(target)+".kustofmt-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		// Both are no-ops on the success path, where the file is already
		// closed and no longer at this name.
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}()

	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
		return err
	}
	if _, err := tmp.Write(out); err != nil {
		return err
	}
	// Sync before the rename. The rename is atomic to other processes whatever
	// we do, but a machine that loses power holding the metadata change and
	// not the data would resurrect the file as a well-named empty one.
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, target)
}

func bytesEqual(a, b []byte) bool { return string(a) == string(b) }

// printUsage writes the manual. Which stream it goes to is the caller's call:
// asked-for help is output, unsolicited help is a diagnostic.
func printUsage(w io.Writer) {
	_, _ = io.WriteString(w, usage)
}

// warnf writes a diagnostic to stderr. Failing to report a failure is not
// itself recoverable, so the write error is deliberately discarded.
func warnf(w io.Writer, format string, args ...any) {
	_, _ = fmt.Fprintf(w, format, args...)
}

// describe adds context to the errors a user is most likely to hit, so the
// message says what to do rather than only what went wrong.
func describe(err error) error {
	if errors.Is(err, format.ErrSemanticsChanged) {
		return fmt.Errorf("%w; left unchanged. This is usually a folded (>) scalar "+
			"containing an indented line, which the YAML emitter cannot reproduce "+
			"exactly. Rewriting it as a literal (|) scalar avoids the ambiguity", err)
	}
	if errors.Is(err, format.ErrNotVerifiable) {
		// Same underlying defect as above in almost every case, but the
		// semantics check could not run to name it, so the user gets an
		// unhelpful "did not converge" unless we explain what happened.
		return fmt.Errorf("%w; left unchanged. The check that compares meaning "+
			"before and after needs a document that decodes to plain values, and "+
			"this one does not -- usually duplicate keys. Fixing that lets kustofmt "+
			"diagnose the file properly; a folded (>) scalar containing an indented "+
			"line is the likeliest cause underneath", err)
	}
	return err
}

// collect expands the given paths into a sorted list of YAML files.
// Directories are walked; explicitly named files are taken as given, whatever
// their extension, because naming a file is an explicit instruction.
func collect(paths []string) ([]string, error) {
	var out []string
	seen := make(map[string]bool)
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			add(p)
			continue
		}
		// os.Stat follows symlinks, so a symlinked directory reaches here --
		// but filepath.WalkDir Lstats its root, sees a link rather than a
		// directory, and walks nothing at all. Left alone that makes
		// `kustofmt -l envs` report success having checked no files, which is
		// the one thing a check mode must never do. Resolve the root here;
		// links found *inside* the tree are still not followed.
		root, err := filepath.EvalSymlinks(p)
		if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDir(d.Name()) && path != root {
					return filepath.SkipDir
				}
				return nil
			}
			if isYAML(path) {
				add(underRoot(p, root, path))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}

// underRoot reports a walked path under the name the caller actually typed.
// Walking a symlinked directory means resolving it first, and someone who ran
// `kustofmt -l envs` wants to read "envs/prod/app.yaml", not the path on the
// other side of the link.
func underRoot(given, root, path string) string {
	if given == root {
		return path
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.Join(given, rel)
}

// skipDir avoids directories whose contents are never hand-maintained YAML.
func skipDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules":
		return true
	}
	return false
}

func isYAML(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yaml" || ext == ".yml"
}

// versionString prefers the linker-injected version and falls back to Go's own
// build stamp, so `go install` produces something meaningful rather than "dev".
func versionString() string {
	v := version
	if v == "" {
		v = "(devel)"
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "kustofmt " + v
	}
	if version == "" && info.Main.Version != "" {
		v = info.Main.Version
	}
	var revision, modified string
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			revision = s.Value
		case "vcs.modified":
			if s.Value == "true" {
				modified = " (dirty)"
			}
		}
	}
	out := "kustofmt " + v
	if revision != "" {
		if len(revision) > 12 {
			revision = revision[:12]
		}
		out += " " + revision + modified
	}
	return out + "\nkyaml " + kyamlVersion(info)
}

// kyamlVersion reports which kyaml the binary was built against. The style
// contract is kyaml's emission, so this is the version that defines it.
func kyamlVersion(info *debug.BuildInfo) string {
	const mod = "sigs.k8s.io/kustomize/kyaml"
	for _, d := range info.Deps {
		if d.Path == mod {
			if d.Replace != nil {
				return d.Replace.Version
			}
			return d.Version
		}
	}
	return "unknown"
}
