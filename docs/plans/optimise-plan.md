# `isc operators optimise`: the smallest operator mirror that still lets clusters upgrade: plan

Status: **planned**, not started. Replaces the earlier `isc operators prune` plan, which could only remove a `minVersion` or keep it; optimise also picks per-channel `minVersion`s and the default channel.

## Summary

`isc operators optimise` rewrites the operator part of an ISC so oc-mirror mirrors only the bundles OLM needs to upgrade clusters from the version they run today to the head of each channel they might be on, checked with oc-mirror's own filter.

```
isc operators optimise [-o yaml|json|kyaml|diff|text|table] [--interactive] [--check] [--package NAME]...
                       [--cache-dir DIR] [--authfile FILE] [--tls-verify=false] [--platform OS/ARCH] [--fetch-timeout SECONDS]
                       [-v] FILE
```

Alias: `optimize`. Input, output, messages and exit codes follow the tool plan's [shared behaviour](isc-tool-plan.md#shared-behaviour-every-command).

### Why

oc-mirror doesn't work out an upgrade graph when choosing which operator bundles to mirror. With a package `minVersion`, it mirrors every bundle from that version to the head, in every channel that has one; without one, only each channel's head. Neither matches what OLM needs:

- **Too much.** `minVersion` pulls in every version of every newer channel: `openshift-gitops-operator` from 1.19.0 mirrors 32 bundles (408 images) where 4 (64 images) let clusters on `latest` upgrade.
- **Too little.** Heads only is enough only if each head upgrades straight from the cluster's version; often it doesn't, and clusters get stuck while the mirror merely looks smaller.

oc-mirror's filter also silently drops prerelease-style versions, and a `minVersion` that empties a default channel makes oc-mirror reject the package.

### What would fix it upstream

oc-mirror already follows the update graph for platform releases (`shortestPath`), but not for operators. If it treated `minVersion` as "the version my clusters run" and mirrored exactly the bundles on OLM's upgrade path to each channel head, the mirror would be minimal and correct by construction, ISCs could keep listing packages without `channels`, and prerelease suffixes and emptied default channels would stop being traps. Until then, `isc operators optimise` is that missing step, run before oc-mirror.

## Plan / Design decisions

### Scope

- **In:** `mirror.operators` packages with a package-level `minVersion`.
- **Out:** platform releases, `additionalImages`, helm charts (passed through untouched); mirroring itself (oc-mirror does that).
- **Model:** OLMv0's upgrade rules (see [OLMv0 and OLMv1](#olmv0-and-olmv1)).
- Same answers as oc-mirror and OLM, by using their libraries (catalog-filter, operator-registry) rather than reimplementing them.

### How oc-mirror and OLM behave

- **oc-mirror's filter** (`internal/pkg/operator/catalog_handler.go`, `filterPackage`): a `minVersion` becomes the range `">=" + minVersion`, and `maxVersion` adds `" <=" + maxVersion`. The filter is `github.com/sherine-k/catalog-filter` (v0.0.5 in oc-mirror's `go.mod`), `pkg/filter/mirror-config/v1alpha1/mirror-filter.go` (`FilterCatalog`):
  - **no range:** keeps each channel's head only (`filterChannelHead`);
  - **with a range:** `channel.filterByVersionRange` (`channel.go`) walks the `replaces` chain from the head, keeping every entry down to the lowest bundle in range, including entries outside the range between them, plus in-range bundles named in `skips`;
  - **package-level range with no channels:** applies to every channel of the package;
  - **package-level and channel-level ranges together** are rejected;
  - **`skipRange` is never read.** The head is the entry with no incoming `replaces` or `skips` edge (`newChannel`); zero or several heads is an error (`multiple channel heads found`).
- **OLMv0 on the cluster** upgrades an installed version V to a bundle whose `replaces` names V, whose `skips` lists V, or whose `skipRange` (a blang/semver range) contains V, within the subscribed channel.

### Assumption

A package's `minVersion` (`X`) is the version clusters run. Goal: the smallest mirror that lets them upgrade to the head of every channel they might be on. Documented in `--help` and the README.

### Listed channels are the scope

