# kustofmt: product requirements

- **Status:** accepted (reconstructed)
- **Date:** 2026-09-03, reconstructed from the build brief of 2026-08-20 and the repository at 0.1.5. Decisions are dated 2026-08-20 to 2026-09-03.
- **Owner:** blairforce1
- **Built by:** Claude Opus 5 in Claude Code, from the brief, under blairforce1's direction; later documentation commits by Claude Fable 5 and Fable 5.1, each commit's trailer naming its model (`decisions/0014`)
- **Decision records:** [`../decisions/INDEX.md`](../decisions/INDEX.md). Requirements cite records as `decisions/NNNN`.

The tool was generated from a written build brief, kept with the course's design record. This specification restates that brief in the shape of a requirements document for the tool as built. Where the build departed from the brief, this document records the build and lists the departure in §11.

## 1. Problem

`kustomize cfg fmt` formatted YAML into kustomize's canonical style. It was removed in kustomize v5 with the rest of the `cfg`/`fn` alpha porcelain, for scope and maintenance reasons during the v5 API stabilisation ([kustomize#3953](https://github.com/kubernetes-sigs/kustomize/issues/3953)). A formatting transformer was suggested as the replacement and never shipped, and users kept meeting the deprecation warning and asking where fmt went ([kustomize#4339](https://github.com/kubernetes-sigs/kustomize/issues/4339)). The command was cut for scope, not for lack of demand. The machinery survived as a library, `sigs.k8s.io/kustomize/kyaml`.

The gap is real. kustomize and Flux both emit one style: two-space maps, indentless sequences (the dash in the same column as its key), block style throughout. The tools people edit YAML with fight it. `yq` re-indents every sequence in any file it touches, and no option changes that, because its emitter couples dash position to indent width. `prettier` cannot produce indentless sequences at all. `google/yamlfmt` comes closest and falls short on three points (§8). The result is a GitOps repository where half the YAML is written by machines in one style, hand edits drift into others, and every review carries whitespace noise nobody asked for.

