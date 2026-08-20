# Changelog

All notable changes to this project are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

**The output style is the public API.** A change that alters what kustofmt
emits is a breaking change and is versioned as one, even when the code change
is small.

## [Unreleased]

## [0.1.0] - unreleased

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

### Compatibility

| kustofmt | kyaml | kustomize CLI equivalent |
|----------|-------|--------------------------|
| 0.1.0 | v0.21.1 | v5.7.1 |
