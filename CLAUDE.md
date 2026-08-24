# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What this is

A `gofmt`-shaped CLI that re-emits YAML in kustomize's house style, by calling
`sigs.k8s.io/kustomize/kyaml`'s own encoder rather than reimplementing it. One
direct dependency, on purpose. The README's "Style contract", "Non-goals" and
"Known limitations" sections are the specification — read them before changing
formatting behaviour.

## Commands

`make help` lists every target. Everything CI runs is a Makefile target, so a
green `make ci` locally means a green pipeline; if you add a check to CI, add
the target too.

```sh
make build          # static binary at ./kustofmt
make test-fast      # -short: unit, golden, CLI. The inner loop.
make test           # full suite, -race, coverage
make ci             # the whole offline pipeline (fmt, vet, tidy, lint, shellcheck, test, selfhost)
make release-check  # ci + fuzz + compat-check (needs network)
```

Narrower runs:

```sh
go test ./format/ -run TestGolden                      # the style contract
go test ./format/ -run 'TestGolden/flow-collections'   # one fixture
go test ./format/ -run FuzzIdempotent -fuzz FuzzIdempotent -fuzztime=60s
make golden                                            # regenerate goldens; see below
make sign-check                                        # release signing args (needs pinned cosign)
make compat-status                                     # unrecorded kustomize releases
```

`-short` skips the property tests (`format/property_test.go`) and anything that
shells out (`diff(1)`, `cosign`). `make fuzz` runs all four targets for
`FUZZTIME` (default 30s).

## Architecture

**`format/`** — the library, and the only place style lives.

`Format` is not a single pass. It formats, then *verifies its own output*
before returning it, because the underlying emitter has a known data-corrupting
bug (folded scalars containing an indented line). Two guards:

- *semantics* — input and output must decode to identical Go values. Compared
  against a baseline derived once from the input, never pass-to-pass, so drift
  cannot accumulate a step at a time.
- *convergence* — iterate to a fixed point, bounded by `maxPasses`. Comment
  placement makes the emitter non-idempotent in one pass; `-l` and pre-commit
  hooks depend on `fmt(fmt(x)) == fmt(x)`.

Some inputs (duplicate keys, aliasing past the decoder's budget) cannot be
semantics-checked at all. They are still formatted, but `ErrNotVerifiable` is
joined to any subsequent error so the diagnosis says the guard was inoperative.
Don't "simplify" that away.

The style itself is two explicit encoder calls in `formatOnce` —
`SetIndent(yaml.DefaultIndent)` and `CompactSeqIndent()`. They restate kyaml's
own defaults deliberately: that pair *is* the contract, and a silent upstream
default change should break a golden test rather than our output.

`blockify` forces block style on non-empty collections and does three repairs
the encoder would otherwise get wrong: empty `{}`/`[]` are left alone (no block
form exists), `!!merge` tags are stripped off `<<` keys, and a flow
collection's trailing line comment is relocated before conversion so it doesn't
migrate onto the following key.

`sops.go` detects encrypted files *structurally*, not textually — a `sops:`
mapping with a MAC and a key source, or `ENC[AES256_GCM,` payloads. A file with
a top-level `sops: "3.11.0"` version pin is not encrypted, and there is a
committed fixture for it.

**`cmd/kustofmt/`** — `run()` takes its streams as arguments so every path is
testable in-process; `main()` is four lines. `-w` writes to a temp file and
renames over the target (mode and ownership copied, symlinks followed), and
skips the write entirely when nothing changed so a clean run touches no mtimes.
Exit codes 0/1/2 are the contract for hooks and CI — treat them as API.

**`internal/compat/` + `cmd/compat/`** — the kustomize compatibility machinery.
`compatibility.yaml` is the single source of truth; the tables in `README.md`
and `CHANGELOG.md` are generated into `<!-- compat:begin/end -->` markers, and
CI re-derives every row from upstream rather than trusting the file. The kyaml
for a kustomize release is *resolved* by `scripts/kyaml-for-kustomize.sh` (two
independent methods that must agree), never derived arithmetically — the
version numbers do not track each other. `Matrix.Decide` returns one of three
actions: nothing to do, record-only (an existing binary already links that
kyaml), or rebuild. `.github/workflows/kustomize-watch.yaml` runs this daily
and opens a PR; merging to main tags and releases in one job.

**`internal/release/`** — reads cosign arguments out of `.goreleaser.yaml` and
the verify command out of `README.md` rather than restating either, so a
signing config or a documented command that stops working fails a test. It
skips unless the cosign on `PATH` matches the pin in `release.yaml`.

## Invariants

- **A changed golden file is a breaking change.** The output style is the
  public API. `make golden` will happily bless wrong output — the job is
  reading the resulting diff and agreeing with it. Version accordingly; a kyaml
  bump that leaves the corpus untouched is a patch.
- **No config file and no style options.** kustofmt is a style formatter, not a
  linter. See the README's non-goals before implementing a request that sounds
  reasonable.
- **Never use kyaml's `filters/fmtr` `FormatFilter`.** It is the obvious-looking
  API for "format like kustomize" and it is the wrong one: it sorts fields into
  a canonical order. Key order is preserved here, always. The encoder is the
  correct entry point, and `formatOnce` is the only place that should call it.
- **No new dependencies** without discussion. The one-direct-dependency count
  is a documented feature.
- `compatibility.yaml`'s `version` is what the release workflow tags. One row
  per kyaml is enforced; a release with no kyaml behind it (packaging, signing)
  advances `version` without adding a row.
- `make selfhost` runs the formatter over this repo's own YAML, excluding
  `format/testdata/` — those files are deliberately not in house style, because
  they are the inputs that prove the transformation.
- `.gitattributes` forces LF everywhere except `format/testdata/crlf.input.yaml`,
  whose carriage returns are the point.
- Adding a fixture is two files (`<case>.input.yaml`, `<case>.golden.yaml`) and
  no wiring: it is picked up automatically by the golden, idempotency,
  semantics and diff-applies tests.

## Conventions

- Commit subjects are Go-style (`package: description`), **not** Conventional
  Commits — `format:`, `cmd/kustofmt:`, `compat:`, `release:`, `docs:`, `ci:`,
  `build:`, `test:`, `deps:`; lowercase imperative description, no trailing
  period, 72 characters. `scripts/check-commit-subject.sh` is the enforced
  definition — do not restate the rule anywhere else. `make hooks` installs it
  as a commit-msg hook; `make commit-check` is what CI runs. CONTRIBUTING.md
  records why release-please does not fit: the version comes from
  `compatibility.yaml` and the bump size from the golden corpus, neither of
  which a commit message can know.
- Update `CHANGELOG.md` under `Unreleased` in the same change.
- Comments here explain *why*, at length, and several encode a bug that was
  actually hit. Match that density; don't strip them.
- `revive`'s `exported` rule is an error, not a warning — every exported symbol
  needs a doc comment.
