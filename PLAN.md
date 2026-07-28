# Plan: Cluster/NodePool Types + Nested Routes + Storage-Level Parent Filtering

## Status

| Phase | Status | Notes |
|-------|--------|-------|
| 1     | Done   | No drifts |
| 2a    | Done    | No drifts |
| 2b    | Done    | No drifts |
| 2c    | Done    | No drifts |
| 3     | Done    | No drifts |
| 4     | Done    | No drifts |
| 5a    | Done    | parentFilterMiddleware in router_helper.go |
| 5b    | Done    | No drifts |
| 5c    | Done    | No drifts |
| 6     | Done    | No drifts |
| 7     | Done    | Dropped mhc-scheduler entirely |

### Drifts / Decisions
- **Phase 5a**: `parentFilterMiddleware` placed in `router_helper.go` instead of `router.go` due to identical function-ending blocks in both `setupRouter` and `setupConvertingRouter` making targeted edits ambiguous. Same package, no behavioral difference.
- **Phase 7**: `mhc-scheduler` dropped entirely — it was a demo controller for the old `ManagedHostedCluster` type and is no longer needed.

---

## Context

The `platform-api` currently serves placeholder `ManagedHostedCluster` types (group `gcp.orlop.thetechnick.ninja/v1alpha1`) modeled on HyperShift. PR #89 in the old `gcp-hcp` experiments repo introduced the real `Cluster` and `NodePool` types (group `hyperkube/v1`) with nested route support (e.g., `/clusters/{id}/nodepools`). This plan ports those changes to gecko AND adds two items PR #89 was missing: storage-level parent filtering and complete OpenAPI spec coverage for nested/status routes.

The old v1alpha1 types will be removed entirely, along with the `github.com/openshift/hypershift/api` dependency.

---

## Phase 1: `ParentResourceInfo` type definition

**File:** `orlop/pkg/apiserver/types/types.go`

Add `ParentResourceInfo` struct (fields: `Plural string`, `IDField string`) and add `ParentResource *ParentResourceInfo` to `ResourceInfo`. This is the foundation everything else depends on.

---

## Phase 2: Extend storage for field-based filtering

### 2a. `ListOptions` — add `FieldFilters`

**File:** `orlop/pkg/apiserver/storage/interface.go`

Add `FieldFilters map[string]string` to `ListOptions`. Keys are dot-separated JSON paths (e.g., `spec.clusterID`), values are expected string values. This is more general than a parent-specific filter and can serve future use cases.

### 2b. Postgres `QueryBuilder` — new `WhereFieldFilters`

**File:** `orlop/pkg/apiserver/storage/postgres/querybuilder.go`

Add `WhereFieldFilters(filters map[string]string)` that generates JSONB path queries: `data->'spec'->>'clusterID' = $N`. The path parts come from server config (not user input), so no injection risk; values are parameterized.

**File:** `orlop/pkg/apiserver/storage/postgres/store.go`

Call `qb.WhereFieldFilters(opts.FieldFilters)` in `buildListQuery()`. In `Watch()`, filter the event stream: after receiving each event from the database notification channel, unmarshal the object and apply field filter matching before emitting to the watcher. Events that don't match are silently dropped — the watcher never sees them.

### 2c. Memory store — field filter in List and Watch

**File:** `orlop/pkg/apiserver/storage/memory/store.go`

Add field filter step in `List()` after the shard filter check — JSON-marshal each object, walk the dot path, compare. Add a `matchesFieldFilters` helper. In `Watch()`, apply the same `matchesFieldFilters` check on each event object before emitting — drop non-matching events silently so watchers only see objects that satisfy the filter.

---

## Phase 3: Parent filter context plumbing

**New file:** `orlop/pkg/apiserver/handlers/parent_filter.go`

Contains:
- `ParentFilter` struct (`IDField`, `ID`)
- `WithParentFilter(ctx, filter)` — sets context value
- `parentFilterFromContext(ctx)` — retrieves from context
- `fieldValueFromMap(m map[string]interface{}, path string) string` — dot-path JSON value extraction (used for Create validation where we already have an unmarshalled map)

---

## Phase 4: Handler changes for parent filter

### 4a. `handlers/resource.go`

