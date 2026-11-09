# Agent instructions

## Goal of this repo

`isc` is one Go program for building and editing oc-mirror v2 ImageSetConfigurations (ISCs), with one subcommand for each step of the pipeline it covers:

1. `isc create --from-cluster` for each cluster (kubectl's current context, or `--context`): one ISC per cluster, every installed operator pinned to its installed version.
2. `isc merge`: combine them into one ISC that mirrors at least what each does (every operator, lowest `minVersion` per catalog).
3. Optionally, your own changes, outside isc (other tools or hand edits).
4. `isc operators optimise`: the smallest operator selection that still lets clusters upgrade.
5. `isc validate`: check it's valid and will run with oc-mirror.
6. `oc-mirror --v2` on the result.

The design is in [docs/plans/](docs/plans/). Read the relevant plan before changing behaviour:
- [isc-tool-plan.md](docs/plans/isc-tool-plan.md): layout, shared behaviour, libraries, build and release, and the build order (what to do next);
- one plan per subcommand, holding its rules and the oc-mirror / OLM sources they rely on: [create](docs/plans/create-plan.md), [merge](docs/plans/merge-plan.md), [optimise](docs/plans/optimise-plan.md), [validate](docs/plans/validate-plan.md).

The tools were first planned in oc-mirror-automation; `isc` is written from scratch here.

## Layout

```
go.mod                  module github.com/ayjM6/isc
main.go                 calls cli.Execute()
internal/cli/           one cobra command per file: flags, usage, building options; no logic
internal/yamlnode/      yaml.v3 node helpers, flattening, 2-space encoder
internal/ocmirror/      pinned copy of oc-mirror code (config loading, validation, API types, filter conversion)
internal/isc/           ISC documents loaded with ocmirror/config: node tree plus typed view
internal/catalog/       digest lookup, OCI copy, configs extraction, digest-keyed cache
internal/{create,merge,optimise,validate}/   optimise also uses internal/{olm,mirrorfilter,versions}/
testdata/{create,merge,optimise,validate,catalogs}/
docs/plans/             plans, one per feature
```

Everything is under `internal/` until the APIs settle.

## Behaviour every command keeps

Until the commands are built, the shared behaviour (input and output, loading with oc-mirror's code, messages, exit codes, catalog access, libraries) is defined in the tool plan's [Shared behaviour](docs/plans/isc-tool-plan.md#shared-behaviour-every-command) section: follow it, and change it there first. It moves here once it has proved out (the tool plan's last build step).

- **Read-only against clusters:** `isc create --from-cluster` only lists and gets.
- **Never edit `internal/ocmirror/` by hand:** it's a pinned copy of oc-mirror's code; change the pin and run `just sync-ocmirror`.

## Go style

- **Formatting and vetting:** `gofmt` and `go vet ./...` clean (`just test`).
- **Errors:** return them, wrapped with context (`fmt.Errorf("%s: %w", path, err)`); only `internal/cli` decides exit codes and prints.
- **No global state:** pass options and dependencies (catalog source, dynamic client, prompt) in, so tests can substitute them.
- **Comments explain why, not what:** external behaviour, ordering constraints, oc-mirror or OLM rules.
- **Cite the source.** When code relies on how oc-mirror, operator-registry or operator-controller behaves, cite the file (and version) the behaviour comes from.
- **Moving code:** `git mv` first, edit in a later step, so history follows.

## Running and testing

- **Before calling a change done:**
  1. `just test` (`gofmt -l .` prints nothing, `go vet ./...`, `go test ./...`);
  2. exercise the failure paths too: invalid input, failed catalog fetch or lookup. Nothing may reach stdout, and the exit code must say what failed.
- **Tests need no network or cluster.** Use fixtures under `testdata/`, an injected catalog source, and `k8s.io/client-go/dynamic/fake` for `create`.
- **Fixture changes are behaviour changes.** Don't change an expected output to make a test pass without saying why.
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