- If a package lists `channels`, **only those channels are processed**: candidate channels, upgrade paths, counts and the report consider nothing else, and the package's other channels are never added. Every candidate keeps exactly the listed channels, so no channel can be lost; the report says the scope came from the ISC.
- If the listed channels don't include the catalog's default channel, `defaultChannel` must name one of them: set to the listed channel with the newest head when missing. An existing `defaultChannel` outside the list makes every candidate invalid, so the package is kept as it is (see [Invalid candidates](#invalid-candidates-are-discarded)).
- A package without `channels` considers every channel with a bundle `>= X`.
- No CLI flag for channels: they come only from the ISC.

### Skipped (passed through, with a `-v` line saying why)

- Catalog entries with `full: true`, no `packages`, or no package with a `minVersion`.
- Packages without a package-level `minVersion`.
- Packages with channel-level `minVersion`/`maxVersion`.
- Packages with `maxVersion` (see [TODO](#todo)).

oc-mirror's ISC has no `bundles` field (`IncludePackage` in `internal/pkg/api/v2alpha1/type_config_include.go` has `name`, `channels`, `defaultChannel`, `minVersion`, `maxVersion`; `minBundle` is commented out), so an exact bundle list can't be written, and an input with one fails to load.

### No-`minVersion` warning

For each channel a heads-only mirror would include, if the head's only edge is `replaces`, a `review` line:

> channel `stable` head 1.4.3 only replaces 1.4.2, so a heads-only mirror gives clusters on any other version no upgrade path. Set `minVersion` to the version your clusters run.

### Algorithm per package

1. Find every bundle at version `X` (build metadata ignored); the path must work from each.
2. Candidate channels: the listed `channels` if any, otherwise every channel with a bundle `>= X` (Masterminds, as the filter compares).
3. Per channel, order as OLMv0's `sortChannel` and choose:
   - **head only** if the head upgrades directly from `X` (`replaces`, `skips`, parseable `skipRange`) or is `X`;
   - else **channel `minVersion` at the first hop** of the upgrade path, prerelease-aware, with every hop confirmed kept by the filter;
   - else **channel `minVersion: X`**, with a `review` line.
4. Default channel: if the catalog's default channel has no candidate bundle, or isn't among the listed channels, set `defaultChannel` to the candidate channel with the newest head and make sure it's listed.
5. Build three candidates and count bundles and images (bundle image + `relatedImages`, deduplicated per catalog entry) for each through the real filter:
   - **package-minversion** (the package as it is);
   - **no-minversion** (heads only);
   - **per-channel** (steps 3–4).
6. Each candidate is checked: **valid** (see below) and **covers every upgrade path** (from every bundle at `X` to the head of every candidate channel, every hop kept by the filter).
7. **auto** picks, among valid candidates that cover every upgrade path, the fewest images, then fewest bundles; ties go to package-minversion, then no-minversion, then per-channel (explicit channel lists miss channels added later).

### Invalid candidates are discarded

- A candidate is offered (to auto, `--interactive`, and as the report's choice) only if what it produces is valid: the resulting ISC passes the oc-mirror-style loader ([Loading like oc-mirror](isc-tool-plan.md#loading-like-oc-mirror)), and the catalog-filter run for that catalog entry succeeds. A `minVersion` that empties the default channel, or a `defaultChannel` the filter rejects, makes a candidate invalid.
- The report still lists discarded candidates, marked invalid, with the loader or filter error.
- **If no candidate is valid and covers every upgrade path,** the package keeps the `minVersion` currently configured, unchanged, with a `review` line (and the filter's error if the input itself fails it). `--interactive` doesn't prompt for such a package.

### Choosing per package: `--interactive`

- No `--strategy` flag: one strategy for every operator doesn't fit. Each package gets auto's pick, unless `--interactive` is given.
- `--interactive`: for each package with more than one offered candidate, show its candidate table and prompt for the one to use, auto's pick as the default. Prompts go to stderr and answers are read from the terminal (`/dev/tty`), so stdout carries only the output. No terminal available is an error. Nothing is saved: the next non-interactive run uses auto again.

### Output: `-o`

On stdout, built in memory and written only on success:

| `-o` | Output |
|---|---|
| `yaml` (default) | the optimised ISC; comments and key order kept |
| `json` | the optimised ISC as JSON |
| `kyaml` | the optimised ISC as Kubernetes KYAML (`sigs.k8s.io/yaml/kyaml` `Encoder`, v1.6.0) |
| `diff` | unified diff of the input against the optimised `yaml` |
| `text` | report, below |
| `table` | the same report as tables, one row per package and candidate |

- `json` and `kyaml` don't keep comments. oc-mirror loads all three ISC formats (`YAMLToJSON` accepts JSON and KYAML, which are YAML).
- **Report**, per catalog and package:
  - the decision (rewritten, kept, skipped) and why, with evidence: `X`, each candidate channel's head and its `replaces`/`skips`/`skipRange`, and the upgrade path from `X` to each head;
  - every candidate: bundles, images, valid or not (with the error), covers every upgrade path or not, the channels available in the mirrored catalog and the default channel; channels lost compared with the input are flagged (never for a package that lists `channels`);
  - which candidate was chosen, and why;
  - per catalog: bundles and images before and after.
- `--check`: exit 3 if the ISC would change; works with any `-o`.
- `--package NAME` (repeatable): only these packages; the rest pass through.
- `-v`: one stderr line per package decision.

### Writing the result

- Flatten anchors, aliases and `<<` merge keys first (`internal/yamlnode`), so a change to one entry can't leak into another through a shared node; the output is plain.
- Edit the yaml.v3 node tree, so comments, key order and unrelated sections are kept.
- **Self-check before output:** load the result with the oc-mirror-style loader, run the per-catalog filter, and confirm every chosen candidate's upgrade paths are fully mirrored; otherwise output nothing and exit 1.
- The report warns that per-channel output is tied to today's catalog: re-run from the original ISC when catalogs change.

### Catalog access

Shared with `isc create` through `internal/catalog`, done the way oc-mirror does it (oc-mirror `upstream/main` at `4802bb8d`):

1. Resolve the reference to a digest (skipped when pinned).
2. **Cache hit:** the unpacked configs for that digest exist in `<cache-dir>/catalogs/<digest>/`: use them.
3. **Miss:** copy `docker://REF@digest` for `--platform` (default `linux/amd64`) into an OCI layout with containers/image, as oc-mirror's `EnsureCatalogInOCIFormat` does through its mirror copier (`internal/pkg/operator/catalog_handler.go`). Same auth files, `registries.conf` and TLS handling as oc-mirror.
4. Open the layout with go-containerregistry, read the `operators.operatorframework.io.index.configs.v1` label, and untar only that path, as `ExtractOCIConfigLayers` and `ExtractOCILayers` (`internal/pkg/manifest/oci-manifest.go`) do, into a temp directory renamed into place. The layout is then removed.
5. `declcfg.LoadFS`.

- An `oci://` catalog reference is read from that layout directly: it's the image the ISC names.
- **No pre-extracted catalogs:** no directory source or `REF=DIR` flag. Content always comes from the image the ISC references, unless that digest is already unpacked in the cache. Tests inject the catalog source in code.
- Flags: `--cache-dir` (default `${XDG_CACHE_HOME:-~/.cache}/isc`), `--authfile`, `--tls-verify`, `--platform`, `--fetch-timeout` (per catalog, default 1800 s).
- Content-addressed: no expiry; a moved tag gives a new digest. To clear the cache, delete the directory.

### Code layout

```
internal/optimise/         Optimise(): optimised ISC, per-package decisions, candidates and counts
internal/olm/              OLMv0 channel order, upgrade path, edge matching
internal/mirrorfilter/     oc-mirror's ISC -> catalog-filter conversion and filter runs
internal/versions/         Masterminds vs blang semver helpers, prerelease-aware minVersion strings
internal/catalog/          digest lookup, OCI copy, configs extraction, cache (shared with create)
```

```go
type Options struct {
    Catalogs catalog.Source
    Packages []string
    Choose   func(pkg Package, offered []Candidate, auto Candidate) (Candidate, error) // nil = auto
}
type Result struct {
    Document  *isc.Document // input is never mutated
    Changed   bool
    Decisions []Decision    // per package: skipped/kept/rewritten, why, every candidate with counts and validity
}
func Optimise(ctx context.Context, doc *isc.Document, opts Options) (*Result, error)
```

`--interactive` supplies `Choose`; everything stays under `internal/` (tool plan).

### Dependencies

- `github.com/sherine-k/catalog-filter`, at the version the targeted oc-mirror pins (v0.0.5 today).
- `github.com/operator-framework/operator-registry` (`declcfg`, `model`, `property`).
- `github.com/Masterminds/semver/v3` (filter semantics), `github.com/blang/semver/v4` (OLM `skipRange` semantics).
- `go.podman.io/image/v5` (digest lookup and the OCI copy), `github.com/google/go-containerregistry` (reading the layout and its layers).
- `sigs.k8s.io/yaml` v1.6.0 or later (`kyaml`).

The matched oc-mirror version and OLM commit are recorded in `docs/compatibility.md` and `isc version`.

### OLMv0 and OLMv1

From operator-controller `main` (`internal/operator-controller/catalogmetadata/filter/successors.go`, `internal/operator-controller/resolve/catalog.go`):

- **Same edges by default:** OLMv1's `legacySuccessor` accepts a bundle that `replaces` the installed bundle, lists it in `skips`, or has a `skipRange` containing it.
- **Different choice:** OLMv1 pools all channels of the package (or those in `spec.source.catalog.channels`) and picks the highest-version successor, preferring non-deprecated bundles; OLMv0 stays within the subscribed channel and follows its order.
- **Only in OLMv1:** an optional version range (`spec.source.catalog.version`), and `upgradeConstraintPolicy: SelfCertified`, which ignores edges entirely. OLMv1 has no dependency resolution.
- **Effect:** a mirror that gives every candidate channel a complete OLMv0 path also gives OLMv1 a path, since every hop is an OLMv1 successor. So optimise's output is safe for OLMv1 clusters on the same channels; an OLMv1 mode could only shrink it further.

### TODO

- `maxVersion` support (packages with one are skipped for now).
- An OLMv1 mode: cross-channel hops, and heads only under `SelfCertified`.

### Build steps

1. `internal/catalog`: digest lookup, OCI copy, configs extraction, cache; an injectable source for tests.
2. `internal/olm`, `internal/mirrorfilter`, `internal/versions`, with fixture tests.
3. `internal/optimise`: skips, warning, per-channel selection, candidates, validity, auto, self-check.
4. The command: `-o` formats, `--check`, `--package`, `--interactive`.
5. Docs: README sections (why, upstream fix, assumption), `docs/compatibility.md`.

## Open questions (proposed answers)

None open; settled answers are in the design above. New questions go here.

## Verification

- **Unit tests** in `internal/olm`, `internal/mirrorfilter`, `internal/versions` on synthetic FBC fixtures in `testdata/catalogs/`, one per edge case: edges via `replaces`/`skips`/`skipRange`, skipped bundles, several bundles at `X`, multiple heads, cycles, bad `skipRange`, prerelease versions (`-rhodf`, `-opr-1`, `7.9.0-2`), emptied default channel, no bundle at `X`, replaces-only heads.
- **Candidates:** an invalid candidate discarded and reported; every candidate invalid (input kept, `review` line); a package with listed `channels` (only those processed, none added, none lost); listed channels without the default channel (`defaultChannel` set); a `defaultChannel` outside the list (kept as is).
- **Golden tests** (`go test ./... -update`) for each `-o` format: `yaml`, `json`, `kyaml` (the last two also load through the oc-mirror-style loader), `diff`, `text`, `table`.
- **Flags:** `--check` (exit 3 on change, 0 otherwise, with any `-o`); `--package`; `--interactive` with scripted terminal input (default accepted, another candidate chosen), and its error without a terminal.
- **Output only on success:** a failed fetch, filter or self-check writes nothing to stdout and exits 1.
- **Round trip:** comments and unrelated sections survive with `-o yaml` byte for byte; anchored input is flattened.
- **Property:** every optimised fixture's filtered output contains the full upgrade path from `X` in each candidate channel.
- **Catalog access:** cache hit and miss with an injected registry; a moved tag fetches again; a pinned reference skips the lookup; a failed fetch leaves no cache entry.
- **`e2e` build tag:** real `redhat-operator-index` catalogs, bundle and image counts recorded per candidate, plus `oc-mirror -c out.yaml --v2 --dry-run --workspace file:///tmp/ws docker://localhost:5999` reaching the registry-access error.

## What was and wasn't run

Planning only. The oc-mirror filter, catalog extraction and loader behaviour, and the operator-controller successor rules, were read in their sources (cited above), not run. No code for `isc operators optimise` exists yet.
