# kustofmt decision records

One decision per file, Nygard's sections (Context, Decision, Consequences) plus MADR's considered options and a closing "where it is enforced" section; template in `TEMPLATE.md`. Records number from 0001 and are cited as `decisions/NNNN`. Accepted records are immutable except the status line; a change is a superseding record. Dates are the dates the git log or the build brief records. At most 120 lines each, honest negatives required, readable in a plain checkout.

The requirements they back are in [the specification](../specs/kustofmt.md).

| N | Decision | Status | Date | Enforced by |
|---|---|---|---|---|
| [0001](0001-name-the-tool-kustofmt.md) | Name the tool kustofmt, not kyamlfmt | accepted | 2026-08-20 | README "Not that KYAML"; module path |
| [0002](0002-use-the-encoder-and-preserve-key-order.md) | Call kyaml's encoder directly and preserve key order | accepted | 2026-08-20 | `formatOnce`, `blockify`; golden corpus |
| [0003](0003-one-direct-dependency.md) | One direct dependency: kyaml and the standard library | accepted | 2026-08-20 | `go.mod`; `make tidy-check` |
| [0004](0004-the-output-style-is-the-public-api.md) | The output style is the public API; the golden corpus decides the version | accepted | 2026-08-20 | `TestGolden`; `cmd/compat apply` |
| [0005](0005-no-config-no-options-no-ignore-file.md) | No config file, no style options, no ignore file | accepted | 2026-08-20 | the flag set; README non-goals |
| [0006](0006-skip-sops-files-by-default.md) | Skip sops-encrypted files by default, detected structurally | accepted | 2026-08-20 | `IsSOPS`; two fixtures |
| [0007](0007-format-verifies-its-own-output.md) | Format verifies its own output before returning it | accepted | 2026-08-20 | `Format`; fuzz seeds |
| [0008](0008-a-gofmt-shaped-cli.md) | A gofmt-shaped CLI with gofmt's exit codes, and atomic writes | accepted | 2026-08-20 | `cmd/kustofmt/run.go` |
| [0009](0009-one-release-per-kyaml.md) | One release per kyaml, mapped to kustomize releases, owned by a watcher | accepted | 2026-08-20 | `compatibility.yaml`; the watcher |
| [0010](0010-static-binary-scratch-image-non-root.md) | A static binary, a scratch image running non-root, a pipe for writes | accepted | 2026-08-20 | `Dockerfile`; `.goreleaser.yaml` |
| [0011](0011-keyless-signing-sigstore-bundle.md) | Keyless cosign; bundle for blobs, classic for the image; the release verifies itself | accepted | 2026-08-21 | `release.yaml`; `internal/release` |
| [0012](0012-go-style-commit-subjects.md) | Go-style commit subjects, not Conventional Commits | accepted | 2026-08-24 | `scripts/check-commit-subject.sh` |
| [0013](0013-ci-is-a-backstop.md) | CI is a backstop: Makefile targets, tiered tests, no third-party reporting | accepted | 2026-08-20 | `Makefile`; `ci.yaml` |
| [0014](0014-disclose-ai-provenance.md) | State AI authorship in the README, and from this decision on as a co-author trailer on every generated commit | accepted | 2026-09-03 | README "Provenance"; the trailers |
| [0015](0015-block-style-now-revisit-with-kyaml.md) | Emit block style now, and re-evaluate when KYAML is in common use | accepted | 2026-09-03 | README "Not that KYAML"; the course's style gate |
