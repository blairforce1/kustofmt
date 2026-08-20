# Contributing to kustofmt

Thanks for looking. This is a small tool with a deliberately small scope, so
the most useful contributions are usually golden test cases and bug reports
with a reproducing input.

## Build and test in three commands

```sh
make build      # static binary at ./kustofmt
make test       # full suite: -race, coverage, property tests
make ci         # everything CI runs, in the same order
```

`make help` lists every target. There is nothing CI does that these targets do
not, which is the point: if `make ci` is green locally, the pipeline should be
green too.

The lanes, when you want something faster or more thorough:

| Command | What it runs | When |
|---------|--------------|------|
| `make test-fast` | unit, golden and CLI tests (`-short`) | inner loop |
| `make test` | the above plus property tests, with `-race` | before pushing |
| `make fuzz` | the four fuzz targets, 30s each (`FUZZTIME=5m` to go longer) | before a release |
| `make release-check` | `make ci` plus fuzzing | cutting a tag |

## Adding a golden test case

Golden tests are the style contract. Adding one is two steps:

1. Write `format/testdata/<case>.input.yaml` containing the YAML that
   demonstrates the behaviour.
2. Run `make golden`, then **read the generated
   `format/testdata/<case>.golden.yaml`**.

That second step is the whole job. `-update` will happily bless wrong output,
so the golden file is only meaningful if a human has looked at it and agreed
that is what the tool should produce. Commit both files together.

Your case is automatically included in the idempotency, semantics-preservation,
no-comment-lost and diff-applies tests. You do not need to wire anything up.

**A change to an existing golden file is a breaking change.** The output style
is this tool's public API, so a style-affecting change is a major or minor
version bump, never a patch. Expect a golden-file diff in a pull request to get
close attention.

## Reporting a formatting bug

The most useful report is the smallest input that misbehaves, plus what you
expected. If you can, add it as a fuzz seed:

```sh
go test ./format/ -run FuzzIdempotent -fuzz FuzzIdempotent -fuzztime=60s
```

An input that the fuzzer finds is written to `format/testdata/fuzz/` and is
committed as a permanent regression test. Several of the current ones came from
exactly that loop.

## Pull requests

- Small and focused. One logical change per PR.
- Conventional prefixes on commits (`format:`, `cmd/kustofmt:`, `docs:`, `ci:`).
- Update `CHANGELOG.md` under `Unreleased` in the same PR.
- `make ci` green before you push.
- No new dependencies without discussing it first. The dependency list is a
  feature; see the README's note on what kyaml already costs.

## Tracking kustomize releases

kustofmt's output style is kyaml's emitter, so every release is built against
exactly one kyaml — the one a given kustomize CLI release ships. That mapping
lives in [`compatibility.yaml`](compatibility.yaml), and the tables in the
README and this changelog are generated from it.

The mapping is **resolved, never assumed**. `scripts/kyaml-for-kustomize.sh`
works it out two independent ways — full MVS resolution through the Go
toolchain, and the `go.mod` upstream published at that tag — and fails if they
disagree. Deriving it arithmetically does not work: of the ten kustomize v5
releases checked, four ship a kyaml whose version differs from their `api`
version, and v5.5.0 ships a kyaml *ahead* of its api.

```sh
make compat-status    # kustomize releases not yet recorded, and what each needs
make compat-check     # re-derive every row from upstream (needs network)
make compat-render    # regenerate the README and CHANGELOG tables
```

The file also carries a `version`, which is what the release workflow tags. It
is usually the newest row, and the watcher advances it automatically. Edit it by
hand only to release something kyaml did not cause — a packaging or signing
change — which has no row to add, because one row per kyaml is an invariant. It
may not fall behind the newest row; `compat check` refuses to load a matrix
where it has.

### What the watcher does

`.github/workflows/kustomize-watch.yaml` runs daily. It takes the oldest
unrecorded kustomize release, resolves its kyaml, and either:

