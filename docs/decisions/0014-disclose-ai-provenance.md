# 0014. State AI authorship in the README, and from this decision on as a co-author trailer on every generated commit

- **Status:** accepted
- **Date:** 2026-09-03 (revised twice the same day, before landing: the first draft chose the README alone, the second a rewrite of the history)
- **Deciders:** blairforce1
- **Supersedes:** none

## Context

kustofmt was written entirely by Claude from a design brief: Claude Opus 5 for the build and the releases (2026-08-20 to 2026-08-24, 47 commits), Claude Fable 5 for one documentation commit (2026-08-30), Claude Fable 5.1 from 2026-09-03. blairforce1 wrote the brief, made the design decisions, reviewed every change and is responsible for the result. A reader of a public repository should know how its code was made. The house rule for blairforce1 repositories is that commits carry no AI co-author trailer, because in those repositories the assistant edits work the author leads. Here the code is wholly machine-written, and for generated code the honest unit of attribution is the commit as well as the README.

## Decision

Two places, one fact, applied from the date of this decision forward. The README carries a "Provenance" section stating that Claude wrote the code, tests, workflows and documents from a brief, that blairforce1 made the decisions and owns the result, and that commits before 2026-09-03 were Claude-written even though they carry no trailer. From this decision on, every commit whose content Claude generated carries a trailer naming the model that wrote it:

```text
Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>
```

The model named is the one that produced the commit, not the current one; a commit made from a later session states its own. Commits with no Claude content carry no trailer: Dependabot and GitHub sign their own commits as they always have. No session identifiers are recorded, because a reader cannot resolve them. The existing history is left as it was: a rewrite would put trailers on commits dated before this decision existed, which is a small fiction of the kind the trailers exist to prevent, and the seam at 2026-09-03 is the honest record of when the practice began. This repository is the exception to the house rule, not a change to it, and its CLAUDE.md states the rule for this repository. The copyright holder stays blairforce1 and NOTICE is unchanged: a licence notice is not an authorship narrative. The decision records name the model on their Deciders line where a model implemented the decision.

## Considered options

- **Say nothing.** Leaves a reader to guess how the code was made.
- **README only (the first draft).** The right fact in the wrong unit: a reader of a commit sees one author.
- **Rewrite the history to add trailers everywhere (the second draft).** Technically safe if no tree changes, and a runbook was written for it. Rejected because back-dated trailers misstate when the practice started, and because a force-push of a public history with six releases carries risk for no gain in honesty.
- **A session trailer as well.** Names a session a reader cannot open. Noise.
- **An AUTHORS file.** Not where a reader looks.

## Consequences

- Easier: README, commit and release now say the same thing about who wrote what, from the same date.
- Easier: the commit that lands this record is the first one carrying a trailer, which is its own evidence.
- Harder: a reader of the August history sees one author and has to read the README to learn the truth. The README states the boundary plainly.
- Harder: every future commit needs the trailer with the right model. The harness supplies its current model by default, which is right for the session making the commit and wrong if a message is copied.
- Follow-up: a trailer check could join `make commit-check`; not built. The same disclosure applies to the sibling tools (fleetnotes, fluxevents) when they exist.

## Where it is enforced

README "Provenance"; the `Co-Authored-By` trailer on every generated commit from 2026-09-03; the repository's CLAUDE.md conventions; the Deciders line of every record here. The commit-subject check does not police trailers, so that half is review.
