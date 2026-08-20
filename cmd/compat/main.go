// Command compat maintains kustofmt's kustomize compatibility matrix.
//
// It is a maintenance tool, not part of a release: goreleaser builds only
// ./cmd/kustofmt. Run it with `go run ./cmd/compat` or through the Makefile.
//
//	compat status              what upstream has published that we have not handled
//	compat decide <kustomize>  resolve that release's kyaml and say what it requires
//	compat apply  <kustomize>  do it: extend a row, or rebuild and cut a version
//	compat render              regenerate the tables in README.md and CHANGELOG.md
//	compat check               the CI gate: re-derive every row from upstream
//
// The kyaml for a kustomize release is resolved by scripts/kyaml-for-kustomize.sh
// rather than reimplemented here, so the exactness-critical logic exists once.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/blairforce1/kustofmt/internal/compat"
)

const (
	matrixFile    = "compatibility.yaml"
	readmeFile    = "README.md"
	changelogFile = "CHANGELOG.md"
	kyamlModule   = "sigs.k8s.io/kustomize/kyaml"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "compat: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		usage()
		return errors.New("a subcommand is required")
	}
	root, err := repoRoot()
	if err != nil {
		return err
	}
	if err := os.Chdir(root); err != nil {
		return err
	}

	switch args[0] {
	case "status":
		return cmdStatus()
	case "next":
		return cmdNext()
	case "version":
		return cmdVersion()
	case "decide":
		return cmdDecide(args[1:])
	case "apply":
		return cmdApply(args[1:])
	case "render":
		return cmdRender()
	case "check":
		return cmdCheck(args[1:])
	case "-h", "--help", "help":
		usage()
		return nil
	default:
		usage()
		return fmt.Errorf("unknown subcommand %q", args[0])
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `usage: compat <command> [args]

  status              list kustomize releases at or above the floor that are
                      not yet recorded
  next                print the oldest unrecorded kustomize release, or nothing.
                      Machine-readable, for the watcher workflow
  version             print the kustofmt version the matrix head describes
  decide <kustomize>  resolve that release's kyaml and report what it requires
  apply  <kustomize>  apply it: extend an existing row, or rebuild against a new
                      kyaml and cut the next version
  render              regenerate the tables in README.md and CHANGELOG.md
  check               verify the matrix against go.mod and against upstream

Flags for apply:
  --allow-style-change  accept a golden-corpus change, taking a minor version
                        instead of a patch. Review the golden diff first.
`)
}

// repoRoot walks up to the directory holding go.mod, so the tool works from
// anywhere in the tree.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("not inside a Go module")
		}
		dir = parent
	}
}

// netTimeout bounds every call that reaches the network. Resolving a kyaml
// version pulls from the module proxy and from GitHub; a hung fetch should fail
// the run rather than stall a scheduled workflow indefinitely.
const netTimeout = 5 * time.Minute

// script runs one of the repository's helper scripts and returns its stdout.
func script(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), netTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, filepath.Join("scripts", name), args...)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("scripts/%s %s: %w", name, strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// kyamlFor resolves which kyaml a kustomize release links.
func kyamlFor(kustomizeVersion string) (string, error) {
	return script("kyaml-for-kustomize.sh", kustomizeVersion)
}

// publishedReleases lists kustomize CLI releases at or above the floor.
func publishedReleases(floor string) ([]string, error) {
	out, err := script("kustomize-releases.sh", floor)
	if err != nil {
		return nil, err
	}
	return strings.Fields(out), nil
}

// goModKyaml reports the kyaml this module currently builds against.
func goModKyaml() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), netTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "list", "-m", "-f", "{{.Version}}", kyamlModule)
	cmd.Stderr = os.Stderr
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("reading %s from go.mod: %w", kyamlModule, err)
	}
	return strings.TrimSpace(string(out)), nil
}

func cmdStatus() error {
	m, err := compat.Load(matrixFile)
	if err != nil {
		return err
	}
	pinned, err := goModKyaml()
	if err != nil {
		return err
	}
	cur := m.Current()

	fmt.Printf("matrix head:  kustofmt %s  kyaml %s\n", cur.Kustofmt, cur.Kyaml)
	fmt.Printf("go.mod pin:   kyaml %s%s\n", pinned, note(pinned == cur.Kyaml, "", "  <-- DISAGREES with the matrix head"))
	fmt.Printf("floor:        kustomize %s\n\n", m.Floor)

	published, err := publishedReleases(m.Floor)
	if err != nil {
		return err
	}
	var pending []string
	for _, k := range published {
		if !m.HasKustomize(k) {
			pending = append(pending, k)
		}
	}
	if len(pending) == 0 {
		fmt.Printf("up to date: all %d published kustomize releases at or above the floor are recorded\n", len(published))
		return nil
	}
	// Decide against a matrix that accumulates as we go: applying these in
	// order is what actually happens, so the forecast has to model that.
	fmt.Printf("unhandled kustomize releases (%d), in the order they would be applied:\n", len(pending))
	for _, k := range pending {
		ky, err := kyamlFor(k)
		if err != nil {
			return err
		}
		d := m.Decide(k, ky)
		fmt.Printf("  %-8s kyaml %-9s %-12s -> kustofmt %s\n", k, ky, d.Action, d.Target)
		m.Record(d)
	}
	return nil
}

