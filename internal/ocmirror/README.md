# Pinned copy of oc-mirror code

The Go code under this directory is copied from [oc-mirror](https://github.com/openshift/oc-mirror) v2, at the commit in [`PIN`](PIN). oc-mirror keeps all of its packages under `internal/`, which other Go modules can't import, so isc keeps a copy instead of reimplementing oc-mirror's behaviour.

oc-mirror is licensed under the Apache License, Version 2.0: see [`LICENSE`](LICENSE). Its source files carry no license headers, so this file is the attribution.

## What's copied

| Here | From oc-mirror |
|---|---|
| `config/` | `internal/pkg/config/`: `load.go`, `validate.go`, `defaults.go`, `metadata.go` and the tests `load_test.go`, `validate_test.go`, `defaults_test.go` (not `pin_catalogs.go`, which needs oc-mirror's mirror, manifest and log packages) |
| `api/v2alpha1/` | `internal/pkg/api/v2alpha1/`, every file |
| `image/` | `internal/pkg/image/`, every file |
| `consts/` | `internal/pkg/consts/`, every file |
| `../../tests/unit/testdata/` | the four files `config/load_test.go` reads through `consts.TestFolder` |
| `LICENSE` | `LICENSE` |

## Changes

The only change is the import paths: `github.com/openshift/oc-mirror/v2/internal/pkg/` becomes `github.com/ayjM6/isc/internal/ocmirror/`. `pin.go` is generated and records the commit.

## Updating

Don't edit the copied files by hand. Change the commit and re-sync:

```
just sync-ocmirror <commit>     # writes PIN and pin.go too
just test
```

Then align `go.mod` with the versions oc-mirror's `go.mod` pins at that commit. CI runs `just check-sync`, which fails if the copy differs from a fresh sync at `PIN`.
