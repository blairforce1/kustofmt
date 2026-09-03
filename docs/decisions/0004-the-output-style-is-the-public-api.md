# 0004. The output style is the public API, and the golden corpus decides the version

- **Status:** accepted
- **Date:** 2026-08-20
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

For a formatter, a change in emitted bytes is the change users notice: a kyaml bump that moves a comment turns every pinned repository's `-l` gate red. Semver's "breaking change" needs a definition a test can apply. The brief said the style is the API and a style-affecting change is a breaking change.

## Decision

The golden corpus in `format/testdata` is the style contract. A changed golden file is a breaking change and is versioned as one. A kyaml bump that leaves the corpus untouched is a patch, because the release is observably identical to its predecessor. `make golden` regenerates the goldens; the job is reading the resulting diff and agreeing with it, and a golden diff in a pull request gets close review. Adding a fixture is two files and no wiring: every fixture is picked up by the golden, idempotency, semantics-preservation, no-comment-lost and diff-applies tests.

## Considered options

- **Version from commit messages (Conventional Commits, release-please).** The commit is written before the corpus runs, so it can only carry a guess (`decisions/0012`).
- **Version from the kyaml version.** kyaml's version does not track its emission. Every kyaml release in kustomize v5's history so far left the corpus unchanged.
- **Treat style changes as patches.** Breaks every pinned consumer silently.

## Consequences

- Easier: "did the style move" is a byte comparison, run on every pull request and by the release watcher (`decisions/0009`).
- Easier: the corpus doubles as documentation. Each fixture names one behaviour.
- Harder: the corpus must cover every behaviour the README promises, or a change escapes. Every README claim needs a fixture.
- Harder: a style-changing kyaml is refused by the watcher and a human decides the major version.

## Where it is enforced

`TestGolden` in `format/format_test.go`; `make golden`; `cmd/compat apply`, which refuses a corpus change without `--allow-style-change`; CONTRIBUTING "Adding a golden test case"; the CHANGELOG preamble.
