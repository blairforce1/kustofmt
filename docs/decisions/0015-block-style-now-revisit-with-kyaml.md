# 0015. Emit block style now, and re-evaluate when KYAML is in common use

- **Status:** accepted
- **Date:** 2026-09-03 (the style itself dates from the course convention of 2026-08-20; this record states the reason and the revisit condition)
- **Deciders:** blairforce1
- **Supersedes:** none

## Context

kustofmt was developed for the gitops-golden-path course, as an example of what can be done when the tool a repository needs does not exist. The course's config repository bans flow style and needed a formatter that matched what kustomize and Flux emit. Block style with indentless sequences is what those tools write today and what most Kubernetes repositories contain. Kubernetes has since defined KYAML, a flow-style, all-strings-quoted subset built for safe hand-authoring, on by default for kubectl output from v1.35. A formatter has to pick one. The two styles serve different problems, and which one a config repository should standardise on will look different once KYAML is common.

## Decision

kustofmt emits block style, and the course standardises on it. The reason is stated as an opinion held now, not a law: block style is the more common style in the ecosystem today, and in the author's current view it is the easier one to read in a diff. Emitting KYAML stays a non-goal (`decisions/0001`). When KYAML is in wide use, the tooling and the course's approach are re-evaluated with the fuller picture. This record is where that re-evaluation is owed. Until then the tool does one thing.

## Considered options

- **Emit KYAML.** `kubectl -o kyaml` already does. The repositories this tool serves are written by kustomize and Flux, which emit block style.
- **Offer both as an option.** A formatter with knobs (`decisions/0005`).
- **Decide once, for good.** The premise, which style is common, is time-bound.

## Consequences

- Easier: the tool and the course say the same thing and say why.
- Harder: an opinion is on the record and will age. The revisit is owed, not scheduled.
- Follow-up: the course's YAML style decision should carry the same clause.

## Where it is enforced

README "Why this exists" and "Not that KYAML"; the course's style convention and its style gate; this record.
