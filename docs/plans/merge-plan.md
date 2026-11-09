# `isc merge`: merging ImageSetConfigurations: plan

Status: **planned**, not started.

## Summary

`isc merge` combines several ISCs into one that mirrors at least what each of them mirrors. Its main use is combining the per-cluster ISCs from [`isc create --from-cluster`](create-plan.md): the result covers every operator installed on any cluster, each with the lowest `minVersion` installed anywhere on that catalog. It merges the whole file: operators, platform, additional images, blocked images, samples, helm and `archiveSize`.

```
isc merge [-v] FILE...
```

Input, output, messages and exit codes follow the tool plan's [shared behaviour](isc-tool-plan.md#shared-behaviour-every-command): every input is loaded as oc-mirror loads an ISC, and the merged ISC goes to stdout.

## Plan / Design decisions

### General rules

- Inputs are any valid ISCs; at least one.
- Keyed lists merge item by item on their key; fields that can be widened take the value that mirrors more; fields that pick a destination or behaviour must agree. "Must agree" means equal in every input that sets it, or set in only one input; otherwise an error naming both files and the field.
- Versions compare with `github.com/blang/semver/v4` (`ParseTolerant`, `Compare`). Two versions that compare equal but are written differently (`1.6`, `1.6.0`) keep the first input's spelling, in command-line order. A version that isn't semver is an error naming the file and item.
- "No `maxVersion` beats any `maxVersion`": an unbounded range mirrors more.

### Operators (`mirror.operators`)

- **Entries by `catalog`,** the full reference (tag or digest kept), normalized with `docker/reference` (`ParseNormalizedNamed`), so references written differently for the same image merge; the output keeps the first input's spelling. An unparseable reference is an error.
  - Keying by full reference matches oc-mirror: it refuses the same catalog twice but accepts the same repository with different tags (`validateOperatorOptions` in `internal/pkg/config/validate.go`, key from `GetUniqueName()` in `internal/pkg/api/v2alpha1/type_config.go`; tests `Invalid/DuplicateCatalogs`, `Invalid/DuplicateCatalogsWithTarget`; oc-mirror `3721eb58`).
  - So clusters on different OpenShift versions give separate entries per version (`...:v4.20`, `...:v4.21`), with the lowest `minVersion` taken within each. Making versions match, if wanted, happens outside isc.
- **Entry fields:** `full: true` wins. `targetCatalog`, `targetTag`, `targetCatalogSourceTemplate` and `skipDependencies` must agree. `platforms` (os/arch) are unioned.
- **Packages** unioned by `name`. The same package:
  - lowest `minVersion`; highest `maxVersion`, and none beats any;
  - `channels` unioned by `name`, but a package with no `channels` list (all channels) beats a list; channel-level `minVersion`/`maxVersion` merge the same way;
  - `defaultChannel` must agree.
- The same package in two catalogs (e.g. Red Hat and community) stays as two entries; `-v` lists it, since it's often a mistake on a cluster.

### Platform (`mirror.platform`)

- **Channels** by `name` + `type`: lowest `minVersion`, highest `maxVersion` (none beats any); `shortestPath` and `full` true win.
- `graph` and `kubeVirtContainer`: true wins. `architectures` and `platforms`: unioned.
- `release` must agree.

### Additional images, blocked images, samples

- Union by `name`.
- `additionalImages` with the same name: `tagsByRegex`, `targetRepo` and `targetTag` must agree; `platforms` unioned.

### Helm (`mirror.helm`)

- `repositories` by `name`; a different `url` for the same name is an error.
- Charts within a repository, and `local` charts, by `name` + `version`; the same chart: `imagePaths` unioned, `path` must agree; `platforms` unioned.

### `archiveSize`

Smallest wins.

### Output

- A complete ISC; comments aren't carried over (the output is a new document built from several).
- **Deterministic:** keyed lists sorted by key (catalogs by reference, packages and channels by name, images by name, ...); plain lists (`architectures`, `imagePaths`) deduplicated and sorted. Merging the same inputs in any order gives the same output, apart from the first-input spelling rule.
- Checked with the oc-mirror-style loader before it's written.
- **`-v`:** one line per item that came from more than one input, with the value kept and where the others came from, e.g.

  ```
  compliance-operator registry.redhat.io/redhat/redhat-operator-index:v4.21 minVersion 1.6.0 from cluster-a.yaml (cluster-b.yaml 1.7.0, cluster-c.yaml 1.6.0)
  ```

  The input file name stands for the cluster: oc-mirror rejects unknown ISC fields, so `create` can't record the cluster name inside the ISC.

## Open questions (proposed answers)

1. **Merging a merge output with more ISCs:** the result is a valid ISC like any other, so it works. Proposed: support it and test it; merging is associative.

## Verification

- **Fixtures**, each with expected output and `-v` lines, per section:
  - operators: an overlapping package (lowest `minVersion`); `maxVersion` highest, and none beats any; channels unioned, and no channels list beats a list; channel-level versions; `full: true` wins; equal versions written differently; a prerelease below its release (`1.0.0-rc.1` < `1.0.0`); numeric ordering (`1.9.0` < `1.10.0`); the same catalog on two tags stays two entries; differently written references for one image merge; a digest reference; a registry with a port; the same package in two catalogs; `platforms` unioned;
  - platform: channels by name + type; `graph` and `kubeVirtContainer`; `architectures` unioned;
  - additional images, blocked images, samples: union by name;
  - helm: repositories by name; charts by name + version with `imagePaths` unioned;
  - `archiveSize`: smallest kept.
- **Errors** (exit 1, nothing on stdout), each naming both files: different `targetCatalog`, `targetTag`, `targetCatalogSourceTemplate`, `skipDependencies` or `defaultChannel`; different `release`; an additional image with a different `tagsByRegex`, `targetRepo` or `targetTag`; a helm repository with a different `url`; a chart with a different `path`; a version that isn't semver; an input the loader rejects (e.g. `metadata`).
- **Properties:** any input order gives the same output; merging the output with a further ISC equals merging them all at once; merging one file returns it, sorted.
- **End to end:** `isc create --from-cluster` on two real clusters, `isc merge`, `isc operators optimise`, then `oc-mirror --v2 --dry-run`.

## What was and wasn't run

Planning only. No merge code exists yet.
