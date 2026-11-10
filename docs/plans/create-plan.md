# `isc create --from-cluster`: generating an ImageSetConfiguration from a cluster: plan

Status: **planned**, not started.

## Summary

Read what a cluster has installed (OLM v0 Subscriptions and OLM v1 ClusterExtensions), and output an ImageSetConfiguration (ISC) on stdout that pins each installed operator to its installed version.

```
isc create --from-cluster [kubectl flags: --kubeconfig, --context, ...] [--cache-dir DIR] [--authfile FILE] [--tls-verify=false] [--fetch-timeout SECONDS] [-v]
```

`create` is a top-level command because it makes a new ISC rather than editing one; `--from-cluster` names the source, so other sources can be added later as other `--from-...` flags. For now `--from-cluster` is the only source and is required (without it: usage error, exit 2).

It's the first step of the pipeline:

1. `isc create --from-cluster` for each cluster (this plan);
2. [`isc merge`](merge-plan.md) the results: every operator, lowest `minVersion` per catalog;
3. optionally, your own changes outside isc (see the [tool plan](isc-tool-plan.md#scope));
4. [`isc operators optimise`](optimise-plan.md): the smallest selection that still lets clusters upgrade;
5. [`isc validate`](validate-plan.md);
6. oc-mirror.

## Plan / Design decisions

### What it produces

For each installed operator, the ISC gives only:
- **catalog image:** the source catalog, with its tag set to the cluster's OpenShift version, e.g. `registry.redhat.io/redhat/redhat-operator-index:v4.21` for a 4.21.9 cluster;
- **package name;**
- **version:** the installed version, as a package-level `minVersion`.

No channels, `maxVersion` or other fields. The output is a complete ISC (`apiVersion`, `kind`, `mirror.operators`) with one entry per catalog, and never a `metadata` block (oc-mirror rejects it).

`isc merge` keys catalogs by full reference, so clusters on different OpenShift versions give separate catalog entries (`...:v4.20`, `...:v4.21`).

### Reading the cluster

- **Go, in this program,** not an Ansible role: the ClusterExtension catalog choice needs the catalog access `isc operators optimise` already has (`internal/catalog`). An Ansible role can still call `isc create --from-cluster` per inventory host.
- **Which cluster: exactly as kubectl.** The kubeconfig is loaded with the same rules and flags as `kubectl`, through `k8s.io/cli-runtime`'s `genericclioptions.ConfigFlags` (what kubectl itself uses):
  - by default, the **current context** of the kubeconfig: `--kubeconfig`, else the `KUBECONFIG` list (merged), else `~/.kube/config`, else in-cluster config;
  - `--context NAME` selects another context;
  - the other standard flags (`--cluster`, `--user`, `--server`, `--token`, `--insecure-skip-tls-verify`, `--certificate-authority`, `--request-timeout`, ...) behave as in kubectl.
  - `-v` keeps its meaning here (decisions on stderr); kubectl's `-v` log level isn't added.
  - Messages name the context and API server used, so it's clear which cluster was read.
- **Client:** `k8s.io/client-go`'s dynamic client, built from the `ConfigFlags` REST config, with `unstructured` objects. That avoids importing operator-controller's main module and its dependency graph just for two API types. Label selection uses `k8s.io/apimachinery` `metav1.LabelSelectorAsSelector`, the same call the resolver makes.
- **Read-only:** list and get only. No credentials in the repo or the output.

### Where the data comes from

- **OpenShift version:** `ClusterVersion` `version`, `status.desired.version`, truncated to `v<major>.<minor>`.
- **OLM v0 Subscriptions** (`subscriptions.operators.coreos.com`, all namespaces):
  - package: `spec.name`;
  - catalog: the CatalogSource named by `spec.source` / `spec.sourceNamespace`, using its `spec.image`;
  - version: `spec.version` of the CSV named in `status.installedCSV`.
- **OLM v1 ClusterExtensions** (`clusterextensions.olm.operatorframework.io`):
  - package: `spec.source.catalog.packageName`;
  - version: `status.install.bundle.version`;
  - catalog: not recorded, so it's worked out by repeating the resolver's choice (see [Which catalog a ClusterExtension installed from](#which-catalog-a-clusterextension-installed-from)). The catalog image is the chosen ClusterCatalog's `spec.source.image.ref`.
  - Also available, as a cross-check: the installed **bundle image**. The controller records it as the `olm.operatorframework.io/bundle-reference` annotation on the revision it applies (a ClusterObjectSet, or the labels of the Helm release Secret), next to `bundle-name`, `bundle-version` and `package-name`. Source: operator-controller `internal/shared/labels/labels.go` and `ApplyBundle` in `internal/operator-controller/controllers/clusterextension_reconcile_steps.go`.
- **Skipped,** with a stderr line each:
  - a Subscription with no installed CSV yet;
  - a CatalogSource built from a gRPC address rather than an image;
  - a ClusterExtension that isn't installed.

### Which catalog a ClusterExtension installed from

`status` only has `conditions` and `install.bundle` (`name`, `version`, optional `release`); see operator-controller `api/v1/clusterextension_types.go`. The resolver logs the chosen catalog at debug level only, and names catalogs in error messages only.

But the resolver's choice is deterministic (`internal/operator-controller/resolve/catalog.go`). From the ClusterCatalogs that `spec.source.catalog.selector` matches (all of them if there's no selector), it takes those containing the package. Among those, it picks the highest `spec.priority`. A tie is an error ("found in multiple catalogs with the same priority").

So `create` repeats that choice:
- if only one ClusterCatalog matches the selector, use it;
- otherwise load each candidate's catalog through `internal/catalog`, with the same cache and flags as `isc operators optimise` (`--cache-dir`, `--authfile`, `--tls-verify`, `--fetch-timeout`; see [validate-plan.md](validate-plan.md#internalcatalog)). The candidates may be in a mirror registry with its own credentials or CA. Keep those that have the package and the installed bundle name, and take the highest priority;
- cross-check that the bundle's image equals the recorded `bundle-reference`;
- a tie, no match, or a `bundle-reference` mismatch is `review`.

### Catalog tags

Any catalog whose tag matches `^v[0-9]+\.[0-9]+$` is retagged to the cluster's version, not only the four Red Hat default catalogs. Other references are kept as they are, with a stderr note, because their tags don't follow OpenShift.

### Mirrored catalogs

Disconnected clusters point at the mirror, not the source: their CatalogSources reference something like `mirror.example.com/redhat/redhat-operator-index:v4.21`. Mirrored and source names wouldn't merge with each other.

- `create` writes catalog images exactly as the cluster references them (apart from the version tag rule above). It doesn't map mirrors back to their source.
- Rewriting references, e.g. a mirror prefix back to `registry.redhat.io`, is out of scope: do it outside the tool (`sed`, `yq`, or a hand edit) before `isc merge`.

### Two installs of one package

Two Subscriptions in different namespaces with different versions: keep the lowest, the same rule as `isc merge`.

### Output and messages

- Output on stdout, as every command does ([shared behaviour](isc-tool-plan.md#shared-behaviour-every-command)).
- **`-v`:** each package found and where it came from: namespace/Subscription or ClusterExtension, CatalogSource or ClusterCatalog, and CSV or bundle. Every package is reported, so a missing operator is easy to spot.

## Upstream (oc-mirror)

- **Worth upstreaming: partly.** Generating an ISC from what a cluster runs is a natural companion to oc-mirror (or an `oc adm` command), but oc-mirror never talks to clusters today: it reads an ISC and registries. An `oc` plugin may fit better than oc-mirror itself.
- **Still needed if oc-mirror followed the upgrade graph for operators: yes.** It supplies the installed versions that such a mode would start from.

## Open questions (proposed answers)

1. **Package-level vs channel `minVersion`:** a package-level range applies to every channel of the package in oc-mirror's filter.
   - Proposed: package-level. `isc operators optimise` then considers every channel with a bundle at or above that version, so a subscribed non-default channel is still covered.
   - Note: recording the subscribed channel would let optimise narrow the scope to it (listed `channels` are optimise's scope). Revisit after trying real clusters.

## Verification

- **Fixtures** of `oc get -o json` output, served through a fake dynamic client (`k8s.io/client-go/dynamic/fake`), with the catalog source injected as in `isc operators optimise`'s tests:
  - a Subscription with a CSV, without a CSV, and on a gRPC CatalogSource;
  - two Subscriptions for one package (lowest kept);
  - a ClusterExtension that is installed, not installed, matching one catalog, matching several catalogs with different priorities, tied on priority, and with a `bundle-reference` that doesn't match;
  - a catalog tag that follows OpenShift and one that doesn't;
  - a mirrored CatalogSource, written as the cluster references it.
- **Kubeconfig selection:** the current context by default, `--context`, `KUBECONFIG` with two files merged, `--kubeconfig` overriding `KUBECONFIG`, an unknown context (error); tested on the `ConfigFlags` result, without a cluster.
- **Usage:** no `--from-cluster` is a usage error (exit 2).
- **Shared failure paths:** an unreadable kubeconfig, an unreachable API server, a failed catalog fetch: exit 1, nothing on stdout.
- **End to end:** run it on a real cluster, read-only, then `isc merge`, `isc operators optimise`, `isc validate` and `oc-mirror --v2 --dry-run`.

## What was and wasn't run

Planning only. The ClusterExtension and ClusterCatalog API types, and the resolver and label code, were read in the operator-controller source. No code for it exists yet, and no cluster was available.
