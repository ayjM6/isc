# One Go program for ImageSetConfiguration tools: plan

Status: **planned**, not started. `isc` is one Go program for building and editing oc-mirror v2 ImageSetConfigurations (ISCs), with one subcommand for each step of the pipeline it covers. This plan covers what the commands share: layout, behaviour, libraries, build and release, and build order. Each command's rules, and the oc-mirror / OLM sources they rely on, are in its own plan.

The tools were first planned in [oc-mirror-automation](https://github.com/tom-stockwell/oc-mirror-automation). `isc` is written from scratch here.

## Summary

| Command | Does | Status | Plan |
|---|---|---|---|
| `isc create --from-cluster` | one ISC per cluster, every installed operator pinned to its installed version | planned | [create-plan.md](create-plan.md) |
| `isc merge` | combines ISCs into one that mirrors at least what each of them does | planned | [merge-plan.md](merge-plan.md) |
| `isc operators optimise` | the smallest operator selection that still lets clusters upgrade | planned | [optimise-plan.md](optimise-plan.md) |
| `isc validate` | checks an ISC is valid and will run with oc-mirror, and that clusters could upgrade | planned | [validate-plan.md](validate-plan.md) |
| `isc preview` | what an ISC will mirror: bundles, images, sizes, and a package's upgrade graph | planned | [preview-plan.md](preview-plan.md) |

```
isc create --from-cluster [kubectl flags: --kubeconfig, --context, ...] [catalog flags] [-v]
isc merge [-v] FILE...
isc operators optimise [-o yaml|json|kyaml|diff|text|table] [--interactive] [--check] [--package NAME]... [catalog flags] [-v] FILE
isc validate [--offline] [--strict] [catalog flags] [-v] FILE...
isc preview [-o summary|tree|table|json|yaml] [--with-sizes] [--depth N] [--package NAME]... [catalog flags] [-v] FILE
isc preview bundles|images|catalogs [-o text|json|yaml] ... FILE
isc preview operator-graph --package NAME [--catalog REF] [--channel NAME] [--min-version VERSION] [--mirrored-only] [-o text|dot|json|yaml] ... FILE
isc version
```

Catalog flags: `--cache-dir DIR`, `--authfile FILE`, `--tls-verify=false`, `--platform OS/ARCH`, `--fetch-timeout SECONDS`.

`isc create`, `isc merge`, `isc validate` and `isc preview` work on the whole ISC, so they're top level. `isc operators optimise` only touches `mirror.operators`, so it sits under `isc operators`; a future command for `platform` or `additionalImages` would get its own group.

Pipeline:

1. `isc create --from-cluster > cluster-a.yaml` for each cluster (kubectl's current context, or `--context`).
2. `isc merge cluster-*.yaml > merged.yaml`.
3. Optionally, your own changes, outside isc: other tools or hand edits (see [Scope](#scope)).
4. `isc operators optimise merged.yaml > isc.yaml`.
5. `isc validate isc.yaml`, and optionally `isc preview isc.yaml` to see what will be mirrored.
6. `oc-mirror --v2 -c isc.yaml ...`.

## Plan / Design decisions

### Scope

isc reads and writes **plain oc-mirror ISCs only**. Changing an ISC beyond what these commands do (adding or removing operators, overriding fields, copying a catalog's packages to another version) is left to other tools or hand edits. Because isc loads ISCs with oc-mirror's own code (see [Loading with oc-mirror's own code](#loading-with-oc-mirrors-own-code)), anything oc-mirror rejects, such as a `metadata` block, is an error rather than silently passed on.

### Layout

One module, `github.com/ayjM6/isc`, one binary, at the root of this repo.

```
go.mod                      module github.com/ayjM6/isc
main.go                     calls cli.Execute()
internal/cli/               one cobra command per file: root.go, version.go, create.go, merge.go,
                            validate.go, preview.go, operators.go (the group), operators_optimise.go
                            (flags, usage, building the options; no logic)
internal/yamlnode/          yaml.v3 node helpers, flatten.go, encode.go (2-space encoder)
internal/ocmirror/          pinned copy of oc-mirror code (below); never edited by hand
internal/isc/               Document: an ISC loaded with ocmirror/config, as a yaml.v3 node tree
                            plus the typed v2alpha1 view
internal/catalog/           digest lookup, OCI copy, configs extraction, digest-keyed cache
internal/create/
internal/merge/
internal/validate/          static, catalog and upgrade-path checks (validate plan); used by optimise
internal/preview/           selection, image collection, counts and sizes (preview plan); used by optimise and validate
internal/optimise/          with internal/olm/, internal/mirrorfilter/, internal/versions/ (optimise plan)
testdata/{create,merge,optimise,validate,preview,catalogs}/   fixtures (Go ignores testdata/ when building)
```

- **Why one program:** the commands share the loader and oc-mirror's code, the checks, the YAML node helpers, reference parsing, semver ordering and catalog access (optimise, and create for ClusterExtensions). One module means one `go.mod` to keep in step with oc-mirror's pins, and one build cache.
- **`internal/`** so nothing outside the module depends on the packages before their APIs settle.

### Shared behaviour (every command)

- **Command line:** `github.com/spf13/cobra`, as oc-mirror uses. Long flags take `--` only; `-o` and `-v` are the only short forms.
- **Input:** no `-i` flag. A command that needs an input file takes it as the first positional argument (`merge` takes several; `create` takes none).
- **Output:** a command that outputs an ISC writes it to **stdout**. There's no output-file flag and no in-place writing, so the input is never modified.
  - `-o FORMAT` exists only on a command that can output different formats (so far: optimise).
  - The output is built and checked in memory and written to stdout only on success: a failure writes nothing to stdout.
- **YAML:** `gopkg.in/yaml.v3` nodes, so comments and key order are kept where a command edits an existing ISC. Output is encoded with 2-space indent.
- **Loaded with oc-mirror's own code:** every input is loaded with oc-mirror's `ReadConfig` before any work starts, and every output is checked the same way before it's written. isc never accepts or outputs an ISC that oc-mirror would reject when loading it. See [Loading with oc-mirror's own code](#loading-with-oc-mirrors-own-code).
- **Outputs** are complete ISCs (`apiVersion: mirror.openshift.io/v2alpha1`, `kind`, `mirror`).
- **Messages:** on stderr, one line each: `review` lines always, decisions only with `-v`. Errors as `error: ...`, naming the file (and package or field) involved. Interactive prompts also go to stderr, with answers read from the terminal.
- **Exit codes:** 0 success, 1 runtime error (and, for validate, an invalid ISC), 2 usage error, 3 `--check` found changes (optimise only).
- **Determinism:** the same inputs give the same output bytes.
- **Catalog access:** shared by validate, preview, optimise and create, done the way oc-mirror does it; see [optimise-plan.md](optimise-plan.md#catalog-access). Same flags in both: `--cache-dir` (default `${XDG_CACHE_HOME:-~/.cache}/isc`), `--authfile`, `--tls-verify`, `--platform`, `--fetch-timeout`.
- **Libraries, not binaries:** no `opm`, `oc`, `skopeo` or `oc-mirror` subprocesses. oc-mirror's own helpers are under `internal/` and can't be imported, so the module uses the libraries they're built on:

| Need | Library |
|---|---|
| Catalog references (parse, normalize, pin) | `go.podman.io/image/v5/docker/reference` |
| Digest lookup, copying a catalog to an OCI layout, image manifests (preview sizes) | `go.podman.io/image/v5` |
| Reading the OCI layout and extracting the configs layer | `github.com/google/go-containerregistry` |
| Catalog model | `github.com/operator-framework/operator-registry` v1.74.0 (`alpha/declcfg`, `alpha/model`, `alpha/property`) |
| oc-mirror's operator filter | `github.com/sherine-k/catalog-filter` (the version oc-mirror pins) |
| oc-mirror's config loading, validation, API types and filter conversion | pinned copy in `internal/ocmirror/` (below), from `github.com/openshift/oc-mirror/v2` |
| KYAML output | `sigs.k8s.io/yaml` v1.6.0+ (`kyaml`) |
| Version ordering, `skipRange` | `github.com/blang/semver/v4` (`ParseTolerant`, `Compare`, `ParseRange`) |
| Filter version semantics | `github.com/Masterminds/semver/v3` |
| Kubeconfig loading and kubectl's flags (create) | `k8s.io/cli-runtime` (`genericclioptions.ConfigFlags`) |
| Cluster reads (create) | `k8s.io/client-go` dynamic client, `k8s.io/apimachinery` |

Build and release tools, not Go dependencies and never run by `isc` itself: `just`, git-cliff and goreleaser ([Release automation](#release-automation)).

### Loading with oc-mirror's own code

oc-mirror v2 has no public packages (everything is under `internal/`, `cmd/` or `v1/`), and Go refuses imports of another module's `internal/`. Rather than reimplement oc-mirror's behaviour, isc keeps a **pinned copy** of the parts it needs in `internal/ocmirror/`:

| Copy | From oc-mirror | How |
|---|---|---|
| `internal/ocmirror/config` | `internal/pkg/config`: `load.go`, `validate.go`, `defaults.go`, `metadata.go` (not `pin_catalogs.go`, which needs the mirror copier) | whole files |
| `internal/ocmirror/api/v2alpha1` | `internal/pkg/api/v2alpha1` | whole package |
| `internal/ocmirror/image`, `internal/ocmirror/consts` | `internal/pkg/image`, `internal/pkg/consts` | whole packages |
| `internal/ocmirror/operator/filter.go` | `filterPackage`, `filterFromImageSetConfig`, `filterCatalog` from `internal/pkg/operator/catalog_handler.go` (the rest of that file needs the mirror copier, logging and manifest packages) | functions, extracted by name |

- **Unmodified except import paths.** About 1,300 lines; their other imports (`sigs.k8s.io/yaml`, Masterminds semver, apimachinery, go-digest, operator-registry, catalog-filter) are dependencies anyway.
- **Sync:** `just sync-ocmirror REF` fetches the pinned module (`go mod download github.com/openshift/oc-mirror/v2@REF`), copies the files and functions, and rewrites import paths. CI re-runs it at the pinned commit and fails on any diff. Never edit `internal/ocmirror/` by hand: change the pin and re-sync.
- **Pin:** the oc-mirror commit (`4802bb8d` when planned), recorded in `internal/ocmirror/README.md` and `docs/compatibility.md`, and printed by `isc version`. Moving it means re-syncing and re-running the tests.
- **Licence:** oc-mirror is Apache-2.0. The copied files keep their headers; `internal/ocmirror/LICENSE` holds the Apache-2.0 text, and the README states the source, the commit and the import-path change.
- **Loading** is `ocmirror/config.ReadConfig`: strict YAML, `kind` and stanza checks, unknown fields rejected (including `metadata`), and all of `Validate` (duplicate catalogs, packages and channels; semver versions; package and channel ranges not mixed; duplicate release channels; `platform.platforms` with `platform.architectures`; blocked-image regexes). So every command accepts exactly what oc-mirror accepts.
- `internal/isc.Document` keeps the typed `v2alpha1.ImageSetConfiguration` for reading and the yaml.v3 node tree for editing, so comments and key order are kept.
- **Not copied:** oc-mirror's catalog extraction (`EnsureCatalogInOCIFormat`, `ExtractOCIConfigLayers`, `ExtractOCILayers`) depends on its mirror copier and archive packages, so `internal/catalog` implements the same steps, citing them ([optimise plan](optimise-plan.md#catalog-access)).
- **Upstream:** ask oc-mirror to expose its config, API types and filter conversion as public packages; then drop the copy.

### Build and release

- **Version:** `isc version` and `isc --version` print `main.version`, set with `-ldflags "-X main.version=..."`; `dev` when unset. With `go install`, fall back to `debug.ReadBuildInfo()`'s module version. Also printed: the oc-mirror version and OLM commit the behaviour matches (`docs/compatibility.md`), and the pinned `internal/ocmirror` commit.
- **Install:** `go install github.com/ayjM6/isc@latest`, or a release tarball for hosts without Go (e.g. the disconnected side). README documents both.

### Release automation

Releases come only from **semantic-version tags**, never from merges. A manual GitHub Action works out the next version, updates the changelog in a commit on `main` and tags it; the tag push builds and publishes the release. Go has no built-in release versioning, so **git-cliff** does both the version calculation and the changelog; **goreleaser** builds, packages and uploads.

#### Conventional Commits

- Commit subjects follow Conventional Commits: `feat:`, `fix:`, `docs:`, `refactor:`, `test:`, `build:`, `ci:`, `chore:`; breaking changes as `feat!:` / `fix!:` or a `BREAKING CHANGE:` footer. After the prefix: imperative, sentence case, describing the behaviour change.
- Checked in CI on pull requests: every commit in the range (`just check-commits`) and the PR title (`just check-title`; squash merges use it as the commit subject).

#### Version calculation: `cliff.toml`

- Conventional commit parsing; changelog groups: Features, Bug fixes, Documentation, Refactoring, Build and CI, Other; `chore(release)` commits left out; `require_conventional = true`.
- `[bump]`: `feat` → minor, `fix` and the rest → patch, breaking → major. **Before 1.0.0, breaking → minor** (`breaking_always_bump_major = false`; revisit at 1.0); `features_always_bump_minor = true`; `initial_tag = "v0.1.0"`.
- The bump base is the latest tag matching `^v[0-9]+\.[0-9]+\.[0-9]+$`: prerelease tags don't move it.

#### Scripts live in the justfile

Any logic beyond a single tool call is a `just` recipe, not inline shell in workflow YAML, so it can be read and run locally. Workflows install `just` (e.g. `extractions/setup-just`) and call recipes; the YAML only handles triggers, permissions, tokens and checkout.

| Recipe | Does | Used by |
|---|---|---|
| `just build` | `go build` with the version ldflags, into `dist/` (git-ignored) | locally |
| `just test` | `gofmt -l .` prints nothing, `go vet ./...`, `go test ./...` | CI, cut-release, release |
| `just run SUBCOMMAND ...` | `go run .` with the arguments, `[no-cd]` so relative paths work | locally |
| `just sync-ocmirror REF` | refresh the pinned oc-mirror copy | CI (drift check), locally |
| `just check-commits BASE` | Conventional Commits check of `BASE..HEAD` (git-cliff with `require_conventional`) | CI (pull requests) |
| `just check-title TITLE` | the PR title against the same pattern | CI (pull requests) |
| `just next-version [BUMP] [PRERELEASE]` | the version cut-release would use: `git cliff --bumped-version`, or `--bump patch\|minor\|major`; prerelease numbering from existing tags (`rc` → `-rc.1`, `-rc.2`, ...); fails if the tag exists or nothing is unreleased | cut-release, locally |
| `just changelog` | the unreleased section, on stdout | locally |
| `just prepare-release VERSION` | regenerate `CHANGELOG.md` (`git cliff --tag VERSION -o CHANGELOG.md`), commit it as `chore(release): VERSION`, annotated tag `VERSION`; no push | cut-release |
| `just check-tag TAG` | strict semver check (`^v(0\|[1-9][0-9]*)\.(0\|[1-9][0-9]*)\.(0\|[1-9][0-9]*)(-[0-9A-Za-z.-]+)?$`) and the tagged commit is on `main` (`git merge-base --is-ancestor`) | release |
| `just release-notes TAG` | that tag's section (`git cliff --current --strip all`), on stdout | release |
| `just release-snapshot` | `goreleaser check`, then `goreleaser release --snapshot --clean` | CI, locally |

Pushing and publishing (`git push --atomic`, `goreleaser release`) stay as workflow steps, so nothing run locally pushes or publishes by accident.

#### Workflow: CI (`.github/workflows/ci.yml`)

On every push and pull request, with the Go version from `go.mod`: `just test`; `just sync-ocmirror` at the pinned commit leaves the tree unchanged; `just release-snapshot` (both targets build); on pull requests, `just check-commits` and `just check-title`.

#### Workflow: cut-release (`.github/workflows/cut-release.yml`, manual)

- **Trigger:** `workflow_dispatch`, on `main` only (refuses other refs). Inputs: `bump: auto|patch|minor|major` (default `auto`) and `prerelease` (optional identifier, e.g. `rc`).
- **Concurrency:** one run at a time.
- **Auth:** a GitHub App installed on the repo with `contents: write`, allowed to bypass `main`'s protection. The workflow mints a short-lived token with `actions/create-github-app-token` (secrets `RELEASE_APP_ID`, `RELEASE_APP_PRIVATE_KEY`). Pushes made with it trigger the release workflow; pushes with the default `GITHUB_TOKEN` wouldn't.
- **Steps:** checkout `main` with full history and tags, using the App token; `just test` (no release from a red `main`); `VERSION=$(just next-version BUMP PRERELEASE)`; `just prepare-release VERSION`, committed and tagged as the App's bot user; `git push --atomic origin main VERSION`, so the commit and the tag land together or not at all.
- The run's summary shows the version and the new changelog section.

#### Workflow: release (`.github/workflows/release.yml`, on tag push)

- **Trigger:** push of a tag matching `v*`; `just check-tag` then rejects anything that isn't strict semver or isn't on `main`, publishing nothing.
- **Steps:** checkout with history; `just test`; `just release-notes TAG > RELEASE_NOTES.md`; `goreleaser release --clean --release-notes RELEASE_NOTES.md`.
- **Permissions:** `contents: write` only.
- **goreleaser** (`.goreleaser.yaml`):
  - `CGO_ENABLED=0`, linux/amd64 and linux/arm64 only (no macOS or Windows builds);
  - a `.tar.gz` per target with the binary, `LICENSE`, `README.md` and `CHANGELOG.md`, plus `checksums.txt`;
  - the version ldflags set from the tag;
  - goreleaser's own changelog disabled: git-cliff's notes are the release description;
  - `release.prerelease: auto`, so a `-rc.N` tag becomes a GitHub prerelease.
- A tag pushed by hand (by someone allowed to) also releases, with git-cliff's notes; `CHANGELOG.md` then catches up at the next cut-release.

#### One-time setup (README, maintainers section)

Create the GitHub App and its two secrets; add the App as a bypass actor on `main`'s protection; protect `v*` tags so only the App (and admins) can create them.

### Build order

Each step is one commit, or a short series, and leaves `just test` passing. README updates land with each step, not at the end.

1. **Skeleton:** `go.mod`, `main.go`, cobra root and `isc version`, justfile with the recipes above, the first `internal/ocmirror/` copy, `cliff.toml`, `.goreleaser.yaml`, the three workflows (CI, cut-release, release), a placeholder `CHANGELOG.md`, `.gitignore`.
2. **Documents:** `internal/isc` on top of `ocmirror/config`, and `internal/yamlnode`.
3. **`isc validate`:** static tier; then `internal/catalog` and the catalog tier; then `internal/olm` and the upgrade-path tier.
4. **`isc preview`:** selection and views; `--with-sizes`; the list subcommands; `operator-graph`.
5. **`isc operators optimise`,** reusing `internal/catalog`, `internal/validate` and `internal/preview`, following its plan's build steps.
6. **`isc merge`.**
7. **`isc create --from-cluster`,** OLM v0 first, then OLM v1.
8. **First release** through the cut-release action: a `-rc.1` prerelease first, then the release.
9. **Move the practices into `AGENTS.md`:** once the commands are built, move the consistent practices that proved out (input and output, loading with oc-mirror's code, messages, exit codes, catalog access, testing patterns) from this plan into `AGENTS.md`, as instructions for keeping commands consistent. Until then, this plan's shared behaviour is the source of truth.

### Upstream (oc-mirror)

Each command plan has an "Upstream (oc-mirror)" section. In summary:

| Command | Worth upstreaming into oc-mirror? | Still needed if oc-mirror followed the upgrade graph for operators? |
|---|---|---|
| `create --from-cluster` | **Partly.** Generating an ISC from what a cluster runs fits oc-mirror (or `oc adm`), but oc-mirror never talks to clusters today; an `oc` plugin may fit better. | **Yes.** It supplies the installed versions such a mode would start from. |
| `merge` | **Partly.** oc-mirror takes one `-c`; accepting several configs and merging them with these rules would remove the need, but it's general ISC tooling. | **Yes.** Unrelated to upgrade graphs. |
| `operators optimise` | **Yes, as the feature itself:** operator upgrade-graph traversal in oc-mirror's filter. | **No.** oc-mirror would mirror the path itself. |
| `preview` | **Yes.** oc-mirror's `--dry-run` already lists the images it would mirror (`mapping.txt`); a structured preview with sizes and the upgrade graph fits alongside. | **Yes.** Knowing what will be mirrored is useful whatever selects it. |
| `validate` | **Yes.** The static tier is oc-mirror's already; the catalog tier is its filter run early (`--dry-run` failing fast, or a `validate` subcommand). | **Partly.** Static and catalog tiers still needed; the upgrade-path tier isn't. |

Also worth upstreaming: making oc-mirror's config, API types and filter conversion public packages, so isc can drop its copy.

## Open questions (proposed answers)

1. **Reading clusters in Go or Ansible?** Decided: Go, as `isc create --from-cluster`, because the ClusterExtension catalog choice needs the catalog access optimise has. An Ansible role can still call it per inventory host.
2. **Dependency weight:** operator-registry already brings client-go, so create's dynamic client and cli-runtime add little. Proposed: accept it; check `go mod why` after step 7, and record the release binary size per target.

## Verification

- **Static:** `gofmt -l` empty, `go vet ./...`.
- **Unit and fixture tests** (`go test ./...`), no network or cluster, as listed in each command's plan.
- **Shared:** a failure writes nothing to stdout and exits 1; usage errors exit 2; the input file is unchanged after every command; the same inputs give the same output bytes.
- **Loader:** oc-mirror's own `load_test.go` and `validate_test.go` cases, run against the copy, give the same results; errors name the file; a valid ISC with every v2alpha1 section is accepted.
- **Sync:** `just sync-ocmirror` at the pinned commit leaves the tree unchanged (CI).
- **CI** green on the pull request.
- **Conventional Commits:** a pull request with a free-form commit or title fails `just check-commits` / `just check-title`.
- **Versioning:** on a scratch branch with sample commits, `just next-version` gives feat → minor, fix → patch, breaking (pre-1.0) → minor; with `prerelease: rc`, `-rc.1` then `-rc.2`; it fails when the tag exists or nothing is unreleased. `just changelog` groups them.
- **Release dry run:** `just release-snapshot` builds both targets locally; `isc version` on each built binary (arm64 under emulation or on an arm64 host) prints the snapshot version.
- **First real release:** cut-release with `prerelease: rc` puts a `chore(release)` commit and a `v0.1.0-rc.1` tag on `main`; the release workflow publishes a GitHub prerelease with both tarballs, `checksums.txt`, and git-cliff's notes as its description. Then a run without `prerelease`.
- **Bad tags:** pushing `v1.2` or `vfoo` triggers the release workflow, which fails `just check-tag` and publishes nothing.
- **End to end (connected session):** `isc create --from-cluster` on two real clusters (read-only), `isc merge`, a hand-made change (an added and a removed operator), `isc operators optimise` and `isc validate` against `registry.redhat.io`, then `oc-mirror --v2 --dry-run` on the result.

## What was and wasn't run

Planning only. This was written from the earlier plans in oc-mirror-automation and the decisions made since. No code exists yet, and nothing was built or run.
