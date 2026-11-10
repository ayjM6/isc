# isc

`isc` builds, checks and edits [oc-mirror](https://github.com/openshift/oc-mirror) v2 ImageSetConfigurations (ISCs). It loads ISCs with oc-mirror's own code, so it accepts exactly what oc-mirror accepts.

Status: early development. Only `isc version` exists so far; the planned commands are described in [docs/plans/](docs/plans/).

## Install

With Go:

```
go install github.com/ayjM6/isc/cmd/isc@latest
```

Or download a release tarball for linux/amd64 or linux/arm64 from the [releases page](https://github.com/ayjM6/isc/releases), check it against `checksums.txt`, and extract the `isc` binary.

## Usage

```
isc version
```

Planned pipeline:

1. `isc create --from-cluster > cluster-a.yaml` for each cluster;
2. `isc merge cluster-*.yaml > merged.yaml`;
3. your own changes, with other tools or by hand;
4. `isc operators optimise merged.yaml > isc.yaml`;
5. `isc validate isc.yaml` (and `isc preview isc.yaml` to see what will be mirrored);
6. `oc-mirror --v2 -c isc.yaml ...`.

## Development

Needs Go (the version in `go.mod`) and [just](https://just.systems). Release previews also use [git-cliff](https://git-cliff.org); goreleaser runs through `go run`.

```
just test              # gofmt check, go vet, go test
just build             # dist/isc
just run version       # build and run with arguments
just sync-ocmirror     # refresh the pinned oc-mirror copy (internal/ocmirror/)
just release-snapshot  # build every release target locally, without publishing
```

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org/); they drive the version and the release notes, and CI checks them on pull requests.

## Releases

Releases come only from semantic version tags (`vMAJOR.MINOR.PATCH`, optionally `-rc.N`):

1. Run the **Cut release** action on `main` (choose the bump, or leave `auto`). It works out the version from the commits with git-cliff, tags the head of `main`, and runs the release for that tag.
2. The **Release** action (also run by pushing a `v*` tag yourself) tests, then goreleaser builds the linux/amd64 and linux/arm64 tarballs and publishes a GitHub release, with git-cliff's notes as its description.

The release notes on GitHub are the changelog; there's no `CHANGELOG.md`.

To preview locally: `just next-version` and `just changelog` (need [git-cliff](https://git-cliff.org)), `just release-snapshot` (runs goreleaser with `go run`, nothing to install).

## License

ISC; see [LICENSE](LICENSE). `internal/ocmirror/` contains code copied from oc-mirror under the Apache License 2.0; see [internal/ocmirror/README.md](internal/ocmirror/README.md).
