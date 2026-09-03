# 0003. One direct dependency: kyaml and the standard library

- **Status:** accepted
- **Date:** 2026-08-20
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

A formatter that runs in pre-commit hooks and CI across many repositories has a supply-chain surface. Every dependency is a paragraph in SECURITY.md and a Dependabot pull request to review. The brief set the list as kyaml plus the standard library, CLI parsing via `flag` as gofmt does, and allowed test-only dependencies.

## Decision

`go.mod` has one direct requirement, `sigs.k8s.io/kustomize/kyaml`. The CLI uses `flag`. Unified diffs are computed in-package (`format/diff.go`). Property tests use `math/rand` and `reflect`; fuzzing uses Go's native fuzzer. No test-only dependency was taken either: the brief allowed one, but the property tests needed nothing a hand-rolled generator could not provide, and one requirement in `go.mod` is a claim the README can make in one sentence. Any new dependency needs discussion first.

## Considered options

- **cobra and viper.** Standard for Kubernetes CLIs. Unnecessary for five flags and a config file that does not exist.
- **go-cmp, rapid or a mutation tool as test dependencies.** Allowed by the brief, not needed. A mutation tool would have been the largest dependency for the least-used lane (`decisions/0013`).
- **A smaller YAML library.** Would replicate kyaml's emission rather than use it (`decisions/0002`).

## Consequences

- Easier: the shipped import graph is kyaml's. SECURITY.md's threat model is three bullets.
- Harder: kyaml brings 13 indirect modules, including kube-openapi and protobuf, and the binary is about 12 MB. The README states the number rather than hiding it.
- Harder: the diff algorithm is hand-written and had to be tuned (the LCS table halved and allocated once).
- Follow-up: Dependabot groups the indirect modules weekly. kyaml is excluded from it (`decisions/0009`).

## Where it is enforced

`go.mod`; `make tidy-check`; CONTRIBUTING "No new dependencies without discussing it first"; README "Known limitations" (binary size).
