# 0011. Sign keylessly with cosign, bundle the blob signature, keep the image classic, and verify in the release

- **Status:** accepted
- **Date:** 2026-08-21 (keyless signing from 0.1.0; the bundle and the self-verification from 0.1.5)
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

The brief listed cosign as a should-have. A stored key is a key to rotate and to leak; GitHub OIDC gives the workflow an identity for free. The first releases signed `checksums.txt` into a detached certificate and signature pair, which carries no transparency-log proof: verification searches Rekor and fails when Rekor is unreachable, a poor property for a tool aimed at CI. Separately, `cosign-installer` v4 installs cosign v3, which removed the flags the old config used: it accepts them, warns, and writes nothing, and GoReleaser never checks that the file exists. A release could publish unsigned without failing.

## Decision

Archives, checksums, SBOMs and the image are signed keylessly against the release workflow's OIDC identity. From 0.1.5 the blob signature is a Sigstore bundle (`checksums.txt.sigstore.json`, `--new-bundle-format=true` stated explicitly), so `cosign verify-blob` needs no Rekor lookup. The image signature stays in cosign's classic format, because Flux's `verify.provider: cosign` reads that convention; moving it is a deliberate decision, never a side effect of a cosign upgrade. Every tool-installing action pins its tool as well as the action. The release job runs the README's verify command against what it just published, including with Rekor pointed at a dead port, and fails if it does not pass. `internal/release` reads the cosign arguments out of `.goreleaser.yaml` and the verify command out of `README.md`, so neither can drift, and skips unless the cosign on `PATH` is the pinned one. A `Signing smoke` workflow runs the keyless half on pull requests that touch signing files, scoped because each run writes a permanent entry to the public transparency log.

## Considered options

- **A stored signing key.** A secret in the one workflow that publishes.
- **Keep the detached pair.** Verifiable only while Rekor is reachable.
- **Bundle the image signature too.** Breaks Flux verification until Flux reads bundles.
- **No verification step in the release.** The published command could be wrong for the life of a release.

## Consequences

- Easier: the README's verify command is tested, not illustrative.
- Harder: releases up to 0.1.4 verify with the old command, so the README carries both.
- Harder: the CI workflow deliberately lacks `id-token: write`, so the keyless check lives in its own workflow.

## Where it is enforced

`signs` and `docker_signs` in `.goreleaser.yaml`; the pins and the "Verify the signature just published" step in `release.yaml`; `internal/release/sign_test.go` via `make sign-check`; `.github/workflows/signing-smoke.yaml`; CONTRIBUTING "Signing".
