# `isc validate`: checking an ISC will work with oc-mirror, before running it: plan

Status: **planned**, not started. First version: "will oc-mirror run?"; more checks follow as [later steps](#later-steps).

## Summary

`isc validate` checks that an ImageSetConfiguration (ISC) is valid and that oc-mirror will run with it successfully, before oc-mirror is run.

```
isc validate [--offline] [--strict] [--cache-dir DIR] [--authfile FILE] [--tls-verify=false] [--fetch-timeout SECONDS] [-v] FILE...
```

- **Valid:** `validation successful` on stdout, with the warning count if there are warnings.
- **Findings:** errors and warnings on stderr, one line each, e.g. `warning: isc.yaml: catalog registry.redhat.io/redhat/redhat-operator-index:v4.21: package foo: not in the catalog`.
- **Exit codes:** 0 valid (warnings allowed), 1 invalid or couldn't check (e.g. a catalog fetch failed), 2 usage.
- There's no `-o`: the result is the exit code and the messages. Other input and message rules follow the tool plan's [shared behaviour](isc-tool-plan.md#shared-behaviour-every-command).

In the pipeline it's the last step before oc-mirror: create → merge → your own changes → `isc operators optimise` → **`isc validate`** → `oc-mirror --v2`.

## Plan / Design decisions

### What the first version checks

The question is only "will oc-mirror run?", answered with oc-mirror's own code where possible.

#### Static (always; no network)

- **oc-mirror's own loading, run as oc-mirror runs it:** `ReadConfig` from the pinned copy of oc-mirror's config package ([tool plan](isc-tool-plan.md#loading-with-oc-mirrors-own-code)). That is strict YAML, `kind` and stanza checks, unknown fields rejected (including `metadata`), and all of `Validate` (`internal/pkg/config/validate.go`, oc-mirror `0ff2c6c6`):
  - duplicate catalogs (key from `GetUniqueName`), duplicate packages in a catalog, duplicate channels in a package;
  - package and channel `minVersion`/`maxVersion` must be semver;
  - package-level and channel-level ranges can't be mixed;
  - no duplicate release channels;
  - `platform.platforms` and `platform.architectures` can't be used together;
  - blocked-image names must be valid regular expressions.
- **Filter configuration:** each operator entry converts to a catalog-filter configuration that passes catalog-filter's `Validate()`, as oc-mirror's `filterFromImageSetConfig` does (from the pinned copy).
- Errors keep oc-mirror's wording, prefixed with the file name; each error in `ReadConfig`'s aggregate becomes its own finding.

#### Catalog (skipped with `--offline`)

Each distinct catalog is fetched once through the shared catalog access ([implementation](#internalcatalog)), then for each operator entry:

| Level | Finding |
|---|---|
| error | oc-mirror's filter fails for the entry, run exactly as oc-mirror runs it: e.g. a `minVersion` that empties the default channel, a default channel filtered out, several heads or a cycle in a channel, a bundle version that isn't strict semver. oc-mirror's wording: `collect catalog "REF": failed to filter catalog: ...`. |
| error | The catalog image has no `operators.operatorframework.io.index.configs.v1` label. oc-mirror silently extracts nothing and fails later with "no related images found"; validate says why. |
| warning | A listed package, listed channel or `defaultChannel` isn't in the catalog. catalog-filter skips these silently, so oc-mirror succeeds but mirrors nothing for them. |

An entry with `full: true` and no packages isn't filtered (as in oc-mirror); only the catalog load is checked.

### Flags

- `--offline`: static checks only. No catalog fetches, no network, no cache: a quick lint for pre-commit hooks or CI without registry credentials.
- `--strict`: warnings fail validation too (exit 1).
- Catalog flags as optimise and create: `--cache-dir`, `--authfile`, `--tls-verify`, `--fetch-timeout`. There's no `--platform`: like oc-mirror, isc reads the catalog from the first manifest in its index.
- `-v`: per-catalog fetch and cache lines on stderr.
- Several files: each is checked and every finding names its file; the exit code covers them all.

### Out of scope

- Whether additional images, platform release versions and helm charts exist or are reachable.
- Mirroring anything.

### Later steps

After [`isc operators optimise`](optimise-plan.md) brings `internal/olm` (the OLMv0 port) and [`isc preview`](preview-plan.md) brings `internal/preview`:

1. **Attribution:** when the filter fails for an entry, re-run it per package (on a clone) to name the package, and check strict-semver bundle versions to name the bundle.
2. **Graph and version warnings:** the package fails opm validation (`declcfg.ConvertToModel`, e.g. stranded bundles; OLMv0 tolerates it); an unparseable `skipRange` (OLM ignores it); several chains, no head or a cycle in a relevant channel (OLMv0's `sortChannel` can't order it); a `minVersion` that silently excludes prerelease-style versions (`7.9.0-2` vs `>=1.2.0`; suggest the `-0` form).
3. **Upgrade paths** (packages with a `minVersion`, treated as the version clusters run, as optimise does): replay OLMv0 on the **filtered** channel, the catalog clusters would see, from every bundle at `minVersion`; error if it can't reach the default channel's head, warning for another mirrored channel; listed `channels` limit the check. Evidence: the head and its edges, the mirrored entries, the hops taken, where it got stuck, and the path in the full catalog for comparison.
4. **`-v`:** "optimise would shrink this package", with before and after counts from `internal/preview`.

A tier runs for a package only if the earlier ones passed for it, so one mistake doesn't produce a cascade of findings.

**OLMv0's upgrade rule**, which steps 2 and 3 rely on (operator-lifecycle-manager `master`: `pkg/controller/registry/resolver/resolver.go`, `cache/predicates.go`, `solver/constraints.go`):
- `sortChannel` orders a channel along the `replaces` chain from the head; entries reachable only through `skips` are left out of the chain; no head, a cycle, or more than one chain is an error (`a unique replacement chain within a channel is required ...`);
- the candidates for an installed bundle B are chain entries whose `replaces` is B's name, whose `skips` lists B, or whose `skipRange` (blang semver) contains B's version;
- the solver prefers the earliest candidate ("Identifiers appearing earlier in the argument list have higher preference"), and candidates are listed head first, so OLM moves to the **newest** candidate, one hop per resolution, then repeats.

### Shared code

`internal/validate`, used by this command and by `isc operators optimise`: a candidate is valid if it passes the static and catalog checks, and optimise's self-check runs every check on the result.

## Implementation design

Second PR of the build (the first is the groundwork in the [tool plan](isc-tool-plan.md#build-order)). Branch `feat/validate`. New dependencies, at oc-mirror's versions: `github.com/sherine-k/catalog-filter` v0.0.5, `go.podman.io/image/v5` v5.41.1, `github.com/google/go-containerregistry` v0.22.0.

### Facts it relies on

Read in the sources (oc-mirror `0ff2c6c6`; the files and functions below are unchanged since `4802bb8d`):

- **oc-mirror's per-entry flow** (`internal/pkg/operator/filtered_collector.go`, `filterOperator`): `full: true` with no packages → the unfiltered catalog; otherwise `filterCatalog` (`catalog_handler.go`): `filterFromImageSetConfig` builds the catalog-filter configuration (`>=min`, ` <=max` ranges, per package or channel) and validates it, then `NewMirrorFilter(config, InFull(op.Full)).FilterCatalog`. Errors are wrapped `failed to validate catalog filter: ...` / `failed to filter catalog: ...`, then `collect catalog "REF": ...`.
- **catalog-filter v0.0.5** (`pkg/filter/mirror-config/v1alpha1`):
  - missing packages and listed channels are **ignored without an error**;
  - every bundle in a kept package needs a strict-semver `olm.package` version (`mmsemver.StrictNewVersion`), or the whole filter fails;
  - default-channel errors: `specified default channel override %q does not exist in the filtered output`, `the default channel %q was filtered out, a new default channel must be configured for this package`; an emptied default channel: `... results in an empty channel`;
  - channel graph errors (`newChannel`): `no channel heads found`, `multiple channel heads found: %v`, `detected a cycle in the upgrade graph of the channel`; `skipRange` is never read;
  - with no packages configured it edits its input in place, and filtered output shares slices with the input: **deep-copy the catalog before every filter run**;
  - prerelease versions fail `>=X` unless `X` has a prerelease (Masterminds semver v3.5.0).
- **Catalog extraction** (`catalog_handler.go` `EnsureCatalogInOCIFormat`, `ExtractOCIConfigLayers`; `internal/pkg/manifest/oci-manifest.go` `GetOCIImageFromIndex`, `ExtractOCILayers`; `internal/pkg/archive/utils/tar.go`): oc-mirror copies the whole index into an OCI layout (`RemoveSignatures`, an accept-anything policy), takes the **first image descriptor** in the index (recursing into the first child index, up to depth 5), reads the configs label, and untars `mutate.Extract(img)` (go-containerregistry applies whiteouts) keeping entries whose name contains the trimmed label, regular files and directories only. A missing label extracts nothing.
- **containers/image v5.41.1:** `NewImageSource` + `GetManifest(ctx, nil)` + `manifest.Digest` give the top-level (index) digest; `copy.Image` with `ImageListSelection: CopySpecificImages` and `Instances` copies chosen instances; `signature.NewPRInsecureAcceptAnything` for the policy; `registries.conf` mirrors are honoured. Copying one instance changes the layout's digest, so the cache is keyed by the **source** digest.
- **go-containerregistry v0.22.0:** `layout.ImageIndexFromPath`, `IndexManifest`, `Image`, `ConfigFile().Config.Labels`, `mutate.Extract`.
- **operator-registry v1.74.0:** `declcfg.LoadFS(ctx, fs.FS)` parses every non-ignored file.

### `internal/ocmirror/operator`

- `filter.go`, synced by name from `catalog_handler.go` by `just sync-ocmirror` (through `tools/extractfuncs`): `filterPackage`, `filterFromImageSetConfig`, `filterCatalog`, and `handleRelatedImages` (for preview later). Imports only the copied v2alpha1, image and consts, declcfg, catalog-filter and apimachinery.
- `exports.go`: the only hand-written file under `internal/ocmirror/`, marked as such: exported wrappers (`FilterConfig`, `FilterCatalog`) for the unexported originals.

### `internal/catalog`

- `type Source interface { Load(ctx context.Context, ref string) (*declcfg.DeclarativeConfig, Info, error) }`, with `Info{Ref, Digest string; CacheHit bool}`.
- `NewRegistry(Options{CacheDir, AuthFile string; TLSVerify bool; Timeout time.Duration}) Source`:
  1. **Digest:** a reference pinned by digest uses it; otherwise `NewImageSource` + `GetManifest(nil)` + `manifest.Digest` (the index digest, as oc-mirror pins).
  2. **Cache hit:** `<cache-dir>/catalogs/sha256-<hex>/` with a `complete` marker → load it.
  3. **Miss:** if the manifest is a list, take its first image instance (oc-mirror's order); `copy.Image` that instance (`CopySpecificImages`; a single image is copied as is) into a temp OCI layout, `RemoveSignatures`, accept-anything policy, the `SystemContext` built from the options (auth file, TLS). Open the layout with go-containerregistry, select the image the same way, read the configs label: **missing → error** naming the image. Extract as oc-mirror does (`strings.Contains` on the trimmed label, regular files and directories, paths sanitized) into a temp directory in `<cache-dir>/catalogs/`, write `complete`, rename into place; delete the layout. A failure leaves no cache entry.
  4. **Load:** `declcfg.LoadFS(ctx, os.DirFS(<entry>/<label>))`.
- `oci://` references are read from that layout directly, with the same selection and extraction, into a temp directory (no cache).
- `--fetch-timeout` bounds each catalog's resolve + copy + extract.
- `Clone(*declcfg.DeclarativeConfig) *declcfg.DeclarativeConfig`: deep copy by JSON round trip, used before every filter run.

### `internal/validate`

```go
type Level int // Error, Warning
type Finding struct {
    Level                                 Level
    File, Catalog, Package, Channel, Message string
}
type Options struct {
    Offline  bool
    Catalogs catalog.Source
}
// Findings are problems in the ISC; the error means it couldn't be checked (fetch, I/O).
func Validate(ctx context.Context, path string, opts Options) ([]Finding, error)
```

1. `isc.Load(path)`; each aggregated `ReadConfig` error → an error finding. Stop for this file if loading failed.
2. For each operator entry: `ocmirror/operator.FilterConfig(entry)` → error finding on failure.
3. Unless offline: load each distinct catalog once; for each entry:
   - `full: true` with no packages → nothing more;
   - warnings for listed packages, listed channels and `defaultChannel` missing from the catalog;
   - `FilterCatalog(ctx, *catalog.Clone(dc), entry)` → error finding with oc-mirror's wrapping on failure.

### `internal/cli/validate.go`

- Flags as in the [summary](#summary); default cache dir `os.UserCacheDir()/isc`.
- Findings on stderr: `error: FILE: catalog C: package P: channel CH: message` (parts left out when empty), `warning: ...`.
- `validation successful` (plus `, N warnings`) on stdout only if there are no errors, and no warnings with `--strict`.
- Exit 0 / 1 / 2 as in the summary.

### Tests and fixtures

- `testdata/catalogs/<name>/catalog.json`, synthetic FBC: a valid catalog; a `minVersion` that empties the default channel; a default channel filtered out; several heads; a cycle; a bundle version that isn't strict semver; a package, a channel and a default channel missing.
- `testdata/validate/<case>.yaml` + `<case>.golden` (expected findings), run through `Validate` with a test-only `Source` reading `testdata/catalogs`; static cases: each oc-mirror `Validate` rule once, `metadata`, a filter configuration catalog-filter rejects.
- `internal/catalog`: an in-process registry (go-containerregistry `pkg/registry`, tests only) serving a multi-arch catalog image: digest resolution, first-instance copy, label extraction, missing-label error, cache hit, a moved tag fetches again, a failed fetch leaves no cache entry; `Clone` gives an independent copy.
- CLI: exit codes; stdout only on success; `--offline` makes no fetch (the test source records no call); `--strict`; several files.

## Upstream (oc-mirror)

- **Worth upstreaming: yes.**
  - The static checks are already oc-mirror's (`internal/pkg/config/validate.go`); isc runs a copy of them.
  - The catalog checks are oc-mirror's own filter (`internal/pkg/operator/catalog_handler.go`, catalog-filter) run early, before any image is copied. They would fit a `--dry-run` that stops at the first filter error, or an `oc-mirror validate` subcommand. The missing-label error and the missing package/channel warnings would be small fixes in oc-mirror itself.
  - The later upgrade checks would be warnings in that mode.
- **Still needed if oc-mirror followed the upgrade graph for operators: partly.** The static and catalog checks are still needed; the later upgrade-path step isn't, because paths would be mirrored by construction.

## Open questions (proposed answers)

None yet; new questions go here.

## Verification

- **Static:** one fixture per oc-mirror `Validate` error (matching oc-mirror's `validate_test.go` cases), unknown fields including `metadata`, a filter configuration catalog-filter rejects.
- **Catalog:** each error and warning in the table, on the synthetic fixtures.
- **Catalog access:** the in-process registry cases above.
- **Flags and output:** `--offline` makes no fetch; `--strict` turns a warning into exit 1; several files, each finding naming its file; stdout has only `validation successful`, and only when valid; a failed fetch exits 1 with nothing on stdout.
- **Connected session (manual):** a real `redhat-operator-index` ISC; one that validate accepts passes `oc-mirror --v2 --dry-run`, and one with a filter error fails it.

## What was and wasn't run

Planning only. oc-mirror's loader, validation, filter flow and catalog extraction were read in its source (`upstream/main` `0ff2c6c6`, and `4802bb8d` before it); catalog-filter v0.0.5, operator-registry v1.74.0, containers/image v5.41.1 and go-containerregistry v0.22.0 in the Go module cache; OLMv0's resolver on GitHub (`master`). No code exists yet.
