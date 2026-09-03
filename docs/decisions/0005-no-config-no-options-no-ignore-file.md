# 0005. No config file, no style options, no ignore file

- **Status:** accepted
- **Date:** 2026-08-20 (the ignore-file stance made explicit 2026-08-21, PR #8)
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

The house style is defined by an emitter, not by preferences. A formatter with knobs has to document which combination is the house style and defend every other one. An ignore file sounds harmless and is the first knob. Tool-owned trees (`flux-system/`, vendored charts, generated manifests) are rewritten by their generator, so reformatting them is a fight the next regeneration wins, and the gate goes red again on someone else's schedule.

## Decision

kustofmt reads no configuration and has no style flags. There is no ignore file and there will not be one; gofmt has none either. Which trees are tool-owned is something only the caller knows, so scoping is the caller's job, done with git's own pathspec:

```sh
git ls-files -z '*.yaml' '*.yml' ':!:**/flux-system/**' | xargs -0 -r kustofmt -l
```

The tool's job is to refuse the files it can prove it must not touch (`decisions/0006`). Requests for options are closed with a link to the README's non-goals.

## Considered options

- **A `.kustofmt.yaml` with exclude patterns.** yamlfmt has one. It becomes the place style options accumulate.
- **Reading `.gitignore` or `.yamllint.yaml`.** Couples the tool to files with other owners and other semantics.
- **Built-in knowledge of `flux-system/`.** A hard-coded list of other tools' directories, wrong for the next tool.

## Consequences

- Easier: the tool is a pure function over files or stdin. The README's CI example is the whole integration.
- Harder: every consumer writes the same pathspec, and the README has to teach it, including the trap of matching `flux-system` as text and dropping a file named `flux-system.podmonitor.yaml`.
- Harder: "could we add an option for" is a recurring conversation. CONTRIBUTING pre-empts it.

## Where it is enforced

The flag set in `cmd/kustofmt/run.go`; README "Non-goals" and "In CI"; CONTRIBUTING "Scope"; the CLAUDE.md invariant "No config file and no style options".
