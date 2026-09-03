# 0010. A static binary, a scratch image running non-root, and a pipe for writes

- **Status:** accepted
- **Date:** 2026-08-20 (the write idiom documented 2026-08-21, PR #8)
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

The consumer the tool was built for runs pure filters as pinned containers. The binary is static and makes no network call, so a base image would provide nothing: no libc, no CA bundle, no shell. A container that writes into a bind mount as root leaves root-owned files on the host. One that runs as a named user needs `/etc/passwd`, which scratch lacks.

## Decision

`CGO_ENABLED=0`, `-trimpath` and a commit-pinned `mod_timestamp`, so builds are reproducible and carry no local paths. The image is `FROM scratch` with the binary as entrypoint, multi-arch (linux/amd64, linux/arm64), running as numeric uid 65532. In the container only check mode works directly: `-w` fails with permission denied on a read-only mount and under most user mappings. The documented write idiom pipes each file through the container in filter mode and lets the host write (`cat`, not `mv`, to keep the file's mode and owner). Per-runtime `--user` flags are documented as fragile, not recommended. Archives ship for linux and darwin on amd64 and arm64, and windows on amd64. SBOMs are attached for the archives and the image.

## Considered options

- **A distroless base with a named user.** Adds a base image to track, for no capability the binary uses.
- **Run as root in the container.** Root-owned files in the user's checkout.
- **Support `-w` via a documented `--user`.** The correct value depends on the daemon (rootless docker and podman map the caller to container uid 0; rootful docker to the caller's own uid), and a wrong guess fails exactly like no flag.

## Consequences

- Easier: the image is the binary. The SBOM is nearly one line. The pin is a tag in the command.
- Harder: the README needs a paragraph on why `-w` does not work in the container and a loop to do it instead, found by the first consumer under podman and docker alike.
- Harder: the pipe idiom must never be combined with `-l` or `-d`, since redirecting a list or a diff over the file empties it. Stated.

## Where it is enforced

`Dockerfile`; `builds`, `archives`, `dockers_v2` and `sboms` in `.goreleaser.yaml`; README "Install"; the CI cross-compile matrix.