- **`List()`**: After building `ListOptions`, check `parentFilterFromContext(r.Context())` and set `opts.FieldFilters[pf.IDField] = pf.ID`. The storage layer handles the actual filtering.
- **`Create()`**: After unmarshalling the object but before storing, validate that `fieldValueFromMap(objMap, pf.IDField) == pf.ID`. Return 400 on mismatch.
- **`Get()`, `Update()`, `Patch()`, `Delete()`**: After fetching the object from store, validate parent ownership if parent filter is in context. Return 404 on mismatch.

### 4b. `handlers/converting.go`

The `ConvertingResourceHandler` delegates to an inner `ResourceHandler`. Because the parent filter is injected via context (by the router middleware in Phase 5a), and the inner handler already reads from context in Phase 4a, the converting layer does **not** need to re-check parent filters. It only needs to pass the request context through unmodified, which it already does. No changes needed in converting.go for parent filtering — the inner handler handles it.

---

## Phase 5: Router + OpenAPI discovery

### 5a. Nested routes in `router.go`

**File:** `orlop/pkg/apiserver/router.go`

Add `parentFilterMiddleware(idField, urlParam string)` — extracts the parent ID from the chi URL param and injects `ParentFilter` into context.

In both `setupRouter()` and `setupConvertingRouter()`, when iterating namespaced handlers: if `res.ParentResource != nil`, register an additional route group under `/namespaces/{namespace}/{parentPlural}/{parentID}/{childPlural}` with the parent filter middleware. The flat routes for the child remain (for listing all children across parents).

### 5b. OpenAPI v3 nested + status paths

**File:** `orlop/pkg/apiserver/handlers/discovery.go`

In `OpenAPIV3GroupVersion()`: when a resource has `ParentResource` set, generate additional path entries for the nested routes (list/create on base, get/put/patch/delete on `/{name}`, put on `/{name}/status`). Also ensure `/status` subresource paths exist for both flat and nested patterns.

### 5c. OpenAPI v2 nested + status paths

In `buildOpenAPIV2Spec()`: same as 5b but in Swagger 2.0 format. For both v2 and v3, nested path variants must have unique operation IDs (e.g., `listNamespacedNodePoolForCluster` vs `listNamespacedNodePool`) and include the parent ID path parameter definition with proper schema/description.

---

## Phase 6: New Cluster/NodePool types in platform-api

### 6a. Remove old v1alpha1 types

Delete `platform-api/api/private/v1alpha1/` and `platform-api/api/public/v1alpha1/` directories entirely. Remove the `github.com/openshift/hypershift/api` dependency from `go.mod`.

### 6b. Create private types — `api/private/v1/`

Source: the type definitions from PR #89, adapted to gecko's module path (`github.com/openshift-online/gecko/platform-api/api/private/v1`) and orlop import path (`github.com/openshift-online/gecko/orlop/...`). The `zz_generated.*` files should be **regenerated** using `orlop-gen` (not copied verbatim) since module paths differ from the original PR.

Files to create:
- `groupversion_info.go` — group `hyperkube`, version `v1`
- `cluster_types.go` — `Cluster`, `ClusterList`, `ClusterSpec` (infraID, issuerURL, platform, release, networking, dns), `ClusterStatus` (conditions, placementResult, hostedClusterResult, versionResolution), and all nested structs. Fields tagged `+orlop:public` except `versionResolution`.
- `nodepool_types.go` — `NodePool`, `NodePoolList`, `NodePoolSpec` (clusterID, platform, release, nodeCount, autoscaling, nodeLabels, taints), `NodePoolStatus` (conditions, versionResolution). `versionResolution` is private-only.
- `zz_generated.deepcopy.go` — from PR #89, updated package path
- `zz_generated.schemas.go` — from PR #89, updated import to `github.com/openshift-online/gecko/orlop/pkg/apiserver/types`
- `.schemas/cluster_schema.yaml` — from PR #89
- `.schemas/nodepool_schema.yaml` — from PR #89

### 6c. Create public types — `api/public/v1/`

Same set of `zz_generated.*` files from PR #89, updated import paths. Public types omit `versionResolution` from both Cluster and NodePool status.

