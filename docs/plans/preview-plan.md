# `isc preview`: what an ISC will mirror, before mirroring: plan

Status: **planned**, not started.

## Summary

`isc preview` shows what an ImageSetConfiguration (ISC) is actually going to mirror, before oc-mirror is run: which bundles and images, which catalogs, packages and channels bring them in, how big it is, and how a package's upgrade graph looks through the ISC.

```
isc preview [-o summary|tree|table|json|yaml] [--with-sizes] [--depth N] [--package NAME]... [catalog flags] [-v] FILE
isc preview bundles  [-o text|json|yaml] [--package NAME]... [catalog flags] FILE
isc preview images   [-o text|json|yaml] [--with-sizes] [--package NAME]... [catalog flags] FILE
isc preview catalogs [-o text|json|yaml] FILE
isc preview operator-graph --package NAME [--catalog REF] [--channel NAME] [--min-version VERSION] [--mirrored-only]
                           [-o text|dot|json|yaml] [catalog flags] FILE
```

Catalog flags, input, output, messages and exit codes follow the tool plan's [shared behaviour](isc-tool-plan.md#shared-behaviour-every-command). It's a top-level command because it covers the whole ISC: operators now, other sections later ([Deferred](#deferred-sections)).

It can be run at any point; typically after `isc operators optimise`, alongside `isc validate`, before oc-mirror.

## Plan / Design decisions

### Selection is oc-mirror's own

- For each operator entry, fetch the catalog through the shared catalog access ([optimise plan](optimise-plan.md#catalog-access)) and run oc-mirror's filter from the pinned copy (`internal/ocmirror/operator`, `filterCatalog`; [tool plan](isc-tool-plan.md#loading-with-oc-mirrors-own-code)).
- **Images** are what oc-mirror collects from the filtered catalog (`internal/pkg/operator/catalog_handler.go`, oc-mirror `4802bb8d`): each bundle's image plus its `relatedImages`, through `handleRelatedImages` (self-contained, extracted into the pinned copy by name), with the loop of `getRelatedImagesFromCatalog` (a method using oc-mirror's logger) implemented the same way and cited. Duplicates across bundles and packages are counted once, as oc-mirror does.
- **The catalog image** itself is listed too: oc-mirror mirrors a rebuilt, filtered catalog.
- So the preview equals what oc-mirror would mirror, not an approximation.

### Views (`-o`)

On stdout, built in memory and written only on success:

| `-o` | Output |
|---|---|
| `summary` (default) | totals per catalog and package: bundles, images, and sizes with `--with-sizes`; grand totals |
| `tree` | catalog → package → channel → bundle → images, with counts (and sizes); `--depth N` stops at a level (1 catalog, 2 package, 3 channel, 4 bundle, 5 image) |
| `table` | one row per bundle: catalog, package, channel(s), version, image count (and size) |
| `json` / `yaml` | the full structure, for other tools |

- `--package NAME` (repeatable) narrows every view and subcommand to those packages.
- `-v` adds per-catalog fetch and cache lines on stderr.

### Flat lists: `bundles`, `images`, `catalogs`

- `isc preview bundles FILE`, `isc preview images FILE`, `isc preview catalogs FILE`: one reference per line (`-o text`, the default), or an array with `-o json` / `-o yaml`. For piping into other tools.
- `images` with `--with-sizes` adds each image's size to its line.
- Subcommands win over file names: a file literally called `bundles`, `images`, `catalogs` or `operator-graph` needs a path prefix (`./images`).

### Sizes: `--with-sizes`

Opt-in. Without it, preview makes no registry calls beyond fetching the catalogs.

- **Manifests only:** for each image, fetch its manifest, never its layers, and verify no signatures (signatures are separate artifacts oc-mirror handles while copying). A manifest lists each layer's and the config's compressed size (`layers[].size`, `config.size`). A multi-arch image means the index, then the manifest for `--platform`.
- Fetched with containers/image (`go.podman.io/image/v5`, `GetManifest`): the same auth, `registries.conf` and TLS handling as catalog fetches.
- **Cache:** manifests are cached by digest in `<cache-dir>/manifests/`. Most operator images are pinned by digest, so entries never go stale; a tag reference is resolved to a digest first.
- **Results:**
  - size per image, rolled up per bundle, channel, package and catalog;
  - totals **before layer dedupe** (the sum per image) and **after** (each layer digest counted once), both from the same manifests;
  - the rebuilt catalog image's size isn't known until oc-mirror builds it: listed as unknown.
- Sizes are compressed sizes, i.e. roughly what's transferred and stored in a registry.

### Upgrade graph for one package: `operator-graph`

```
isc preview operator-graph --package NAME [--catalog REF] [--channel NAME] [--min-version VERSION] [--mirrored-only]
                           [-o text|dot|json|yaml] [catalog flags] FILE
```

- **Graph:** the package's upgrade graph in the ISC's catalog: each channel (or the package's listed `channels`, or just `--channel`), with bundles as nodes and `replaces` / `skips` / `skipRange` edges.
- **Overlay from the ISC:**
  - which bundles oc-mirror would mirror;
  - the version clusters run: the package's `minVersion`, or `--min-version VERSION` to ask "what if clusters ran this version";
  - each channel's head, and the default channel;
  - the OLMv0 upgrade path from that version to each head, flagged where it leaves the mirrored set (the same finding as [`isc validate`](validate-plan.md)'s upgrade tier).
- `--catalog REF` picks the entry when the package is in several; `--mirrored-only` hides bundles and edges that wouldn't be mirrored.
- **Formats:**
  - `text` (default): a table per channel, head first, one row per bundle: version, `replaces`, `skips`, `skipRange`, mirrored, on the path, head and default-channel markers;
  - `dot`: a Graphviz graph, one edge style per edge type, mirrored bundles and the path highlighted (render with `dot -Tsvg`);
  - `json` / `yaml`: nodes, edges, overlay and paths.
- Uses `internal/olm` (channel order, edges, paths), shared with optimise and validate, so every command agrees on what an upgrade path is.

### Shared code

`internal/preview`: selection, image collection, counts and sizes. [`isc operators optimise`](optimise-plan.md) uses it for its candidate counts and report; [`isc validate`](validate-plan.md) `-v` for its "optimise would shrink this" counts.

### Deferred sections

Designed for now, built later:

- **Additional images:** listed, with `tagsByRegex` expanded to the matching tags.
- **Platform releases:** the versions selected through the update graph, and their payload images.
- **Helm charts:** the images found in the charts.

How the design keeps room for them:

- The structure's root is the ISC's `mirror` sections (`operators` now; `additionalImages`, `platform`, `helm` later), in every view and in `json` / `yaml`.
- `isc preview images` will list every section's images, each able to say which section brought it in (a `--section` filter later).
- Sizes and dedupe totals are computed across all sections, so adding one doesn't change how totals work.
- Until then, other sections present in the ISC are shown as "not previewed yet" rather than silently left out.

## Upstream (oc-mirror)

- **Worth upstreaming: yes.** oc-mirror's `--dry-run` already writes the images it would mirror (`mapping.txt`, `missing.txt`). A structured preview (by catalog, package and channel, with sizes and the upgrade graph) fits alongside it.
- **Still needed if oc-mirror followed the upgrade graph for operators: yes.** Knowing what will be mirrored, and how big it is, is useful whatever selects it. `operator-graph`'s path overlay becomes less important, since paths would be mirrored by construction.

## Open questions (proposed answers)

`operator-graph` edge cases, to settle together:

1. **Package in several catalog entries without `--catalog`** (e.g. `:v4.20` and `:v4.21`). Proposed: error listing the entries.
2. **Package not in the ISC.** Proposed: error by default; maybe later a mode showing the catalog's graph without an overlay.
3. **No bundle at the version, or several** (different builds of one version). Proposed: none is a finding with the nearest versions; several are all shown, with a path from each.
4. **Channel-level ranges.** Proposed: the overlay shows each channel's own range; the path starts from the channel's `minVersion` when the package has none.
5. **`full: true`.** Proposed: every bundle mirrored; the overlay still shows the path.
6. **Very large channels.** Proposed: no truncation; narrow with `--channel` and `--mirrored-only`.

## Verification

- **Selection** equals oc-mirror's filter output on synthetic FBC fixtures (shared `testdata/catalogs/`): package ranges, channel ranges, `full: true`, listed channels.
- **Images:** bundle images and related images shared across bundles and packages are deduplicated; the catalog image is listed.
- **Views:** golden output for `summary`, `tree` (with `--depth`), `table`, `json`, `yaml`; `--package` narrowing.
- **Subcommands:** `bundles`, `images`, `catalogs` in `text`, `json`, `yaml`.
- **Sizes** with an injected manifest source: single-arch; multi-arch index with `--platform`; a tag resolved to a digest; before and after dedupe totals with layers shared between images; the catalog image size unknown; a manifest cache hit; no registry calls beyond catalogs without `--with-sizes` (the injected source records none).
- **operator-graph:** each format; the overlay (mirrored, heads, default channel, path, broken path flagged); `--min-version`, `--channel`, `--catalog`, `--mirrored-only`; each open-question case once decided.
- **Deferred sections** in the input are reported as not previewed.
- **Failures:** a failed fetch exits 1 with nothing on stdout.
- **Connected session:** a real `redhat-operator-index` ISC; `isc preview images` compared with `oc-mirror --v2 --dry-run`'s `mapping.txt`.

## What was and wasn't run

Planning only. oc-mirror's image collection was read in `internal/pkg/operator/catalog_handler.go` (`upstream/main` `4802bb8d`). No code exists yet.
