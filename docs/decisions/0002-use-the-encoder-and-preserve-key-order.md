# 0002. Call kyaml's encoder directly and preserve key order

- **Status:** accepted
- **Date:** 2026-08-20
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

The build brief pointed at `kio/filters.FormatFilter` as the canonical formatting pass and left key order open: preserve it for arbitrary YAML, accept kyaml's Kubernetes field ordering if that is what the library produces, and pin whichever happens in golden tests. The spike showed that `FormatFilter` sorts fields into a canonical order. A formatter that reorders fields turns the first run over a repository into a whole-file diff, makes `-d` unreadable, and makes the tool a linter with opinions about content.

## Decision

`formatOnce` is the only place that encodes. It calls kyaml's YAML encoder with `SetIndent(yaml.DefaultIndent)` and `CompactSeqIndent()`, restated explicitly even though they are kyaml's defaults, so a silent upstream default change breaks a golden test rather than the output. Key order is preserved always, for Kubernetes resources and arbitrary YAML alike. `FormatFilter` is never used, and the repository's CLAUDE.md records that as an invariant. `blockify` does the three repairs the encoder does not: it leaves empty collections in flow style, strips `!!merge` tags from `<<` keys, and relocates a flow collection's trailing comment before conversion so it does not migrate onto the next key.

## Considered options

- **`kio/filters.FormatFilter`.** The obvious API and the wrong one: it sorts fields.
- **Preserve order for arbitrary YAML, canonical order for recognised resources.** Two behaviours, one flag away from a config option. A kustomize-emitted file already arrives in canonical order, so nothing is gained.
- **A YAML library other than kyaml.** Would replicate kyaml's emission instead of using it, so the style could drift from the ecosystem it claims to match.

## Consequences

- Easier: "what you wrote is what you get, restyled" is the whole promise. The first run over a repository is a whitespace diff.
- Easier: `kustomize build` output and `flux create --export` output pass `-l` untouched, because they arrive in the emitter's own order.
- Harder: the style is defined by two encoder calls and a repair pass, not by a named library function. The golden corpus is the only complete statement of it (`decisions/0004`).
- Harder: whatever the encoder gets wrong is this tool's to detect (`decisions/0007`).

## Where it is enforced

`format/format.go` (`formatOnce`, `blockify`); the CLAUDE.md invariant "Never use kyaml's `filters/fmtr` `FormatFilter`"; `TestGolden` over `format/testdata`, in particular the `kustomize-output` and `flux-kustomization` fixtures.