- **records it** against an existing kustofmt release, if some version already
  links that kyaml — nothing is rebuilt and no version is cut, because the
  binary that already exists is provably the right one to pin; or
- **rebuilds** against the new kyaml and prepares the next version.

For a rebuild, the golden corpus decides the version. Unchanged means nothing
observable moved, so it is a patch. Changed means the output style moved — a
breaking change to this tool's public API — and the watcher **refuses**, exiting
non-zero rather than proposing it. That case needs a human:

```sh
go test ./format/ -run TestGolden        # read the diff
make golden                              # accept it, then read the diff again
go run ./cmd/compat apply <version> --allow-style-change
```

### Repository settings the watcher needs

`pull-requests: write` in the workflow is necessary but not sufficient. The
repository must also allow Actions to open pull requests, which is off by
default:

**Settings → Actions → General → Workflow permissions →
“Allow GitHub Actions to create and approve pull requests”**

or, equivalently:

```sh
gh api -X PUT repos/OWNER/REPO/actions/permissions/workflow \
  -f default_workflow_permissions=read \
  -F can_approve_pull_request_reviews=true
```

The default token permission stays `read`; each workflow widens it where it
needs to. Without the setting the watcher does all its work and then fails at
the last step with *“GitHub Actions is not permitted to create or approve pull
requests”*. Forks need it too.

### Reviewing a watcher pull request

1. Check the resolved kyaml against upstream's own `go.mod` — the PR body links
   to it at the exact tag.
2. Confirm the golden corpus is untouched. If it is not, the PR should not exist;
   the apply step is meant to have refused.
3. Approve the workflow run. Pull requests opened with `GITHUB_TOKEN` start in an
   approval-required state by design, which is the human checkpoint.
4. Merge. Merging tags the version and publishes the release.

## Signing

Releases are signed keylessly with cosign, using the GitHub OIDC identity of the
release workflow. The blob signature is a Sigstore bundle, so verification needs
no Rekor lookup; the container image signature stays in cosign's classic format,
because Flux's `verify.provider: cosign` reads that convention.

The whole release pipeline runs in one workflow that CI never executes, which is
how a signing config that produced no signature could reach a tag unnoticed.
Three things close that:

```sh
make sign-check       # run .goreleaser.yaml's own cosign arguments (needs cosign)
```

`internal/release` reads the arguments out of `.goreleaser.yaml` and the verify
command out of `README.md` rather than restating either, so a config or a
documented command that stops working fails here. It skips unless the cosign on
your `PATH` is the version `release.yaml` pins — a verdict from a different
cosign is not a verdict about the release. `make cosign-version` prints the pin.

Keyless signing cannot run on a pull request: it needs `id-token: write`, which
CI does not have. **Dispatch `Signing smoke` before merging any change to
signing**, and read the run: it signs with a real identity and then verifies
using the README's command verbatim, including with Rekor unreachable. It is
dispatch-only because each run writes a permanent entry to Sigstore's
public-good transparency log.

Two traps worth knowing:

- Pinning an action does not pin the tool it installs. `cosign-installer` v3
  installs cosign v2, v4 installs cosign v3, and cosign v3 removed the flags the
  old config used — accepting them, warning, and writing nothing. Every
  tool-installing action in `release.yaml` therefore pins its tool as well.
- GoReleaser's sign pipe never checks that the file it asked cosign for exists.
  It registers the artifact for upload either way, so a silent no-write surfaces
  as a failed upload after the tag is cut.

## Scope

Please read the non-goals in the README before proposing a feature. In
particular there is no configuration file and there are no style options: the
house style *is* the product. A formatter with knobs is a linter with regrets.

That is not a brush-off, it is what keeps the tool honest — but it does mean a
"could we add an option for..." issue will usually be closed with a link to
that section. Bug reports, golden cases and documentation fixes are always
welcome.
