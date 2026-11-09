# One Go program for ImageSetConfiguration tools: plan

Status: **planned**, not started. `isc` is one Go program for building and editing oc-mirror v2 ImageSetConfigurations (ISCs), with one subcommand for each step of the pipeline it covers. This plan covers what the commands share: layout, behaviour, libraries, build and release, and build order. Each command's rules, and the oc-mirror / OLM sources they rely on, are in its own plan.

The tools were first planned in [oc-mirror-automation](https://github.com/tom-stockwell/oc-mirror-automation). `isc` is written from scratch here.

## Summary

| Command | Does | Status | Plan |
|---|---|---|---|
| `isc create --from-cluster` | one ISC per cluster, every installed operator pinned to its installed version | planned | [create-plan.md](create-plan.md) |
| `isc merge` | combines ISCs into one that mirrors at least what each of them does | planned | [merge-plan.md](merge-plan.md) |
| `isc operators optimise` | the smallest operator selection that still lets clusters upgrade | planned | [optimise-plan.md](optimise-plan.md) |

```
isc create --from-cluster [kubectl flags: --kubeconfig, --context, ...] [catalog flags] [-v]
isc merge [-v] FILE...
isc operators optimise [-o yaml|json|kyaml|diff|text|table] [--interactive] [--check] [--package NAME]... [catalog flags] [-v] FILE
isc version
```

Catalog flags: `--cache-dir DIR`, `--authfile FILE`, `--tls-verify=false`, `--platform OS/ARCH`, `--fetch-timeout SECONDS`.

`isc create` and `isc merge` work on the whole ISC, so they're top level. `isc operators optimise` only touches `mirror.operators`, so it sits under `isc operators`; a future command for `platform` or `additionalImages` would get its own group.

Pipeline:

1. `isc create --from-cluster > cluster-a.yaml` for each cluster (kubectl's current context, or `--context`).
2. `isc merge cluster-*.yaml > merged.yaml`.
3. Optionally, your own changes, outside isc: other tools or hand edits (see [Scope](#scope)).
4. `isc operators optimise merged.yaml > isc.yaml`.
5. `oc-mirror --v2 -c isc.yaml ...`.

## Plan / Design decisions

### Scope

isc reads and writes **plain oc-mirror ISCs only**. Changing an ISC beyond what these commands do (adding or removing operators, overriding fields, copying a catalog's packages to another version) is left to other tools or hand edits. Because isc loads ISCs as oc-mirror does (see [Loading like oc-mirror](#loading-like-oc-mirror)), anything oc-mirror rejects, such as a `metadata` block, is an error rather than silently passed on.

### Layout

One module, `github.com/ayjM6/isc`, one binary, at the root of this repo.

```
go.mod                      module github.com/ayjM6/isc
main.go                     calls cli.Execute()
internal/cli/               one cobra command per file: root.go, version.go, create.go, merge.go,
                            operators.go (the group), operators_optimise.go
                            (flags, usage, building the options; no logic)
internal/yamlnode/          yaml.v3 node helpers, flatten.go, encode.go (2-space encoder)
internal/isc/               load.go + types.go: loading and checking like oc-mirror (below);
                            Document: an ISC as a yaml.v3 node tree with typed accessors
internal/catalog/           digest lookup, OCI copy, configs extraction, digest-keyed cache
internal/create/
internal/merge/
internal/optimise/          with internal/olm/, internal/mirrorfilter/, internal/versions/ (optimise plan)
testdata/{create,merge,optimise,catalogs}/   fixtures (Go ignores testdata/ when building)
```

- **Why one program:** the commands share the loader, the YAML node helpers, reference parsing, semver ordering and catalog access (optimise, and create for ClusterExtensions). One module means one `go.mod` to keep in step with oc-mirror's pins, and one build cache.
- **`internal/`** so nothing outside the module depends on the packages before their APIs settle.

### Shared behaviour (every command)

- **Command line:** `github.com/spf13/cobra`, as oc-mirror uses. Long flags take `--` only; `-o` and `-v` are the only short forms.
- **Input:** no `-i` flag. A command that needs an input file takes it as the first positional argument (`merge` takes several; `create` takes none).
- **Output:** a command that outputs an ISC writes it to **stdout**. There's no output-file flag and no in-place writing, so the input is never modified.
  - `-o FORMAT` exists only on a command that can output different formats (so far: optimise).
  - The output is built and checked in memory and written to stdout only on success: a failure writes nothing to stdout.
- **YAML:** `gopkg.in/yaml.v3` nodes, so comments and key order are kept where a command edits an existing ISC. Output is encoded with 2-space indent.
- **Loaded like oc-mirror:** every input is loaded and checked the way oc-mirror loads an ISC before any work starts, and every output is checked the same way before it's written. isc never accepts or outputs an ISC that oc-mirror's loader would reject. See [Loading like oc-mirror](#loading-like-oc-mirror).
- **Outputs** are complete ISCs (`apiVersion: mirror.openshift.io/v2alpha1`, `kind`, `mirror`).
- **Messages:** on stderr, one line each: `review` lines always, decisions only with `-v`. Errors as `error: ...`, naming the file (and package or field) involved. Interactive prompts also go to stderr, with answers read from the terminal.
- **Exit codes:** 0 success, 1 runtime error, 2 usage error, 3 `--check` found changes (optimise only).
- **Determinism:** the same inputs give the same output bytes.
- **Catalog access:** shared by optimise and create, done the way oc-mirror does it; see [optimise-plan.md](optimise-plan.md#catalog-access). Same flags in both: `--cache-dir` (default `${XDG_CACHE_HOME:-~/.cache}/isc`), `--authfile`, `--tls-verify`, `--platform`, `--fetch-timeout`.
- **Libraries, not binaries:** no `opm`, `oc`, `skopeo` or `oc-mirror` subprocesses. oc-mirror's own helpers are under `internal/` and can't be imported, so the module uses the libraries they're built on:

| Need | Library |
|---|---|
| Catalog references (parse, normalize, pin) | `go.podman.io/image/v5/docker/reference` |
| Digest lookup, copying a catalog to an OCI layout | `go.podman.io/image/v5` |
| Reading the OCI layout and extracting the configs layer | `github.com/google/go-containerregistry` |
| Catalog model | `github.com/operator-framework/operator-registry` v1.74.0 (`alpha/declcfg`, `alpha/model`, `alpha/property`) |
| oc-mirror's operator filter | `github.com/sherine-k/catalog-filter` (the version oc-mirror pins) |
| Loading like oc-mirror; KYAML output | `sigs.k8s.io/yaml` v1.6.0+ (`UnmarshalStrict`, `YAMLToJSON`, `kyaml`), `encoding/json` (`DisallowUnknownFields`) |
| Version ordering, `skipRange` | `github.com/blang/semver/v4` (`ParseTolerant`, `Compare`, `ParseRange`) |
| Filter version semantics | `github.com/Masterminds/semver/v3` |
| Kubeconfig loading and kubectl's flags (create) | `k8s.io/cli-runtime` (`genericclioptions.ConfigFlags`) |
| Cluster reads (create) | `k8s.io/client-go` dynamic client, `k8s.io/apimachinery` |

### Loading like oc-mirror

oc-mirror's loader is in `internal/pkg/config` and can't be imported, so `internal/isc` repeats its steps, citing the source (`load.go` `ReadConfig` / `LoadConfig`, oc-mirror `upstream/main` at `4802bb8d`):

1. `sigs.k8s.io/yaml.UnmarshalStrict` into a map (rejects malformed YAML, e.g. duplicate keys);
2. `kind` present and equal to `ImageSetConfiguration`; a `mirror` stanza present; no `delete` stanza;
3. `sigs.k8s.io/yaml.YAMLToJSON`, then `encoding/json` with `DisallowUnknownFields` into `types.go`: our own copy of the v2alpha1 field set (`internal/pkg/api/v2alpha1`: `TypeMeta`, `mirror` with `platform`, `operators`, `additionalImages`, `helm`, `blockedImages`, `samples`, and `archiveSize`). Any unknown field fails, including `metadata`;
4. the duplicate-catalog rule from `validate.go` (`validateOperatorOptions`, key from `GetUniqueName`).

- Errors keep oc-mirror's wording where it has one, prefixed with the file name.
- Only these rules are repeated. The rest of `Validate` (platform and other field rules) isn't; `oc-mirror --v2 --dry-run` remains the final check.
- The typed struct is only for checking. Reading and editing still use yaml.v3 nodes, so comments and key order are kept.
- `types.go` follows oc-mirror's field set; check it against oc-mirror when moving the pins, and name the oc-mirror commit it matches in a comment.

### Build and release

- **justfile:**
  - `just build`: `go build` with the version ldflags, into `dist/` (git-ignored);
  - `just test`: `gofmt -l .` must print nothing, then `go vet ./...` and `go test ./...`;
  - `just run SUBCOMMAND ...`: `go run .` with the arguments, `[no-cd]` so relative paths work.
- **Version:** `isc version` and `isc --version` print `main.version`, set with `-ldflags "-X main.version=..."`; `dev` when unset. With `go install`, fall back to `debug.ReadBuildInfo()`'s module version. Also printed: the oc-mirror version and OLM commit the behaviour matches (`docs/compatibility.md`).
- **CI (GitHub Actions):** on every push and pull request, `just test` (or the same three commands) with the Go version from `go.mod`.
- **Releases (GitHub Actions + goreleaser):** pushing a `v*` tag runs goreleaser with `.goreleaser.yaml` from the repo:
  - `CGO_ENABLED=0`, linux/amd64 and linux/arm64 only (no macOS or Windows builds);
  - a `.tar.gz` per target with `LICENSE` and `README.md`, plus `checksums.txt`;
  - the version ldflags set from the tag;
  - published as a GitHub release.
- **Install:** `go install github.com/ayjM6/isc@latest`, or a release tarball for hosts without Go (e.g. the disconnected side). README documents both.

### Build order

Each step is one commit, or a short series, and leaves `just test` passing. README updates land with each step, not at the end.

1. **Skeleton:** `go.mod`, `main.go`, cobra root and `isc version`, justfile, CI workflow, `.goreleaser.yaml` and release workflow, `.gitignore`.
2. **Loader and documents:** `internal/isc` (loading like oc-mirror) and `internal/yamlnode`.
3. **`isc operators optimise`,** following its plan's build steps; catalog access comes first.
4. **`isc merge`.**
5. **`isc create --from-cluster`,** OLM v0 first, then OLM v1.
6. **First tagged release.**
7. **Move the practices into `AGENTS.md`:** once the commands are built, move the consistent practices that proved out (input and output, loading like oc-mirror, messages, exit codes, catalog access, testing patterns) from this plan into `AGENTS.md`, as instructions for keeping commands consistent. Until then, this plan's shared behaviour is the source of truth.

## Open questions (proposed answers)

1. **Reading clusters in Go or Ansible?** Decided: Go, as `isc create --from-cluster`, because the ClusterExtension catalog choice needs the catalog access optimise has. An Ansible role can still call it per inventory host.
2. **Dependency weight:** operator-registry already brings client-go, so create's dynamic client and cli-runtime add little. Proposed: accept it; check `go mod why` after step 5, and record the release binary size per target.

## Verification

- **Static:** `gofmt -l` empty, `go vet ./...`.
- **Unit and fixture tests** (`go test ./...`), no network or cluster, as listed in each command's plan.
- **Shared:** a failure writes nothing to stdout and exits 1; usage errors exit 2; the input file is unchanged after every command; the same inputs give the same output bytes.
- **Loader:** each step's rejection (malformed YAML and a duplicate key, missing or wrong `kind`, missing `mirror`, a `delete` stanza, an unknown field at top level and nested, `metadata`, a duplicate catalog), each naming the file; a valid ISC with every v2alpha1 section accepted; the cases in oc-mirror's `load_test.go` and the duplicate cases in `validate_test.go` give the same accept/reject result.
- **CI** green on the pull request.
- **Release dry run:** `goreleaser release --snapshot --clean` builds both targets locally; `isc version` on each built binary (arm64 under emulation or on an arm64 host) prints the snapshot version.
- **End to end (connected session):** `isc create --from-cluster` on two real clusters (read-only), `isc merge`, a hand-made change (an added and a removed operator), `isc operators optimise` against `registry.redhat.io`, then `oc-mirror --v2 --dry-run` on the result.

## What was and wasn't run

Planning only. This was written from the earlier plans in oc-mirror-automation and the decisions made since. No code exists yet, and nothing was built or run.
