# 0013. CI is a backstop: every check is a Makefile target, tests are tiered, and nothing reports to a third party

- **Status:** accepted
- **Date:** 2026-08-20 (dead controls removed 2026-08-21, PR #4)
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

A red CI on a mechanical check is a process failure: the check existed and did not run locally. The brief asked for tiered tests (short on pull requests, the full battery on main and releases), coverage with a badge and a threshold, mutation testing, property-based testing, and a self-hosting check. The repository briefly uploaded coverage to a third-party service that silently rejected every upload for want of a token, and carried a Go Report Card badge for a service that had been retired: two green controls that checked nothing.

## Decision

`make ci` is the whole offline pipeline (gofmt, vet, tidy, lint, shellcheck, commit-check, race tests with coverage, selfhost), every CI job maps to a target, and a check added to CI is added to the Makefile. The lanes: `make test-fast` (`-short`: unit, golden, CLI) for the inner loop; `make test` (plus property tests, `-race`, coverage) before pushing; `make fuzz` (four targets, `FUZZTIME`) before a release; `make release-check` (ci, fuzz, compat-check) when cutting a tag, which the release job runs with `FUZZTIME=60s`. CI runs a 20-second fuzz smoke on every pull request and the tests on ubuntu, macos and windows. Coverage is written to the run's own step summary; there is no badge, no threshold and no upload. Mutation testing was not built: the fuzzers and property tests exercise the three guarantees directly, and a mutation tool would have been a dependency for the least-used lane. Linters are pinned (golangci-lint, the shellcheck image) so local and CI agree about what is clean. The self-hosting check runs the built binary over the repository's own YAML, excluding `format/testdata`, whose files are deliberately not in house style.

## Considered options

- **A coverage badge with a threshold.** A number that goes green without saying whether the tests kill anything. The guards and fuzzers say more.
- **go-gremlins in the full battery.** Dropped from the brief's list rather than shipped unrun.
- **Build tags for the tiers.** `testing.Short()` was enough and needs no flag on the command line.

## Consequences

- Easier: a green `make ci` locally means a green pipeline.
- Harder: two items the brief asked for, the coverage badge and mutation testing, are absent by decision. This record is where that is said.
- Harder: the Windows job needed `shell: bash`, because PowerShell splits `-coverprofile=cover.out`.

## Where it is enforced

`Makefile`; `.github/workflows/ci.yaml`; CONTRIBUTING "Build and test in three commands"; the CLAUDE.md commands section.
