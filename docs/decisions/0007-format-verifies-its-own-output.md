# 0007. Format verifies its own output before returning it

- **Status:** accepted
- **Date:** 2026-08-20
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

Fuzzing the first version found two defects in the underlying emitter. A folded scalar containing an indented line is re-emitted with an extra newline in it, which silently changes the value. A foot comment can gain a blank line on the pass after it is first written, so one pass is not idempotent, and `-l` and pre-commit hooks depend on `fmt(fmt(x)) == fmt(x)`. Neither is hypothetical; both have committed fuzz seeds. A formatter that cannot vouch for its output should refuse to produce it rather than hand back a file that looks fine.

## Decision

`Format` runs two guards on every call, not only in tests. The semantics guard: input and output must decode to identical Go values, compared against a baseline derived once from the input, never pass-to-pass, so drift cannot accumulate a step at a time. The convergence guard: iterate to a fixed point, bounded by `maxPasses` (4); `ErrNotConverged` exists so the failure cannot be silent. Semantics are checked before convergence, because a meaning change is the more serious failure and the more specific diagnosis. Inputs that do not decode (duplicate keys, aliasing past the decoder's budget) cannot be semantics-checked; they are still formatted, and `ErrNotVerifiable` is joined to any later error so the diagnosis says the guard was inoperative. The folded-scalar case is refused, with a suggestion to rewrite the scalar as a literal.

## Considered options

- **Trust the emitter and test in CI only.** The corpus cannot contain every input. The fuzzer proved the defect exists.
- **Refuse unverifiable inputs.** Rejects documents that are unusual, not malformed.
- **Fix the emitter upstream.** Right in the long run and not a substitute. The guard stays, because it protects against the next defect too.

## Consequences

- Easier: `-w` cannot write a file whose meaning changed. SECURITY.md can promise that.
- Harder: every file is parsed at least three times (format, baseline, verify). Acceptable at the size of GitOps files.
- Harder: a user with the folded-scalar shape gets a refusal instead of a format, and rewrites the scalar.

## Where it is enforced

`Format`, `baseline` and `verifySemantics` in `format/format.go`; `FuzzIdempotent` and `FuzzSemanticsPreserved` with committed seeds under `format/testdata/fuzz`; README "Known limitations"; the CLAUDE.md architecture section.
