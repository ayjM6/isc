# `isc validate`: checking an ISC will work with oc-mirror, before running it: plan

Status: **planned**, not started.

## Summary

`isc validate` checks that an ImageSetConfiguration (ISC) is valid and that oc-mirror will run with it successfully, before oc-mirror is run. It also checks that clusters could upgrade from each package's `minVersion` with what would be mirrored.

```
isc validate [--offline] [--strict] [--cache-dir DIR] [--authfile FILE] [--tls-verify=false] [--platform OS/ARCH] [--fetch-timeout SECONDS] [-v] FILE...
```

- **Valid:** `validation successful` on stdout, with the warning count if there are warnings.
- **Findings:** errors and warnings on stderr, one line each, e.g. `error: isc.yaml: catalog registry.redhat.io/redhat/redhat-operator-index:v4.21: package foo: channel stable not in the catalog`.
- **Exit codes:** 0 valid (warnings allowed), 1 invalid or couldn't check (e.g. a catalog fetch failed), 2 usage.
- There's no `-o`: the result is the exit code and the messages. Other input and message rules follow the tool plan's [shared behaviour](isc-tool-plan.md#shared-behaviour-every-command).

In the pipeline it's the last step before oc-mirror: create → merge → your own changes → `isc operators optimise` → **`isc validate`** → `oc-mirror --v2`.

## Plan / Design decisions

### Tiers

Checks run in three tiers. A tier runs for a package only if the earlier tiers passed for it, so one mistake doesn't produce a cascade of findings.

#### 1. Static (always; no network)

- **oc-mirror's own loading, run as oc-mirror runs it:** `ReadConfig` from the pinned copy of oc-mirror's config package ([tool plan](isc-tool-plan.md#loading-with-oc-mirrors-own-code)). That is strict YAML, `kind` and stanza checks, unknown fields rejected (including `metadata`), and all of `Validate` (`internal/pkg/config/validate.go`, oc-mirror `4802bb8d`):
  - duplicate catalogs (key from `GetUniqueName`), duplicate packages in a catalog, duplicate channels in a package;
  - package and channel `minVersion`/`maxVersion` must be semver;
  - package-level and channel-level ranges can't be mixed;
  - no duplicate release channels;
  - `platform.platforms` and `platform.architectures` can't be used together;
  - blocked-image names must be valid regular expressions.
- **Filter configuration:** each operator entry converts to a catalog-filter configuration (oc-mirror's `filterFromImageSetConfig`, from the pinned copy) that passes catalog-filter's `Validate()`.
- Errors keep oc-mirror's wording, prefixed with the file name.

#### 2. Catalog (skipped with `--offline`)

Fetch each operator catalog through the shared catalog access ([optimise plan](optimise-plan.md#catalog-access)) and run oc-mirror's filter (`filterCatalog`, from the pinned copy) for each entry: all packages together, as oc-mirror does, then one package at a time so errors are attributed.

| Level | Finding |
|---|---|
| error | Package, listed channel or `defaultChannel` not in the catalog. |
| error | oc-mirror's filter fails, e.g. a `minVersion` that empties the default channel; names the channels that would work, and explains that setting `defaultChannel` alone doesn't fix it. |
| error | A relevant channel has several heads or a cycle (OLMv0 can't resolve it). |
| warning | The package fails opm validation (e.g. stranded bundles); OLMv0 tolerates it. |
| warning | An unparseable `skipRange`, which OLM ignores. |
| warning | A `minVersion` that silently excludes prerelease-style versions (`7.9.0-2` vs `>=1.2.0`); suggests the `-0` form. |

#### 3. Upgrade paths (packages with a `minVersion`; skipped with `--offline`)

Treats `minVersion` as the version clusters run, the same assumption as [`isc operators optimise`](optimise-plan.md#assumption), stated in `--help` and the README. If the package lists `channels`, only those are checked.

| Level | Finding |
|---|---|
| error | No OLMv0 upgrade path from `minVersion` to the default channel's head. |
| error | A bundle on that path isn't mirrored. |
| warning | The same, for another mirrored channel. |
| `-v` | `isc operators optimise` would shrink this package (before and after counts, from [`isc preview`](preview-plan.md)'s counting, `internal/preview`). |

### Evidence

Every catalog and upgrade finding carries its evidence on the following stderr lines, indented: the channel, its head and the head's edges (`replaces`, `skips`, `skipRange`), the mirrored bundles, and the upgrade path.

### Flags

- `--offline`: static tier only. No catalog fetches, no network, no cache: a quick lint for pre-commit hooks or CI without registry credentials.
- `--strict`: warnings fail validation too (exit 1).
- Catalog flags as optimise and create: `--cache-dir`, `--authfile`, `--tls-verify`, `--platform`, `--fetch-timeout`.
- Several files: each is checked and every finding names its file; the exit code covers them all.

### Out of scope

- Whether additional images, platform release versions and helm charts exist or are reachable.
- Mirroring anything.

### Shared code

`internal/validate`, used by this command and by `isc operators optimise`: a candidate is valid if it passes the static and catalog tiers, and optimise's self-check before output runs all three tiers on the result.

## Upstream (oc-mirror)

- **Worth upstreaming: yes.**
  - The static tier is already oc-mirror's (`internal/pkg/config/validate.go`); isc runs a copy of it.
  - The catalog tier is oc-mirror's own filter (`internal/pkg/operator/catalog_handler.go`, catalog-filter) run early, before any image is copied. It would fit a `--dry-run` that stops at the first filter error, or an `oc-mirror validate` subcommand.
  - The upgrade tier would be warnings in that mode.
- **Still needed if oc-mirror followed the upgrade graph for operators: partly.** The static and catalog tiers are still needed; the upgrade-path tier isn't, because paths would be mirrored by construction.

## Open questions (proposed answers)

None yet; new questions go here.

## Verification

- **Static tier:** one fixture per oc-mirror `Validate` error, matching the cases in oc-mirror's `validate_test.go`; unknown fields (including `metadata`); a catalog-filter configuration that fails `Validate()`.
- **Catalog tier:** each error and warning in the table, on synthetic FBC fixtures in the shared `testdata/catalogs/`.
- **Upgrade tier:** no path; a path with an unmirrored bundle; the same on a non-default channel (warning); listed `channels` limit the check.
- **Tier gating:** a package that fails an earlier tier isn't checked further.
- **Flags:** `--offline` makes no fetch (the injected catalog source records no call); `--strict` turns a warning into exit 1; several files, each finding naming its file.
- **Output:** stdout has only `validation successful`, and only when valid; findings on stderr with their evidence; a failed fetch exits 1 with nothing on stdout.
- **Connected session:** real catalogs; an ISC that validate accepts also passes `oc-mirror --v2 --dry-run`, and one with a catalog-tier error fails it.

## What was and wasn't run

Planning only. oc-mirror's `Validate` rules were read in its source (`upstream/main` `4802bb8d`); the catalog and upgrade checks come from the earlier ISC CLI plan. No code exists yet.