kustofmt was developed for the [gitops-golden-path](https://github.com/blairforce1/gitops-golden-path-course) course, as an example of what can be done when the tool a repository needs does not exist. The course's config repository bans flow style and needed a formatter that matched the tools' own output, so one was built, and the course consumes it as a pinned filter. Block style is a deliberate current choice, not an accident of the emitter, and it carries a stated revisit condition as KYAML spreads (`decisions/0015`).

## 2. Consumers

- **git pre-commit hooks.** Changed files only, fast, with a one-line fix instruction.
- **CI style jobs.** A version-pinned container over the whole tree, scoped past tool-owned directories.
- **Editors on save.** Filter mode: stdin to stdout.
- **The consumer it was built for.** The gitops-golden-path course's config repository, whose conventions the tool encodes (indentless style, a ban on flow style, encrypted files untouchable) and which pins the tool by version in its hooks and CI. It shaped the golden corpus and found the container integration issues (§7). Nothing there blocks the general tool.

## 3. Goals

- G1. Emit byte-for-byte what kyaml emits, by calling kyaml's own encoder rather than describing its style (`decisions/0002`).
- G2. A gofmt-shaped CLI: instantly familiar, composable in pipes, with exit codes hooks and CI can branch on (`decisions/0008`).
- G3. Safe to run over a production repository on day one: idempotent, semantics-preserving, verified per file, atomic on write, and skipping encrypted files by default (`decisions/0006`, `decisions/0007`).
- G4. Compatible: files freshly emitted by `kustomize build` and `flux create --export` pass the check untouched.
- G5. Pinnable to a kustomize release, with the mapping proven rather than asserted (`decisions/0009`).
- G6. A public repository fit to be depended on: the complete open-source document set, CI that is reproducible locally, signed releases, and a comparison table whose every claim was verified against the competing tool (`decisions/0011`, `decisions/0013`).

## 4. Non-goals

Restraint is a feature. kustofmt does not:

- N1. Emit KYAML. `kubectl -o kyaml` is that tool (`decisions/0001`). Block style is the choice for now, with the re-evaluation owed when KYAML is in common use (`decisions/0015`).
- N2. Read a config file, offer style options, or read an ignore file (`decisions/0005`).
- N3. Validate against schemas, convert YAML 1.1 to 1.2, alphabetise maps, reorder fields, or expand anchors and aliases.
- N4. Touch the network. Ever.
- N5. Ship mutation testing or a coverage badge (`decisions/0013`).
- N6. Take a dependency beyond kyaml and the standard library, in the binary or in the tests (`decisions/0003`).

## 5. The product

**kustofmt.** One line: gofmt for GitOps YAML, kustomize's emitted style as a formatter. Module `github.com/blairforce1/kustofmt`, image `ghcr.io/blairforce1/kustofmt`, topics `yaml`, `formatter`, `kubernetes`, `gitops`, `kustomize`. The name is chosen against a collision: KYAML is an official Kubernetes output encoding since v1.34 that is the opposite of this style, so `kyamlfmt` is banned (`decisions/0001`).

The transformation, from the repository that motivated the tool:

```yaml
# before                          # after
spec:                             spec:
  containers:                       containers:
    - name: app                     - name: app
      resources:                      resources:
        requests: {cpu: 50m}            requests:
      ports:                              cpu: 50m
        - containerPort: 8080           ports:
                                        - containerPort: 8080
```

## 6. Requirements

### 6.1 The style contract

Output is byte-for-byte what kyaml emits (`decisions/0002`, `decisions/0004`).

- S1. Two-space map indentation.
- S2. Sequence items are not indented relative to their key. The encoder is forced to compact style, never left to preserve the input's.
- S3. Block style everywhere. Non-empty flow maps and sequences are converted to block. Genuinely empty collections (`{}`, `[]`) stay flow, because they have no block form.
- S4. Comments are preserved and attached to the nodes they were written on. A flow collection's trailing comment is relocated before conversion so it does not migrate onto the following key.
- S5. Scalar formatting and quoting are preserved. `"1.36"` stays a quoted string. Strings are never re-wrapped. Literal block scalars are untouched.
- S6. Key order is preserved, always, for Kubernetes resources and arbitrary YAML alike.
- S7. Multi-document files are preserved, and a leading `---` round-trips per file: Flux exports keep theirs, kustomize output stays bare.
- S8. Anchors, aliases and merge keys (`<<:`) survive untouched. `!!merge` tags are stripped from `<<` keys so the encoder does not print them.
- S9. CRLF input becomes LF. Mixed line endings are a formatting inconsistency like any other, and LF is what the ecosystem emits.
- S10. A file that parses to zero documents (empty, whitespace only, comments only) is returned unchanged, because the object model cannot represent it and re-emitting would discard its comments.

### 6.2 Guarantees

Each is backed by tests, and the first two are checked on every file the tool formats, not only in the suite (`decisions/0007`).

- V1. **Idempotent.** `fmt(fmt(x)) == fmt(x)`, byte for byte. The emitter is not stable in one pass for every comment placement, so `Format` iterates to a fixed point, bounded, and fails loudly if it does not converge.
- V2. **Semantics-preserving.** Input and output decode to identical values, compared against a baseline derived once from the input. A document that does not decode (duplicate keys, aliasing past the decoder's budget) is still formatted, with the failure mode reported as unverifiable if anything else then goes wrong.
- V3. **Compatible.** A file emitted by `kustomize build` and a Flux `Kustomization` export pass `-l` untouched. Both are golden fixtures captured from real output.
- V4. **Refuses rather than corrupts.** Where the emitter is known to change a value (a folded scalar containing an indented line), the file is refused with a suggestion, never written.

### 6.3 The command line (`decisions/0008`)

- C1. `kustofmt [flags] [path ...]`. Paths are files or directories; directories are walked recursively for `*.yaml` and `*.yml`, following symlinked directories.
- C2. No paths: read stdin, write stdout. Filter mode.
- C3. One path and no mode flag: print the formatted result. Several paths require a mode flag.
- C4. `-l` lists files whose formatting differs and exits 1 if any. This is the gate for hooks and CI.
- C5. `-w` writes the result back in place: to a temp file beside the target, renamed over it, mode and ownership copied, symlinks followed, and no write at all when nothing changed.
- C6. `-d` prints unified diffs and exits 0. The explanation, not the gate.
- C7. `--include-sops` formats encrypted files. `-version` prints the version and the kyaml it was built against. `-h` prints usage on stdout and exits 0.
- C8. Exit codes: 0 clean or succeeded, 1 files need formatting under `-l`, 2 operational error (unreadable file, parse failure). A parse failure names the file and position and does not stop the walk.
- C9. Encrypted content never reaches stdout in `-l` or `-d` mode. In filter mode an encrypted document passes through byte-identically.
- C10. Every code path runs in-process under test: `run()` takes its streams as arguments and `main()` is four lines.

### 6.4 sops safety (`decisions/0006`)

- P1. A sops-encrypted file is skipped by default, with a notice on stderr naming the file and the override.
- P2. Detection is structural: a `sops` mapping carrying a MAC and a key source, or an `ENC[AES256_GCM,` payload. A top-level `sops: "3.11.0"` version pin is not encrypted and is a committed fixture.
- P3. Unparseable input is not classified as encrypted; the parse error is reported instead.
- P4. A false negative in detection is a security-relevant bug and SECURITY.md says so.

### 6.5 The library

- L1. `format` is a public package: `Format([]byte) ([]byte, error)`, `IsSOPS([]byte) bool`, `Diff(a, b string, src, out []byte)`, and the named errors `ErrNotConverged`, `ErrSemanticsChanged`, `ErrNotVerifiable`.
- L2. Every exported symbol has a doc comment; the linter treats a missing one as an error.
- L3. The package doc states the contract in full, and comments explain why at length, several encoding a bug that was actually hit.

### 6.6 Repository shape and quality (`decisions/0003`, `decisions/0013`)

- Q1. Go, current stable release, `CGO_ENABLED=0`, `-trimpath`, one direct dependency.
- Q2. Layout: `cmd/kustofmt/` (the CLI), `format/` (the library and `testdata/`), `cmd/compat/` and `internal/compat/` (the compatibility matrix), `internal/release/` (signing checks), `scripts/`, `compatibility.yaml`, `Makefile`, the four workflows (`ci`, `release`, `kustomize-watch`, `signing-smoke`), `.goreleaser.yaml`, `Dockerfile`, `.golangci.yaml`, `.gitattributes`, `.github/dependabot.yml`, and the document set in §6.9.
- Q3. Golden cases, at minimum: indented sequences to indentless; flow collections to block; empty collections retained; comments above, inline, footer, and comments-only; block scalars; anchors and aliases; quoted scalars; a GitHub Actions workflow (`on:` key, nested lists); multi-document bare and with a leading separator; a kustomize-emitted file (zero diff); a Flux Kustomization export (zero diff); CRLF input; empty and whitespace-only files; a sops-encrypted Secret (skipped) and the sops version-pin false positive; the worked example from §5.
- Q4. Adding a fixture is two files and no wiring. Every fixture feeds the golden, idempotency, semantics, no-comment-lost and diff-applies tests, and seeds the fuzzers.
- Q5. Four fuzz targets (idempotent, semantics preserved, diff applies, sops detection never panics); inputs the fuzzer finds are committed under `format/testdata/fuzz` as permanent regressions.
- Q6. Property tests over generated YAML drive V1 and V2; `-short` skips them and anything that shells out.
- Q7. `make ci` is the whole offline pipeline and every CI job maps to a Makefile target: gofmt, vet, tidy, pinned golangci-lint, pinned shellcheck, commit-subject check, race tests with coverage on ubuntu, macos and windows, cross-compile for five targets, the self-hosting check, the compatibility re-derivation, the signing check, and a 20-second fuzz smoke.
- Q8. The self-hosting check runs the built binary over the repository's own YAML, excluding `format/testdata`, whose files are deliberately not in house style.
- Q9. `.gitattributes` forces LF everywhere except the CRLF fixture.
- Q10. Commit subjects are Go-style `package: description`, checked by one script that both the opt-in hook and CI run (`decisions/0012`).

### 6.7 Distribution and release (`decisions/0010`, `decisions/0011`)

- R1. GoReleaser on a `v*` tag, and on a merge to main whose matrix version is untagged, in one job (a tag pushed with `GITHUB_TOKEN` does not trigger a tag workflow). Releases are queued, never cancelled.
- R2. Archives for linux and darwin on amd64 and arm64, windows on amd64; `checksums.txt`; release notes extracted from the CHANGELOG section for the version, and the job fails if the section is missing; version injected via `-ldflags -X main.version=`; a commit-pinned build timestamp.
- R3. A `FROM scratch` multi-arch image, `ghcr.io/blairforce1/kustofmt:<version>` and `latest`, running as uid 65532, with OCI labels and annotations. Check mode only in the container; the write idiom is a pipe with the host writing.
- R4. SBOMs for the archives and the image.
- R5. Keyless cosign signing via GitHub OIDC. The blob signature is a Sigstore bundle; the image signature stays classic for Flux. Every tool-installing action pins its tool.
- R6. The release job runs the README's own verify command against what it published, including with Rekor unreachable, and fails if it does not pass.
- R7. `make release-check` (ci, fuzz, compat-check) runs before anything is published.
- R8. Semver from 0.1.0. A style-affecting change is a breaking change (`decisions/0004`).
- R9. Every release's notes state the kyaml it was built against.

### 6.8 Compatibility tracking (`decisions/0009`)

- K1. `compatibility.yaml` is the single source of truth: one row per kyaml, each listing the kustomize CLI releases that ship it, a `floor` below which nothing is tracked, and a `version` that the release workflow tags.
- K2. The README and CHANGELOG tables are generated from it between markers, and CI re-derives every row from upstream's `go.mod` at the tag.
- K3. The kyaml for a kustomize release is resolved two independent ways that must agree, never derived arithmetically.
- K4. A daily watcher takes the oldest unrecorded kustomize release and opens a pull request: record-only when an existing kustofmt already links that kyaml, otherwise a rebuild whose version the golden corpus decides. A corpus change makes it refuse.
- K5. Merging the watcher's pull request tags and releases.
- K6. Dependabot ignores kyaml and groups everything else weekly.
- K7. `kustofmt -version` prints the kyaml. The README explains how to read the table and why a kyaml bump is usually a patch.

### 6.9 The document set

- D1. **README**: the lineage (§1) with the two issues cited; the worked example; install (release, `go install`, container, and the container write idiom); the verify commands for both signature formats; usage, flags, exit codes; hook and CI examples including scoping past tool-owned trees; library usage; the style contract and guarantees; sops safety; the comparison table with its verification date and the script that produced it, and the courtesy paragraph about yamlfmt; "Not that KYAML"; non-goals; known limitations; version compatibility; provenance (`decisions/0014`); badges for CI, release, Go reference and licence.
- D2. **LICENSE** Apache-2.0, with a NOTICE naming kyaml and the Kubernetes authors.
- D3. **CONTRIBUTING**: build and test in three commands; the lanes; adding a golden case; reporting a formatting bug as a fuzz seed; pull request rules; the commit convention and why not Conventional Commits; tracking kustomize releases and reviewing a watcher pull request; signing and its two traps; scope.
- D4. **CODE_OF_CONDUCT**: Contributor Covenant 2.1.
- D5. **SECURITY**: private reporting via GitHub advisories, a supported-versions table (latest minor), and a threat model in three bullets.
- D6. **CHANGELOG**: Keep a Changelog, an Unreleased section maintained in every change, the generated compatibility table, and the preamble stating that output style is the public API.
- D7. **CLAUDE.md**: commands, architecture, invariants and conventions for an agent working in the repository.
- D8. `scripts/compare-tools.sh` reproduces the comparison table.

### 6.10 Identity and licence

- I1. Published as `blairforce1`. No other name anywhere: not in the README, LICENSE, NOTICE, contacts, author fields or git metadata. Contact address `git@blairforce1.com`; copyright `Copyright 2026 blairforce1`. The local git identity is checked before the first commit.
- I2. Commits are signed, small, with real messages. From 2026-09-03, every commit whose content Claude generated carries a `Co-Authored-By` trailer naming the model that wrote it; Dependabot and GitHub sign their own commits, and no session trailers are recorded. Earlier commits were left as they were. This is the exception to the house rule for blairforce1 repositories (`decisions/0014`).
- I3. Apache-2.0, consistent with the kustomize ecosystem.
- I4. The README states that the code was written by Claude from a brief, that blairforce1 made the decisions, and that commits before 2026-09-03 carry no trailer and were Claude-written all the same (`decisions/0014`).

## 7. Known limitations, accepted

Stated in the README, because a formatter you cannot trust is worse than no formatter.

- Folded scalars containing an indented line are refused (V4).
- A document that does not decode is formatted without the semantics guard, and any later failure says so (V2).
- Folded scalars are re-flowed to the emitter's width; the value is unchanged.
- Blank lines between keys are removed. Machine-emitted YAML does not have them.
- Aligned inline-comment columns collapse to one space. The first consumer normalised its own files to match.
- Parse-error line numbers can be one line early for some error classes. The filename is always right.
- `%YAML` directives are refused by the parser; `%TAG` shorthands are expanded to their verbose form. Neither appears in kustomize or Flux output.
- CRLF becomes LF (S9).
- `-w` gives the file a new inode, which breaks hard links and open readers (C5).
- The binary is about 12 MB: one direct dependency, 13 indirect modules. The price of using kyaml's emission rather than replicating it.

## 8. Prior art, verified

Checked on 2026-08-20 against yamlfmt v0.21.0, yq v4.53.2 and prettier 3.9.6, with the binaries, not from memory. `scripts/compare-tools.sh` is the source of the README table.

| Requirement | kustofmt | yamlfmt | yq | prettier | `kustomize cfg fmt` |
|---|---|---|---|---|---|
| Indentless sequences | yes | yes, with `indentless_arrays` | no, always re-indents | no option | yes |
| Flow arrays to block | yes | yes, with `force_array_style` | no | no | yes |
| Flow maps to block | yes | no `force_map_style` | no | reformats to `{ cpu: 50m }` | yes |
| Empty `{}`/`[]` retained | yes | yes | yes | yes | yes |
| Leading `---` round-trips per file | yes | global setting only | yes | no | yes |
| sops-aware | default skip | no | no | no | no |
| Still exists | yes | yes | yes | yes | removed in v5 |

Three cells are the reason to exist: flow-map normalisation (the majority of real violations in the motivating repository), per-file document-start preservation, and sops safety. yamlfmt reproduced a kustomize-emitted file byte for byte with two options set, and it does more than kustofmt does. If it grows the two missing features, most of the case for kustofmt goes away and the README will say so. An upstream contribution was considered; the kyaml-emission guarantee and the sops default made a standalone tool the cleaner answer.

## 9. Definition of done

For 0.1.0 (shipped 2026-08-20):

- All golden and property tests pass; the four fuzz targets run clean.
- `-w`, `-l`, `-d` and stdin mode each demonstrated on the golden inputs.
- The self-hosting check is green; CI is green on main.
- The release carries binaries for five targets, the multi-arch image, checksums, SBOMs and keyless signatures.
- The document set is complete and every comparison claim was verified against the competing tool.

State at 0.1.5 (2026-08-21), beyond 0.1.0: the compatibility matrix and watcher, five releases mapped to kustomize 5.6.0 through 5.8.1, the Sigstore bundle with self-verification in the release, the commit-subject check (2026-08-24), and the kustomize removal record cited in the README (2026-08-30). The provenance disclosure, a README section and the commit trailers, is the change this specification accompanies (2026-09-03).

## 10. Order of work

As built, from the git log: spike the kyaml round-trip and lock the style with golden tests; the diff; the CLI; output verification; test lanes, pinned linting and self-hosting; the document set; CI, release chain and image; a day of hardening from fuzzing and review (atomic writes, ownership, symlinks, encrypted input off stdout, allocation tuning); the compatibility matrix and the watcher, then the historical replay of kustomize 5.6.0 to 5.8.1 as 0.1.0 to 0.1.4; Dependabot; Windows CI; the signing bundle and self-verification as 0.1.5; the commit convention; the removal citation; provenance.

## 11. Departures from the build brief

The brief is the record of what was asked. Where the build did otherwise, the build is what this specification describes.

| The brief said | The build did | Why | Record |
|---|---|---|---|
| Use `kio/filters.FormatFilter` for the formatting pass; key order may follow kyaml's Kubernetes field ordering | Call the encoder directly; preserve key order always | `FormatFilter` sorts fields, which makes the first run a whole-file diff | `decisions/0002` |
| `internal/format/` | `format/`, a public package | The library is a stated feature and pkg.go.dev documents it | §6.5 |
| Test-only dependencies are fine (rapid, testing/quick) | None taken | A hand-rolled generator was enough; one requirement in `go.mod` is the claim | `decisions/0003` |
| Mutation testing in the full battery; coverage badge with a threshold | Neither; coverage goes to the run summary | The guards and fuzzers exercise the guarantees directly; two dead badges were removed rather than added to | `decisions/0013` |
| Renovate or Dependabot proposes kyaml bumps | A daily watcher owns kyaml; Dependabot ignores it | A Dependabot bump produces a pull request the compatibility gate can never pass | `decisions/0009` |
| A README mapping table of recent versions to kyaml | `compatibility.yaml` as the source of truth, generated tables, CI re-derivation, a tracking floor | A hand-maintained table cannot be proven | `decisions/0009` |
| Cosign is a should-have | Shipped in 0.1.0; bundle and self-verification in 0.1.5 | The detached pair could not be verified without Rekor | `decisions/0011` |
| Go Report Card badge | Removed | The service was retired | `decisions/0013` |
| Container usable with `-w` via a bind mount | Check mode only; the write idiom is a pipe | The image runs as uid 65532 and the correct `--user` depends on the daemon | `decisions/0010` |
| Conventional commits (the org default) | Go-style subjects | The version comes from a measurement made after the commit | `decisions/0012` |
| No AI attribution anywhere in the repo, never a co-author trailer | A README provenance section, and from 2026-09-03 a `Co-Authored-By` trailer on every generated commit; the earlier history left as it was | The repository is wholly machine-written and should say so, from the date it was decided | `decisions/0014` |

## 12. Open items

- Re-evaluate the block-style choice, and the course's approach with it, when KYAML is in common use (`decisions/0015`). The course's own YAML style decision should carry the same clause.
- Re-run `scripts/compare-tools.sh` against current yamlfmt when it releases, and update the courtesy paragraph if the two gaps close.
- The sibling tools (fleetnotes, fluxevents) follow the same shape and the same provenance disclosure.
