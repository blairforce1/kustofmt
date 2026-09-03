# 0008. A gofmt-shaped CLI with gofmt's exit codes, and atomic writes

- **Status:** accepted
- **Date:** 2026-08-20
- **Deciders:** blairforce1; implemented by Claude Opus 5 in Claude Code
- **Supersedes:** none

## Context

The consumers are hooks, CI jobs and editors, all of which already know gofmt's shape. The exit codes are the contract those consumers branch on. A write interrupted mid-file leaves a truncated manifest in a repository whose reconciler will apply it.

## Decision

`kustofmt [flags] [path ...]`. No paths reads stdin and writes stdout. One path with no mode flag prints the result; several paths require `-l`, `-w` or `-d`, because concatenated documents on stdout are nonsense. `-l` lists files whose formatting differs and exits 1 if any: it is the gate. `-d` prints unified diffs and exits 0: it is the explanation, not the gate. `-w` writes in place. Exit codes: 0 clean or succeeded, 1 files need formatting under `-l`, 2 operational error. A parse failure names the file and position and does not stop the walk. `-h` prints usage on stdout and exits 0, so it pipes. `-w` writes to a temp file beside the target and renames it into place, copies mode and ownership, follows symlinks, and skips the write when nothing changed so a clean run touches no mtimes. CRLF input becomes LF. `run()` takes its streams as arguments so every path is tested in-process.

## Considered options

- **`-d` exits 1 like `-l`.** Two gates with different output shapes. One gate is enough, and `-d`'s output is for reading.
- **Overwrite in place.** Simpler, and one interrupted run leaves a half-written manifest.
- **`--check` instead of `-l`.** Not gofmt's spelling. The point of the shape is zero learning.

## Consequences

- Easier: the pre-commit hook is three lines and `xargs` composes.
- Harder: the atomic replace gives the file a new inode, which breaks hard links and leaves a process holding the old file reading old content. Stated in the README.
- Harder: parse-error line numbers come from the parser and are one line early for some error classes. Correcting them would make the accurate cases wrong.
- Harder: `xargs` reports a failing gate as 123, so callers branch on non-zero, not on 1.

## Where it is enforced

`cmd/kustofmt/run.go` and `run_test.go`; `cmd/kustofmt/owner_unix.go`; README "Usage" and "Known limitations".
