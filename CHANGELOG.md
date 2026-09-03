# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

**The output style is the public API.** A change that alters what kustofmt
emits is a breaking change and is versioned as one, even when the code change
is small.

## [Unreleased]

### Added

- `make commit-check` validates commit subjects against the repository's
  convention, and `make hooks` installs the same check as a `commit-msg` hook.
  `scripts/check-commit-subject.sh` is the single definition both use, so a
  local verdict and a CI verdict cannot disagree.

### Changed

- README.md states the project's provenance: the code, tests, workflows and
  documents were written by Claude (Claude Opus 5, in Claude Code) from a
  brief, with the design decisions made and reviewed by blairforce1. Nothing
  about the formatter changed.
- CONTRIBUTING.md documents the commit convention in full, and records why it
  is Go's `package: description` rather than Conventional Commits: the release
  version comes from `compatibility.yaml` and the bump size from the golden
  corpus, so a commit message has nothing to decide. Nothing about the
  formatter changed.

## [0.1.5] - 2026-08-21

Nothing about the formatter changed: the output is byte-identical to 0.1.4, and
this release is still built against kyaml v0.21.1. What changed is how the
release proves it is ours.

### Changed

- The release is signed into `checksums.txt.sigstore.json`, a Sigstore bundle,
  replacing the detached `checksums.txt.pem` and `checksums.txt.sig` pair. The
  bundle carries the transparency-log inclusion proof, so `cosign verify-blob`
  no longer searches Rekor and no longer fails when Rekor is unreachable. The
  README documents the new command; releases up to 0.1.4 keep the old pair and
  the old command.
- Container image signatures are unchanged and still verify with `cosign verify`.

### Added

- `make sign-check` runs the release's own cosign arguments, read out of
  `.goreleaser.yaml`, and fails if they do not write the file the config
  declares. It runs in CI, which previously executed no part of the release
  pipeline. A `Signing smoke` workflow covers the keyless half, on pull requests
  that touch signing and on demand.
- The release workflow verifies what it just published, using the command the
  README publishes, including with Rekor pointed at a dead port.
- `compatibility.yaml` carries an explicit `version`, so a release can be cut
  for a reason kyaml did not cause. This one is the first.

## [0.1.4] - 2026-08-20

### Changed

- Built against kyaml v0.21.1, the library shipped by kustomize 5.8.1.
  The golden corpus is unchanged, so this release emits byte-identical output
  to its predecessor; only the provenance differs.

## [0.1.3] - 2026-08-20

### Changed

- Built against kyaml v0.21.0, the library shipped by kustomize 5.8.0.
  The golden corpus is unchanged, so this release emits byte-identical output
  to its predecessor; only the provenance differs.

## [0.1.2] - 2026-08-20

### Changed

- Built against kyaml v0.20.1, the library shipped by kustomize 5.7.1.
  The golden corpus is unchanged, so this release emits byte-identical output
  to its predecessor; only the provenance differs.

## [0.1.1] - 2026-08-20

### Changed

- Built against kyaml v0.20.0, the library shipped by kustomize 5.7.0.
  The golden corpus is unchanged, so this release emits byte-identical output
  to its predecessor; only the provenance differs.

## [0.1.0] - 2026-08-20

### Added

- Initial release: a gofmt-shaped formatter that emits kustomize's house YAML
  style — two-space maps, indentless sequences, block style throughout, with
  genuinely empty collections (`{}`, `[]`) left in flow style.
- `-l`, `-w`, `-d` and stdin filter mode, with gofmt's exit-code conventions
  (`0` clean, `1` files need formatting under `-l`, `2` operational error).
- sops-encrypted files are detected structurally and skipped by default;
  `--include-sops` overrides.
- Key order is preserved. kustofmt is a style formatter and never reorders
  fields.
- A leading `---` round-trips per file: Flux exports keep theirs, kustomize
  output stays bare.
- Output is verified before it is returned — it must decode to the same values
  as the input and be a fixed point — which catches two defects in the
  underlying YAML emitter rather than writing them to disk.
- Files are replaced atomically, preserving mode and ownership.
- Built against kyaml v0.19.0, the library shipped by kustomize 5.6.0.

## Compatibility

Which kustomize releases each version matches, generated from
[`compatibility.yaml`](compatibility.yaml):

<!-- compat:begin -->
| kustofmt | kyaml | kustomize CLI |
|----------|-------|---------------|
| 0.1.0 | v0.19.0 | 5.6.0 |
| 0.1.1 | v0.20.0 | 5.7.0 |
| 0.1.2 | v0.20.1 | 5.7.1 |
| 0.1.3 | v0.21.0 | 5.8.0 |
| 0.1.4 | v0.21.1 | 5.8.1 |

The current release is **0.1.5**, built against kyaml v0.21.1. Each row is the
release that first linked that kyaml; later releases linking the same kyaml
emit identical output.
<!-- compat:end -->
