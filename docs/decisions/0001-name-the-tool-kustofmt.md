# 0001. Name the tool kustofmt, not kyamlfmt

- **Status:** accepted
- **Date:** 2026-08-20
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

The obvious name was `kyamlfmt`: the tool wraps `sigs.k8s.io/kustomize/kyaml`. Since Kubernetes v1.34 (beta and default-on in v1.35), KYAML is an official Kubernetes output encoding: a YAML subset that uses flow style for maps and sequences and double-quotes every string. That is the opposite of what this tool emits. A tool called `kyamlfmt` that removes flow style would mislead everyone who met the name. The kyaml library predates the encoding by six years and is unrelated to it.

## Decision

The tool is `kustofmt`, module `github.com/blairforce1/kustofmt`, image `ghcr.io/blairforce1/kustofmt`. The name says what it emits: kustomize's format. The README carries a "Not that KYAML" section that names the encoding, states the relationship plainly, and states that emitting KYAML is a non-goal because `kubectl -o kyaml` already exists. The kyaml library remains the dependency; only the public name was poisoned.

## Considered options

- **`kyamlfmt`.** Collides with the KYAML encoding. Actively misleading.
- **`kustomize-fmt` or `cfgfmt`.** Implies a kustomize subcommand or ownership by the kustomize project. Neither is true.
- **A name unrelated to kustomize.** Hides the one fact a user needs: whose style this is.

## Consequences

- Easier: the name and the one-line description ("gofmt for GitOps YAML, kustomize's emitted style as a formatter") say the same thing.
- Harder: the README spends a section on a tool this is not, for as long as KYAML is current.
- Follow-up: none. The name was checked for collisions on GitHub and pkg.go.dev at creation.

## Where it is enforced

README "Not that KYAML"; the module path in `go.mod`; `project_name` and the image name in `.goreleaser.yaml`.
