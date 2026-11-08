# Agent instructions

## Goal of this repo

`isc` is one Go program for building and editing oc-mirror v2 ImageSetConfigurations (ISCs), with one subcommand for each step of the pipeline it covers:

1. `isc create --from-cluster` for each cluster (kubectl's current context, or `--context`): one ISC per cluster, every installed operator pinned to its installed version.
2. `isc operators merge`: every operator from every cluster, lowest `minVersion`, one entry per catalog per tag.
3. Optionally, your own changes, outside isc (e.g. kustomize in another project).
4. `isc operators prune`: flatten the ISC and remove the `minVersion`s the catalog makes unnecessary.
5. `oc-mirror --v2` on the result.

The design is in [docs/plans/](docs/plans/). Read the relevant plan before changing behaviour:
- [isc-tool-plan.md](docs/plans/isc-tool-plan.md): layout, shared behaviour, libraries, build and release, and the build order (what to do next);
- one plan per subcommand, holding its rules and the oc-mirror / OLM sources they rely on: [create](docs/plans/create-plan.md), [merge](docs/plans/merge-plan.md), [prune](docs/plans/prune-plan.md).

The tools were first planned in oc-mirror-automation; `isc` is written from scratch here.

## Layout

```
go.mod                  module github.com/ayjM6/isc
main.go                 calls cli.Execute()
internal/cli/           one cobra command per file: flags, usage, building options; no logic
internal/yamlnode/      yaml.v3 node helpers, flattening, 2-space encoder
internal/fileio/        atomic writes, same-path check
internal/isc/           loading and checking like oc-mirror; ISC accessors on a node tree
internal/catalog/       digest lookup, digest-keyed render cache, in-process render
internal/{create,merge,prune}/
testdata/{create,merge,prune}/
docs/plans/             plans, one per feature
```

Everything is under `internal/` until the APIs settle.

## Behaviour every subcommand keeps

These are requirements, not defaults. Changing one needs a plan change first.

- **Input is never modified.** Refuse `-o` if it resolves to an input (symlinks resolved), before reading anything.
- **Safe writing:** temp file in the destination directory, mode 0644, rename. On any error: exit non-zero, write nothing, leave an existing output as it was.
- **Comments and key order are kept:** work on `gopkg.in/yaml.v3` nodes, not structs. Encode with 2-space indent.
- **Loaded like oc-mirror:** every input, and every output before it's written, goes through `internal/isc`'s loader, which repeats oc-mirror's `ReadConfig` steps (strict YAML, kind and stanza checks, unknown fields rejected, duplicate catalogs). Never accept or write an ISC oc-mirror would reject.
- **Plain oc-mirror ISCs only:** `metadata` is an unknown field, so it's an error; nothing kustomize-specific is read or written.
- **Messages on stderr,** one line each: `review` lines always, decisions only with `-v`. Errors as `error: ...`, naming the file (and package or field).
- **Exit codes:** 0 success, 1 runtime error, 2 usage error.
- **Deterministic:** the same inputs give the same output bytes.
- **Libraries, not binaries:** no `opm`, `oc` or `skopeo` subprocesses. Use the libraries oc-mirror is built on (the plan's library table), at the versions oc-mirror pins.
- **Read-only against clusters:** `isc create --from-cluster` only lists and gets.

## Go style

- **Formatting and vetting:** `gofmt` and `go vet ./...` clean (`just test`).
- **Errors:** return them, wrapped with context (`fmt.Errorf("%s: %w", path, err)`); only `internal/cli` decides exit codes and prints.
- **No global state:** pass options and dependencies (render function, dynamic client) in, so tests can substitute them.
- **Comments explain why, not what:** external behaviour, ordering constraints, oc-mirror or OLM rules.
- **Cite the source.** When code relies on how oc-mirror, operator-registry or operator-controller behaves, cite the file (and version) the behaviour comes from.
- **Moving code:** `git mv` first, edit in a later step, so history follows.

## Running and testing

- **Before calling a change done:**
  1. `just test` (`gofmt -l .` prints nothing, `go vet ./...`, `go test ./...`);
  2. for anything that writes output, exercise the failure paths too: invalid input, failed render or lookup, output == input. Nothing may be written and an existing output must be untouched.
- **Tests need no network or cluster.** Use fixtures under `testdata/`, an injected catalog render, and `k8s.io/client-go/dynamic/fake` for `create`.
- **Fixture changes are behaviour changes.** Don't change an expected output to make a test pass without saying why.
- **`testdata/prune/` is prune's acceptance test.** Its fixtures were copied from oc-mirror-automation's `prune-minversion`; message wording may change, decisions must not.
- **Never run against a real cluster or registry unless asked,** and then read-only.
- **Don't delete state without asking:** caches, test data, outputs.
- **Report results faithfully.** Show failures with their output. If something wasn't run, say so.

## Docs and plans

- **New features:** write a plan in `docs/plans/<feature>-plan.md` first, on the feature branch. Sections:
  - Plan / Design decisions;
  - Open questions, each with a proposed answer;
  - Verification;
  - a note on what was and wasn't run.
- Keep a plan's status and the tool plan's summary table current as work lands.
- **README:** update it in the same change as the behaviour (usage, flags, the pipeline).

## Git

- **Commit subjects:** imperative, sentence case, no prefix, describing the behaviour change.
- **Branches:** one feature per branch, based on current `main`.
- **Each commit leaves `go vet` and `go test` passing.**
- **Commit, push or merge only when asked.**
- **Releases:** pushing a `v*` tag runs goreleaser and publishes a GitHub release. Never tag or release unless asked; check locally with `goreleaser release --snapshot --clean`.
