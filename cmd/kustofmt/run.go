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
	fs.Usage = func() { warnf(stderr, "%s", usage) }
	fs.BoolVar(&opts.list, "l", false, "list files whose formatting differs")
	fs.BoolVar(&opts.write, "w", false, "write result to (source) file instead of stdout")
	fs.BoolVar(&opts.diff, "d", false, "display diffs instead of rewriting files")
	fs.BoolVar(&opts.includeSOPS, "include-sops", false, "format sops-encrypted files")
	fs.BoolVar(&showVersion, "version", false, "print version information")
	if err := fs.Parse(args); err != nil {
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
		if _, err := stdout.Write(src); err != nil {
			return exitError
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
func writeFile(path string, src, out []byte) error {
	if bytesEqual(src, out) {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, info.Mode().Perm())
}

func bytesEqual(a, b []byte) bool { return string(a) == string(b) }

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
		err = filepath.WalkDir(p, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDir(d.Name()) && path != p {
					return filepath.SkipDir
				}
				return nil
			}
			if isYAML(path) {
				add(path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return out, nil
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