func note(cond bool, yes, no string) string {
	if cond {
		return yes
	}
	return no
}

// cmdNext prints the oldest unrecorded kustomize release and nothing else, so
// the watcher can consume it without parsing human-facing output. Empty output
// with a zero exit means there is nothing to do.
func cmdNext() error {
	m, err := compat.Load(matrixFile)
	if err != nil {
		return err
	}
	published, err := publishedReleases(m.Floor)
	if err != nil {
		return err
	}
	for _, k := range published {
		if !m.HasKustomize(k) {
			fmt.Println(k)
			return nil
		}
	}
	return nil
}

// cmdVersion prints the version at the matrix head -- the release the current
// tree would publish. The release workflow uses it to decide what to tag.
func cmdVersion() error {
	m, err := compat.Load(matrixFile)
	if err != nil {
		return err
	}
	fmt.Println(m.Current().Kustofmt)
	return nil
}

func cmdDecide(args []string) error {
	if len(args) != 1 {
		return errors.New("decide needs exactly one kustomize version")
	}
	m, err := compat.Load(matrixFile)
	if err != nil {
		return err
	}
	ky, err := kyamlFor(args[0])
	if err != nil {
		return err
	}
	d := m.Decide(strings.TrimPrefix(args[0], "v"), ky)
	fmt.Printf("kustomize %s links kyaml %s\n", d.Kustomize, d.Kyaml)
	fmt.Printf("action:   %s\n", d.Action)
	fmt.Printf("kustofmt: %s\n", d.Target)
	return nil
}

func cmdRender() error {
	m, err := compat.Load(matrixFile)
	if err != nil {
		return err
	}
	for _, f := range []string{readmeFile, changelogFile} {
		changed, err := compat.UpdateDoc(f, m)
		if err != nil {
			return err
		}
		fmt.Printf("%-14s %s\n", f, note(changed, "updated", "already current"))
	}
	return compat.Save(matrixFile, m)
}

// cmdCheck verifies the matrix. Two classes of check live here, and the
// difference between them matters.
//
// Correctness -- go.mod agrees with the matrix head, the generated tables are
// current, and every recorded row still matches upstream -- holds at any commit,
// so release builds run it.
//
// Completeness -- no published kustomize release is missing -- only holds on
// main. A tag cut months ago cannot know about a kustomize released since, and
// demanding it would make every historical tag fail its own gate the moment
// upstream ships. It sits behind --complete, which CI passes on main.
func cmdCheck(args []string) error {
	complete := false
	for _, a := range args {
		if a != "--complete" {
			return fmt.Errorf("unknown flag %q", a)
		}
		complete = true
	}

	m, err := compat.Load(matrixFile)
	if err != nil {
		return err
	}
	var problems []string

	pinned, err := goModKyaml()
	if err != nil {
		return err
	}
	if cur := m.Current(); pinned != cur.Kyaml {
		problems = append(problems, fmt.Sprintf(
			"go.mod links kyaml %s but the newest matrix row (kustofmt %s) says %s",
			pinned, cur.Kustofmt, cur.Kyaml))
	}

	for _, f := range []string{readmeFile, changelogFile} {
		ok, err := compat.CheckDoc(f, m)
		if err != nil {
			return err
		}
		if !ok {
			problems = append(problems, f+" is stale; run `make compat-render`")
		}
	}

	// The check that makes this a gate rather than a formatter: every claim in
	// the file is re-derived from upstream instead of being trusted.
	recorded := 0
	for _, r := range m.Releases {
		for _, k := range r.Kustomize {
			recorded++
			got, err := kyamlFor(k)
			if err != nil {
				return err
			}
			if got != r.Kyaml {
				problems = append(problems, fmt.Sprintf(
					"matrix claims kustomize %s ships kyaml %s, upstream says %s", k, r.Kyaml, got))
			}
		}
	}

	if !complete {
		if len(problems) > 0 {
			return report(problems)
		}
		fmt.Printf("compat: matrix agrees with go.mod, the docs and upstream (%d releases, %d kustomize versions)\n",
			len(m.Releases), recorded)
		return nil
	}

	published, err := publishedReleases(m.Floor)
	if err != nil {
		return err
	}
	for _, k := range published {
		if !m.HasKustomize(k) {
			problems = append(problems, fmt.Sprintf(
				"kustomize %s is published but not recorded; run `go run ./cmd/compat apply %s`", k, k))
		}
	}
	if len(problems) > 0 {
		return report(problems)
	}
	fmt.Printf("compat: matrix agrees with go.mod, the docs and upstream, and records all %d published kustomize releases\n",
		len(published))
	return nil
}

func report(problems []string) error {
	for _, p := range problems {
		fmt.Fprintf(os.Stderr, "  FAIL  %s\n", p)
	}
	return fmt.Errorf("%d compatibility problem(s)", len(problems))
}
