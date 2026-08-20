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

## Scope

Please read the non-goals in the README before proposing a feature. In
particular there is no configuration file and there are no style options: the
house style *is* the product. A formatter with knobs is a linter with regrets.

That is not a brush-off, it is what keeps the tool honest — but it does mean a
"could we add an option for..." issue will usually be closed with a link to
that section. Bug reports, golden cases and documentation fixes are always
welcome.
