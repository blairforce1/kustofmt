# 0006. Skip sops-encrypted files by default, detected structurally

- **Status:** accepted
- **Date:** 2026-08-20
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

sops computes a MAC over the whole document structure. Reformatting an encrypted file invalidates the MAC and the file can no longer be decrypted. No other YAML formatter checks for this; the motivating repository had already exempted its secrets from yamllint for the same reason. Detecting by text ("contains `sops`") has a real false positive: a version-pin file with a top-level `sops: "3.11.0"`, which the motivating repository commits.

## Decision

Encrypted files are skipped with a notice on stderr that names the file and the override. `--include-sops` formats them. Detection is structural: a `sops` mapping carrying a MAC and a key source (`age`, `pgp`, `kms`, `gcp_kms`, `azure_kv`, `hc_vault`), or an `ENC[AES256_GCM,` payload. A file that does not parse is not classified; the parse error is reported instead. In stdin filter mode an encrypted document passes through byte-identically. In `-l` and `-d` modes encrypted content never reaches stdout. The version-pin file is a committed fixture.

## Considered options

- **Exclude patterns.** yamlfmt's answer. Relies on every repository naming its secrets consistently.
- **A textual match on `sops:` or `ENC[`.** Skips the version-pin file silently. A silent no-op on a file the user asked to format is worse than a wrong format.
- **Off by default, with `--skip-sops`.** The default is the point: the file is corrupted before anyone learns the flag.

## Consequences

- Easier: `kustofmt -w .` over a GitOps repository is safe on day one.
- Harder: a false negative in detection is a data-loss bug. SECURITY.md classes it as security-relevant.
- Harder: the notice appears on stderr on every run, which some hooks will find noisy.

## Where it is enforced

`IsSOPS` in `format/sops.go`; `format/sops_test.go`; the `sops-secret.yaml` and `sops-false-positive.yaml` fixtures; README "sops safety, on by default"; SECURITY.md threat model.
