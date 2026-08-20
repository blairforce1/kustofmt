# kustofmt

[![CI](https://github.com/blairforce1/kustofmt/actions/workflows/ci.yaml/badge.svg)](https://github.com/blairforce1/kustofmt/actions/workflows/ci.yaml)
[![Release](https://github.com/blairforce1/kustofmt/actions/workflows/release.yaml/badge.svg)](https://github.com/blairforce1/kustofmt/actions/workflows/release.yaml)
[![Go Reference](https://pkg.go.dev/badge/github.com/blairforce1/kustofmt.svg)](https://pkg.go.dev/github.com/blairforce1/kustofmt)
[![Go Report Card](https://goreportcard.com/badge/github.com/blairforce1/kustofmt)](https://goreportcard.com/report/github.com/blairforce1/kustofmt)
[![License](https://img.shields.io/badge/license-Apache--2.0-blue)](LICENSE)

**gofmt for GitOps YAML — kustomize's emitted style as a formatter.**

```yaml
# before                                  # after
spec:                                     spec:
  containers:                               containers:
    - name: app                             - name: app
      resources:                              resources:
        requests: {cpu: 50m}                    requests:
      ports:                                      cpu: 50m
        - containerPort: 8080                   ports:
                                                - containerPort: 8080
```

## Why this exists

`kustomize cfg fmt` used to format YAML into kustomize's canonical style. It was
removed in kustomize v5. The machinery survived as a library — `sigs.k8s.io/kustomize/kyaml`
— but the command did not, and the gap it left is a real one.

kustomize and Flux both *emit* a single house style: two-space map indentation,
**indentless sequences** (the dash sits at the same column as its key), block
style throughout. The tools people actually edit YAML with fight that style.
`yq` re-indents every sequence in any file it touches. `prettier` cannot produce
indentless sequences at all. The result is a GitOps repository where half the
YAML is written by machines in one style, and hand-edits quietly drift into
others, and every review carries a little whitespace noise that nobody asked for.

kustofmt closes the gap: **a formatter that re-emits YAML exactly as the
kustomize ecosystem's own marshaller would**, shipped as a single static binary
with a `gofmt`-shaped CLI. It is built for git pre-commit hooks, CI style jobs,
and format-on-save.

The style is not invented here, and that is the point. kustofmt uses kyaml's own
encoder, configured the way kyaml configures it. When kyaml's emission changes,
kustofmt's output changes with it — by construction, not by us chasing it.

## Install

**Released binary** — from the [releases page](https://github.com/blairforce1/kustofmt/releases),
or:

```sh
go install github.com/blairforce1/kustofmt/cmd/kustofmt@latest
```

**Container** — useful as a version-pinned CI filter:

```sh
docker run --rm -v "$PWD:/data:ro,z" -w /data ghcr.io/blairforce1/kustofmt:latest -l .
```

Archives, the image, checksums and SBOMs are signed with
[cosign](https://docs.sigstore.dev/) using keyless GitHub OIDC. To verify:

```sh
cosign verify-blob --certificate checksums.txt.pem --signature checksums.txt.sig \
  --certificate-identity-regexp 'https://github.com/blairforce1/kustofmt/.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt
```

## Usage

```
kustofmt [flags] [path ...]
```

Paths are files or directories; directories are walked recursively for `*.yaml`
and `*.yml`. With no paths, kustofmt reads stdin and writes stdout, so it
composes in a pipe.

| Flag | Effect |
|------|--------|
| `-l` | List files whose formatting differs. **Exits 1 if any.** This is the check mode for hooks and CI. |
| `-w` | Write the result back to the file. |
| `-d` | Print unified diffs instead of rewriting. |
| `--include-sops` | Format sops-encrypted files instead of skipping them. |
| `-version` | Print the version, and the kyaml release that defines the style. |

Exit codes: `0` clean or succeeded, `1` files need formatting (`-l`), `2` an
operational error — an unreadable file, a parse failure. A parse failure names
the file and does not stop the walk, so one broken file cannot hide the state of
everything after it.

### In a pre-commit hook

```sh
#!/bin/sh
# .git/hooks/pre-commit
files=$(git diff --cached --name-only --diff-filter=ACM -- '*.yaml' '*.yml')
[ -z "$files" ] && exit 0
if ! kustofmt -l $files; then
    echo "YAML above is not in house style. Run: kustofmt -w $files"
    exit 1
fi
```

### In CI

```yaml
- name: Check YAML formatting
  run: docker run --rm -v "$PWD:/data:ro,z" -w /data ghcr.io/blairforce1/kustofmt:v0.1.0 -l .
```

### As a library

```go
import "github.com/blairforce1/kustofmt/format"

out, err := format.Format(src)     // apply house style
skip := format.IsSOPS(src)         // encrypted? leave it alone
d := format.Diff("a", "b", src, out)
```

## The style contract

Output is byte-for-byte what kyaml emits:

- Two-space map indentation.
- **Sequence items are not indented** relative to their key.
- **Block style everywhere** — flow maps and sequences (`{a: b}`, `[x, y]`) are
  converted to block, *except* genuinely empty collections (`{}`, `[]`), which
  have no block form and stay as they are.
- **Comments are preserved**, attached to the nodes they were written on.
- **Scalar quoting is preserved.** `"1.36"` stays a quoted string and never
  decays into a float. Strings are never re-wrapped.
- **Key order is preserved**, always. kustofmt is a style formatter, not a
  linter; it will not reorder your fields, alphabetise your labels, or move
  `apiVersion` to the top. What you wrote is what you get, restyled.
- Multi-document files are preserved, and **a leading `---` round-trips per
  file** — Flux exports keep theirs, kustomize output stays bare.
- Anchors, aliases and merge keys (`<<:`) survive untouched.

Guarantees, each backed by tests:

- **Idempotent.** `fmt(fmt(x)) == fmt(x)`, byte for byte.
- **Semantics-preserving.** Input and output decode to identical values.
- **Compatible.** Files freshly emitted by `kustomize build`, and Flux
  `Kustomization` exports, pass `-l` untouched. Both are golden fixtures
  captured from real output.

kustofmt checks the last two on **every file it formats**, not just in tests. If
the result would not decode to the same values as the input, or is not a fixed
point, kustofmt refuses to write it and tells you why. This is not paranoia; see
[Known limitations](#known-limitations).

## sops safety, on by default

A file encrypted with [sops](https://github.com/getsops/sops) carries a MAC
computed over its whole document structure. Reformatting it invalidates the MAC
and the file can no longer be decrypted. kustofmt detects encrypted files and
**skips them with a notice**:

```
kustofmt: clusters/prod/secrets/db.secret.yaml: sops-encrypted, skipped (use --include-sops to override)
```

Detection is structural, not textual. A file counts as encrypted if it carries a
`sops` **mapping** with a MAC and a key source, or contains `ENC[AES256_GCM,...]`
payloads. That distinction matters: a version-pin file with a top-level

```yaml
sops: "3.11.0"
```

is not encrypted, and skipping it would be a confusing silent no-op. That exact
file, from the repository that motivated this tool, is a committed test case.

## Compared with

Verified empirically on 2026-08-20 against yamlfmt v0.21.0, yq v4.53.2 and
prettier 3.9.6 — not from memory. Re-run it yourself with
[`scripts/compare-tools.sh`](scripts/compare-tools.sh), which is the source of
this table.

| | kustofmt | [yamlfmt](https://github.com/google/yamlfmt) | [yq](https://github.com/mikefarah/yq) | prettier | `kustomize cfg fmt` |
|---|---|---|---|---|---|
| Indentless sequences | ✅ | ✅ with `indentless_arrays` | ❌ always re-indents | ❌ no option exists | ✅ |
| Flow **arrays** → block | ✅ | ✅ with `force_array_style` | ❌ | ❌ | ✅ |
| Flow **maps** → block | ✅ | ❌ no `force_map_style` | ❌ | ❌ reformats to `{ cpu: 50m }` | ✅ |
| Empty `{}`/`[]` retained | ✅ | ✅ | ✅ | ✅ | ✅ |
| Leading `---` round-trips **per file** | ✅ | ❌ global setting only | ✅ | ❌ | ✅ |
| sops-aware | ✅ default skip | ❌ breaks the MAC | ❌ | ❌ | ❌ |
| Still exists | ✅ | ✅ | ✅ | ✅ | ❌ removed in v5 |

Three of those cells are this tool's entire reason to exist: flow-map
normalisation, per-file document-start preservation, and sops safety. In the
repository that motivated kustofmt, flow maps were the majority of real
violations.

**On yamlfmt specifically:** it is good software that made different choices,
and it does more than kustofmt does — multiple formatters, configuration,
line-break retention. With `indentless_arrays: true` and
`force_array_style: block` it reproduced a kustomize-emitted file byte for byte
in our testing. If it grows `force_map_style` and per-file document-start
preservation, most of the case for kustofmt goes away, and this section will say
so rather than pretend otherwise. An upstream contribution was considered; the
kyaml-emission guarantee and the sops default made a standalone tool the cleaner
answer.

## Not that KYAML

Since Kubernetes v1.34 (beta, default-on in v1.35), **KYAML** is an official
Kubernetes *output encoding*: a stricter YAML subset that
[uses flow style with `{}` and `[]`, and double-quotes every string](https://kubernetes.io/docs/reference/encodings/kyaml).
It exists to make hand-authored YAML harder to get wrong — no surprise
type coercion, no indentation ambiguity.

That is the deliberate opposite of what kustofmt emits, which is why this tool
is not called `kyamlfmt`. A formatter that *removes* flow style and *preserves*
existing quoting would confuse everyone who met a name like that.

The two serve different problems. KYAML optimises for humans writing YAML
safely. kustofmt serves repositories whose YAML is *written by* kustomize and
Flux, where the emitters define the style and the job is keeping hand-edits
consistent with them. **Emitting KYAML is a non-goal** — `kubectl -o kyaml`
already does that.

(The `sigs.k8s.io/kustomize/kyaml` *library* this tool depends on is unrelated
to the encoding. It predates it by six years and remains the correct dependency.)

## Non-goals

Restraint is a feature. kustofmt will not:

- Emit KYAML. `kubectl -o kyaml` is that tool.
- Read a config file. There isn't one.
- Offer style options. The house style **is** the product; a formatter with
  knobs is a linter with regrets.
- Validate against schemas, convert YAML 1.1 to 1.2, or alphabetise your maps.
- Expand anchors or aliases — they are preserved as written.
- Touch the network. Ever.

## Known limitations

Stated plainly, because a formatter you cannot trust is worse than no formatter.

**Folded scalars containing an indented line are refused.** Given

```yaml
key: >
  one
   two
```

the underlying YAML emitter re-emits a value with an extra newline in it —
silently changing your data. kustofmt detects this, refuses to write the file,
and suggests rewriting the scalar as a literal (`|`), which has no such
ambiguity. Ordinary folded scalars are formatted normally. This was found by
fuzzing, not by a user, and it is the reason `Format` verifies its own output.

**Folded scalars are re-flowed.** `>` folds line breaks into spaces by
definition, so the emitter re-joins them at its own width. The value is
unchanged; the source lines move. Literal (`|`) scalars are untouched.

**Blank lines between keys are removed.** Machine-emitted YAML does not have
them, and preserving them is not something the emitter offers.

**Parse-error line numbers can be one line early.** They come from the YAML
parser as-is. Indentation errors are reported accurately; an unterminated flow
collection (`[1, 2` with no `]`) is reported one line before the bracket. The
offset differs by error class, so correcting it would make the accurate cases
wrong. The filename is always right.

**CRLF input becomes LF.** A documented choice: mixed line endings are a
formatting inconsistency like any other, and LF is what the ecosystem emits.

**The binary is about 12 MB.** kustofmt has exactly **one direct dependency**,
kyaml — but kyaml brings 13 transitive modules of its own, including
kube-openapi and protobuf. That is the honest number. Taking a smaller
dependency would mean *replicating* kyaml's emission rather than *using* it, and
the whole value proposition here is that the style cannot drift from the
ecosystem it claims to match. The size is the price of that guarantee, and it is
stated here rather than buried.

## Version compatibility

The style is defined by kyaml, so each release records which kyaml it was built
against. `kustofmt -version` prints it too.

| kustofmt | kyaml | Equivalent kustomize CLI |
|----------|-------|--------------------------|
| 0.1.0 | v0.21.1 | v5.7.1 |

A kyaml bump that changes emitted style is a **breaking change** for kustofmt
and is versioned accordingly. Semver from day one; the style contract is the API.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md). `make ci` runs everything the pipeline
runs. Golden test cases and reproducing inputs for bugs are the most useful
contributions.

## License

[Apache-2.0](LICENSE), matching the kustomize ecosystem. See [NOTICE](NOTICE).