- `zz_generated.groupversion_info.go`
- `zz_generated.cluster_types.go`
- `zz_generated.nodepool_types.go`
- `zz_generated.deepcopy.go`
- `zz_generated.schemas.go`
- `.schemas/cluster_schema.yaml`
- `.schemas/nodepool_schema.yaml`

---

## Phase 7: Wiring + Dockerfile

### 7a. Update `resources.go`

**File:** `platform-api/cmd/platform-api-server/resources.go`

Replace v1alpha1 references with v1. Wire `privatev1.GetResourceInfos()` and `publicv1.GetResourceInfos()`. Set `ParentResource` on NodePool entries:
```go
ParentResource: &types.ParentResourceInfo{Plural: "clusters", IDField: "spec.clusterID"}
```

Register schemes: `privatev1.AddToScheme(scheme)`, `publicv1.AddToScheme(scheme)`.

### 7b. Update `main.go` if needed

**File:** `platform-api/cmd/platform-api-server/main.go` — likely no changes needed since it delegates to `getPrivateResources()` etc.

### 7c. Update `go.mod`

Remove `github.com/openshift/hypershift/api` and any indirect deps it pulled in. Also remove the `api/private/v1beta1hs/` scaffolding directory, which only contains empty subdirectories for HyperShift CRD manifests and has no Go source files — it exists solely because of the hypershift dependency. Run `go mod tidy`.

### 7d. Add Dockerfile

**New file:** `Dockerfile` (at repo root, builds from both orlop/ and platform-api/)

Standard multi-stage distroless build.

---

## Verification

1. **Orlop unit tests:** `cd orlop && make test` — existing tests pass (no regressions from new optional fields)
1a. **New orlop unit tests** (required, not optional):
   - **Postgres `QueryBuilder`**: test `WhereFieldFilters` generates correct JSONB path queries for single and multi-level dot paths, handles empty filters as no-op, and parameterizes values correctly.
   - **Memory store**: test `List()` with `FieldFilters` returns only matching objects; test `matchesFieldFilters` with nested paths, missing fields, and type mismatches.
   - **Parent filter middleware**: test `parentFilterMiddleware` extracts the URL param and injects `ParentFilter` into context; test round-trip with `parentFilterFromContext`.
   - **Handler parent validation**: test `Create()` rejects objects whose field value doesn't match the parent filter (400); test `Get()`/`Update()`/`Delete()` return 404 when the fetched object doesn't belong to the parent.
2. **Build platform-api:** `cd platform-api && go build ./...`
3. **Run server locally:** `cd platform-api && go run ./cmd/platform-api-server`
4. **Smoke test — flat routes:**
   ```bash
   # Create cluster
   curl -X POST http://localhost:8080/apis/hyperkube/v1/namespaces/default/clusters -d '{"metadata":{"name":"c1"},"spec":{...}}'
   # Create nodepool
   curl -X POST http://localhost:8080/apis/hyperkube/v1/namespaces/default/nodepools -d '{"metadata":{"name":"np1"},"spec":{"clusterID":"c1",...}}'
   # List all nodepools
   curl http://localhost:8080/apis/hyperkube/v1/namespaces/default/nodepools
   ```
5. **Smoke test — nested routes:**
   ```bash
   # List nodepools for cluster c1
   curl http://localhost:8080/apis/hyperkube/v1/namespaces/default/clusters/c1/nodepools
   # Create nodepool via nested route
   curl -X POST http://localhost:8080/apis/hyperkube/v1/namespaces/default/clusters/c1/nodepools -d '{"metadata":{"name":"np2"},"spec":{"clusterID":"c1",...}}'
   # Reject mismatched parent
   curl -X POST http://localhost:8080/apis/hyperkube/v1/namespaces/default/clusters/c1/nodepools -d '{"metadata":{"name":"np3"},"spec":{"clusterID":"c2",...}}'
   # Should return 400
   ```
6. **OpenAPI verification:**
   ```bash
   curl http://localhost:8080/openapi/v3/apis/hyperkube/v1 | jq '.paths | keys'
   # Should include nested paths and /status paths
   ```
7. **Public API filtering:**
   ```bash
   curl http://localhost:8081/apis/hyperkube/v1/namespaces/default/clusters/c1/nodepools
   # Should show only nodepools for c1, with private fields stripped
   ```
