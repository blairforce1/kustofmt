# 0009. One kustofmt release per kyaml, mapped to kustomize releases and owned by a watcher

- **Status:** accepted
- **Date:** 2026-08-20 (explicit `version` field added 2026-08-21)
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

The style is kyaml's emitter, so "emits what kustomize emits" is only checkable if a user can pin the kustofmt built from the kyaml their kustomize ships. kyaml's version does not track kustomize's: of ten kustomize v5 releases checked, four ship a kyaml whose version differs from their `api` module, and v5.5.0 ships a kyaml ahead of it. The brief asked for a README mapping table and for Renovate or Dependabot to propose kyaml bumps with the golden suite as the gate. A Dependabot bump would put `go.mod` out of step with the matrix and produce a pull request that can never pass, weekly, forever.

## Decision

`compatibility.yaml` is the source of truth: one row per kyaml, each listing the kustomize CLI releases that ship it, and a `version` field that the release workflow tags. The README and CHANGELOG tables are generated into `compat:begin/end` markers from it, and CI re-derives every row from upstream's `go.mod` at the tag, so a wrong row fails the build. The kyaml for a kustomize release is resolved two independent ways (full MVS resolution through the Go toolchain, and the `go.mod` published at the tag) that must agree, and is never derived arithmetically. A daily watcher takes the oldest unrecorded kustomize release and either records it against an existing kustofmt (a binary already links that kyaml, so nothing is rebuilt) or rebuilds against the new kyaml and prepares the next version, with the golden corpus deciding patch or refuse (`decisions/0004`). Merging the watcher's pull request tags and releases in one job. Dependabot ignores kyaml. Tracking starts at a stated floor (5.6.0); older releases are deliberately absent. A release with no kyaml behind it (packaging, signing) advances `version` without adding a row.

## Considered options

- **Dependabot or Renovate on kyaml.** Produces the unpassable pull request described above.
- **A mapping table maintained by hand.** Wrong rows sit there looking plausible.
- **Derive kyaml from the kustomize version.** Wrong four times in ten.
- **One release per kustomize release.** More releases with identical output. The row lists them instead.

## Consequences

- Easier: "pin kustofmt 0.1.2 for kustomize 5.7.1" is a lookup, and every row is proven by CI.
- Easier: the historical replay (kustomize 5.6.0 to 5.8.1) was cut as 0.1.0 to 0.1.4 on one day from the same mechanism.
- Harder: `cmd/compat` and `internal/compat` are a large share of the Go in a formatter's repository.
- Harder: the watcher needs the repository setting that lets Actions open pull requests, and a pull request opened with `GITHUB_TOKEN` starts in an approval-required state. Both are documented.
- Follow-up: a style-changing kyaml is a human decision and a major-version conversation.

## Where it is enforced

`compatibility.yaml`; `cmd/compat` and `internal/compat`; `scripts/kyaml-for-kustomize.sh`; `.github/workflows/kustomize-watch.yaml`; the `ignore` entry in `.github/dependabot.yml`; the CI `compat` job; the "Decide whether to release" step in `release.yaml`.
