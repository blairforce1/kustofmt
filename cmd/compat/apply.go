package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/blairforce1/kustofmt/internal/compat"
)

func cmdApply(args []string) error {
	allowStyleChange := false
	var version string
	for _, a := range args {
		switch {
		case a == "--allow-style-change":
			allowStyleChange = true
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("unknown flag %q", a)
		default:
			if version != "" {
				return errors.New("apply takes exactly one kustomize version")
			}
			version = a
		}
	}
	if version == "" {
		return errors.New("apply needs a kustomize version")
	}
	version = strings.TrimPrefix(version, "v")

	m, err := compat.Load(matrixFile)
	if err != nil {
		return err
	}
	kyamlVersion, err := kyamlFor(version)
	if err != nil {
		return err
	}
	d := m.Decide(version, kyamlVersion)
	fmt.Printf("kustomize %s links kyaml %s -> %s\n", d.Kustomize, d.Kyaml, d.Action)

	switch d.Action {
	case compat.ActionNone:
		fmt.Printf("nothing to do: already recorded against kustofmt %s\n", d.Target)
		return nil
	case compat.ActionMatrixOnly:
		return applyMatrixOnly(m, d)
	case compat.ActionRebuild:
		return applyRebuild(m, d, allowStyleChange)
	}
	return fmt.Errorf("unhandled action %q", d.Action)
}

// applyMatrixOnly records a kustomize release that ships a kyaml some existing
// kustofmt release already links. Nothing is rebuilt and no version is cut: the
// binary that already exists is, provably, the right one to pin.
func applyMatrixOnly(m *compat.Matrix, d compat.Decision) error {
	if _, ok := m.ByKyaml(d.Kyaml); !ok {
		return fmt.Errorf("no release links kyaml %s", d.Kyaml)
	}
	m.Record(d)
	if err := compat.Save(matrixFile, m); err != nil {
		return err
	}
	if err := renderDocs(m); err != nil {
		return err
	}
	fmt.Printf("recorded: kustofmt %s now also covers kustomize %s (no rebuild, no release)\n",
		d.Target, d.Kustomize)
	return nil
}

// applyRebuild points go.mod at the new kyaml, then lets the golden corpus
// decide what kind of release this is.
func applyRebuild(m *compat.Matrix, d compat.Decision, allowStyleChange bool) error {
	fmt.Printf("bumping go.mod to %s %s\n", kyamlModule, d.Kyaml)
	if err := goCmd("mod", "edit", "-require="+kyamlModule+"@"+d.Kyaml); err != nil {
		return err
	}
	if err := goCmd("mod", "tidy"); err != nil {
		return err
	}

	// The corpus is the arbiter. If it still passes, nothing a user can observe
	// has changed, and the output style is this tool's public API -- so this is
	// a patch. If it fails, the emitted style moved, and that is not something
	// to decide automatically.
	styleChanged := goCmd("test", "./format/", "-run", "TestGolden|TestZeroDiff") != nil

	version := d.Target
	switch {
	case !styleChanged:
		fmt.Printf("golden corpus unchanged: no observable change, taking a patch version\n")
	case !allowStyleChange:
		return fmt.Errorf(`the golden corpus changed: kyaml %s emits a different style.

This is a breaking change to kustofmt's output, not a routine bump. Review it:

    go test ./format/ -run TestGolden       # see the diff
    make golden                             # accept it, then read the diff again
    go run ./cmd/compat apply %s --allow-style-change

go.mod has been left pointing at %s so you can inspect the difference`,
			d.Kyaml, d.Kustomize, d.Kyaml)
	default:
		version = compat.NextMinor(m.Current().Kustofmt)
		fmt.Printf("golden corpus changed and the change was accepted: taking a minor version\n")
		if err := goCmd("test", "./format/", "-update"); err != nil {
			return err
		}
	}

	d.Target = version // the corpus may have promoted this to a minor
	m.Record(d)
	if err := compat.Save(matrixFile, m); err != nil {
		return err
	}
	if err := renderDocs(m); err != nil {
		return err
	}
	if err := addChangelogEntry(version, d, styleChanged); err != nil {
		return err
	}
	fmt.Printf("prepared kustofmt %s against kyaml %s for kustomize %s\n", version, d.Kyaml, d.Kustomize)
	return nil
}

func renderDocs(m *compat.Matrix) error {
	for _, f := range []string{readmeFile, changelogFile} {
		if _, err := compat.UpdateDoc(f, m); err != nil {
			return err
		}
	}
	return nil
}

// addChangelogEntry inserts a section for the new version directly below
// Unreleased, saying which kustomize release prompted it and whether anything
// observable moved.
func addChangelogEntry(version string, d compat.Decision, styleChanged bool) error {
	src, err := os.ReadFile(changelogFile)
	if err != nil {
		return err
	}
	const anchor = "## [Unreleased]\n"
	idx := strings.Index(string(src), anchor)
	if idx < 0 {
		return fmt.Errorf("%s has no %q section to insert below", changelogFile, strings.TrimSpace(anchor))
	}

	kind := "### Changed\n\n" +
		"- Built against kyaml " + d.Kyaml + ", the library shipped by kustomize " + d.Kustomize + ".\n" +
		"  The golden corpus is unchanged, so this release emits byte-identical output\n" +
		"  to its predecessor; only the provenance differs.\n"
	if styleChanged {
		kind = "### Changed\n\n" +
			"- **Output style changed.** Built against kyaml " + d.Kyaml + ", the library\n" +
			"  shipped by kustomize " + d.Kustomize + ", whose emitter produces different bytes.\n" +
			"  The golden corpus records the difference; review it before upgrading.\n"
	}

	entry := fmt.Sprintf("\n## [%s] - %s\n\n%s", version, time.Now().Format("2006-01-02"), kind)
	at := idx + len(anchor)
	out := string(src[:at]) + entry + string(src[at:])
	return os.WriteFile(changelogFile, []byte(out), 0o644)
}

// goCmd runs the Go toolchain. Output goes to stderr so that stdout stays
// clean for callers that parse it.
func goCmd(args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), netTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
