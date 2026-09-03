# 0012. Go-style commit subjects, not Conventional Commits

- **Status:** accepted
- **Date:** 2026-08-24
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

The blairforce1 repositories default to Conventional Commits, and the tooling around that convention derives the next version from commit messages. In this repository the version comes from `compatibility.yaml` and the bump size from the golden corpus (`decisions/0004`, `decisions/0009`): a measurement made after the commit is written, so a commit message could only carry a guess. `cmd/compat` already writes the CHANGELOG section and the generated tables, and `release.yaml` already tags on merge.

## Decision

Commit subjects follow Go's convention, `package: description` (`format:`, `cmd/kustofmt:`, `compat:`, `release:`, `docs:`, `ci:`, `build:`, `test:`, `deps:`), lowercase imperative, no trailing period, at most 72 characters. `scripts/check-commit-subject.sh` is the only definition. `make hooks` installs it as an opt-in `commit-msg` hook via `core.hooksPath`; `make commit-check` runs it in CI over `origin/main..HEAD`, with a full-depth checkout so the range is not empty. CI is authoritative: forks have no hooks, `--no-verify` skips them, and Dependabot and the watcher commit through the API. Commits carry no AI co-author trailer (`decisions/0014`).

## Considered options

- **Conventional Commits with release-please.** Contests `compatibility.yaml`, `cmd/compat` and `release.yaml` for no gain, and trades written release notes for a list of subjects.
- **No convention.** The log is the repository's explanation of itself. A prefix makes it scannable.

## Consequences

- Easier: the convention is the ecosystem's, for a tool whose whole argument is adopting style from its ecosystem.
- Harder: a contributor used to `feat:` and `fix:` is corrected by CI. CONTRIBUTING explains why in full.
- Harder: commits are signed, so amending a pushed subject means re-signing it.

## Where it is enforced

`scripts/check-commit-subject.sh`; `scripts/hooks/commit-msg`; `make commit-check` in the CI lint job; CONTRIBUTING "Commit messages".
