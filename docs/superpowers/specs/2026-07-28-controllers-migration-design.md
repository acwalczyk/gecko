# Controllers Migration Design

**Date:** 2026-07-28
**Status:** Approved

## Overview

Migrate the five adapter controllers from `github.com/openshift-hyperfleet/hyperfleet-adapters-go`
into this repository as a new `controllers/` Go module. Update all type references from the old
`github.com/thetechnick/orlop-gcp-hcp/api/private/v1` package to the canonical types in
`github.com/openshift-online/gecko/platform-api/api/private/v1`.

This is phase 1 of 2. Phase 2 (planned separately) will replace the direct Orlop HTTP connection
with Kubernetes API aggregation as described in the gecko-api-aggregation design decision.

## Module Structure

New top-level module at `controllers/` with its own `go.mod`:

```
gecko/
├── orlop/             (existing)
├── platform-api/      (existing)
└── controllers/       (new)
    ├── go.mod
    ├── Makefile
    ├── Dockerfile
    ├── main.go                      # root CLI entrypoint, registers subcommands
    ├── cmd/
    │   ├── versionresolution/       # NewCommand() for version-resolution subcommand
    │   ├── nodepoolvrresolution/    # NewCommand() for nodepool-vr subcommand
    │   ├── placement/               # NewCommand() for placement subcommand
    │   ├── hc/                      # NewCommand() for hc subcommand
    │   └── nodepool/                # NewCommand() for nodepool subcommand
    ├── versionresolution/           # Cluster version resolution reconciler
    ├── nodepoolvrresolution/        # NodePool version resolution reconciler
    ├── placement/                   # Cluster placement reconciler + selector
    ├── hc/
    │   ├── hc_controller.go
    │   └── manifest/                # HostedCluster ManifestWork builder
    ├── nodepool/
    │   ├── nodepool_controller.go
    │   └── manifest/                # NodePool ManifestWork builder
    ├── client/
    │   ├── maestro/                 # Maestro HTTP + gRPC client
    │   └── transport/               # transport.Client interface + maestro/mock impls
    └── util/
        ├── constants/               # Annotation/label keys, resource constants
        ├── errors/                  # Typed error hierarchy
        ├── logger/                  # Structured logger (slog-backed, context-aware)
        └── version/                 # Binary version (set via ldflags)
```

### Conventions (following HyperShift)

- No `internal/` directory — flat package layout
- Controller files named `{resource}_controller.go`
- Reconciler structs named `{Resource}Reconciler`
- Domain clients in `client/`, generic utilities in `util/`
- Adding a new controller = new top-level package + new `cmd/` subpackage + one line in `main.go`

## Module Dependencies

`controllers/go.mod` module name: `github.com/openshift-online/gecko/controllers`

References `platform-api` types via a `replace` directive (same pattern `platform-api` uses for `orlop`):

```
require github.com/openshift-online/gecko/platform-api v0.0.0
replace github.com/openshift-online/gecko/platform-api => ../platform-api
```

## Type Reference Migration

All references to `github.com/thetechnick/orlop-gcp-hcp/api/private/v1` are replaced with
`github.com/openshift-online/gecko/platform-api/api/private/v1`.

The types (`Cluster`, `NodePool`, `ClusterSpec`, `NodePoolSpec`, etc.) are structurally
compatible — this is an import path change, not a structural change.

## Controllers

Five controllers are migrated as-is. No behavioural changes in this phase.

| Subcommand          | Watches    | Responsibility                                      |
|---------------------|------------|-----------------------------------------------------|
| `version-resolution`| Cluster    | Resolves release version → image via Cincinnati     |
| `nodepool-vr`       | NodePool   | Same version resolution for node pools              |
| `placement`         | Cluster    | Selects management cluster and DNS base domain      |
| `hc`                | Cluster    | Creates/updates HostedCluster ManifestWork via Maestro |
| `nodepool`          | NodePool   | Creates/updates NodePool ManifestWork via Maestro   |

Each controller connects to the Orlop API server via `--orlop-url` (default: `http://hyperfleet-api:8080`)
using a controller-runtime Manager and standard `rest.Config`. This connection mechanism is
unchanged in phase 1 and will be replaced with API aggregation in phase 2.

## Binary

Single binary `gecko-controllers` built from `controllers/main.go`. Each controller is a Cobra
subcommand. Deployed as independent pods, each running a different subcommand.

## Deployment

Independent from the `platform-api-server`. The existing `Dockerfile` at the repo root builds
`platform-api-server` only. A new `controllers/Dockerfile` builds `gecko-controllers`.

## What Is Not In Scope

- Behavioural changes to any controller
- API aggregation / auth mechanism changes (phase 2)
- CI workflow for building/pushing the controllers image (follow-up PR, same pattern as PR #10)