# API Aggregation for Gecko's Internal API

## Summary

Replace gecko's private API HTTP layer with a Kubernetes aggregated API server (`k8s.io/apiserver`'s `GenericAPIServer`). Internal consumers (controllers, admin users, SRE, monitoring, GCP workflows) access gecko through the GKE kube apiserver, which handles authentication, authorization, and audit logging. The external/public API remains unchanged as a standalone HTTP server.

## Motivation

Gecko currently reimplements a significant portion of Kubernetes API server functionality:

- **Authentication**: SA token validation against a Secret store (`orlop/pkg/apiserver/authn/`). Tokens are stored as `kubernetes.io/service-account-token` secrets in gecko's own datastore. Anonymous requests pass through as `system:anonymous`. Token rotation and lifecycle must be managed manually.
- **Authorization**: Full RBAC engine (`orlop/pkg/apiserver/rbac/`) with Role, ClusterRole, RoleBinding, ClusterRoleBinding — all stored as resources in gecko's datastore, under API group `rbac.orlop.gcp.managed.openshift.io/v1`.
- **API discovery, watch protocol, content negotiation, health endpoints**: All custom implementations.

With API aggregation, the GKE kube apiserver handles authn/authz/audit for internal traffic. Gecko's custom auth stack and its operational burden (token provisioning, rotation, revocation, security patching, audit trail) are eliminated for the internal path.

## What API Aggregation Is

Kubernetes API aggregation allows registering a custom API server with the cluster's kube apiserver via an `APIService` resource. The kube apiserver then proxies requests for registered API groups to the custom server:

```
Controller (client-go)
    → GKE kube apiserver (authn, authz, audit)
        → proxy (APIService routing)
            → Gecko aggregated API server
                → ResourceStore (Postgres / Spanner)
```

Reference: https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/apiserver-aggregation/

## What Changes

### 1. Replace Private Port HTTP Server with `GenericAPIServer`

**Current** (`orlop/pkg/apiserver/server.go`):
- `Server` struct wraps two `*http.Server` instances (private port 8080, public port 8081)
- Private server uses a `chi.Router` with custom middleware chain
- `ListenAndServe()` — plain HTTP, no TLS

**Target**:
- Private port becomes a `GenericAPIServer` from `k8s.io/apiserver/pkg/server` — standalone chi.Router mode for private API is removed entirely
- TLS required (aggregated API servers must serve HTTPS; the kube apiserver proxies over TLS and validates the serving certificate)
- Local development uses `--disable-auth` to skip token/SAR delegation (same `GenericAPIServer` code path, no kube-apiserver dependency)
- The `Server` struct manages both servers: `GenericAPIServer` for private, `chi.Router` for public

**Key steps**:
- Create `apiserver.RecommendedConfig()` or `apiserver.NewConfig()` with:
  - TLS cert/key configuration (can use self-signed + CA bundle in `APIService.spec.caBundle`)
  - Delegated authentication via `--authentication-kubeconfig` or in-cluster config (validates tokens via `TokenReview` against the GKE kube apiserver)
  - Delegated authorization via `--authorization-kubeconfig` or in-cluster config (checks permissions via `SubjectAccessReview`)
- Call `config.Complete().New("gecko-platform-api", ...)` to get a `GenericAPIServer`
- Register API groups via `server.InstallAPIGroup()` using `rest.Storage` adapters (see below)

**Files affected**:
- `orlop/pkg/apiserver/server.go` — `Server` struct uses `GenericAPIServer` for private API. Public side stays as-is (`chi.Router` + `http.Server`)
- `orlop/pkg/apiserver/aggregated/config.go` — `GenericAPIServer` configuration builder
- `orlop/pkg/apiserver/aggregated/apiserver.go` — aggregated server type wrapping `GenericAPIServer`

### 2. Implement `rest.Storage` Adapter Layer

**Current**: `ResourceHandler` (`orlop/pkg/apiserver/handlers/resource.go`) is a plain HTTP handler struct. It receives `http.ResponseWriter`/`*http.Request`, parses JSON, calls `storage.ResourceStore`, writes JSON responses. All logic (schema validation, metadata stamping, finalizer flow, generation tracking, owner ref validation) lives in handler methods.

**Target**: Adapter types that wrap `storage.ResourceStore` and implement `k8s.io/apiserver/pkg/registry/rest` interfaces:

```go
// rest.Storage (base)
// rest.Getter — Get(ctx, name, options) → (runtime.Object, error)
// rest.Lister — List(ctx, options) → (runtime.Object, error)
// rest.Creater — Create(ctx, obj, createValidation, options) → (runtime.Object, error)
// rest.Updater — Update(ctx, name, objInfo, createValidation, updateValidation, forceAllowCreate, options) → (runtime.Object, bool, error)
// rest.GracefulDeleter — Delete(ctx, name, deleteValidation, options) → (runtime.Object, bool, error)
// rest.Watcher — Watch(ctx, options) → (watch.Interface, error)
// rest.Patcher — Patch(ctx, name, patchType, patchBytes, ...) → (runtime.Object, bool, error)
```

**Translation layer**:

| Gecko (`handlers/resource.go`) | `rest.Storage` adapter |
|---|---|
| `json.NewDecoder(r.Body).Decode(&objMap)` + `processor.Process()` + `json.Unmarshal` into typed object | `rest.Strategy` does validation; `GenericAPIServer` handles deserialization and content negotiation |
| `h.store.Create(ctx, clientObj)` | Adapter's `Create()` calls `store.Create()`, translating `client.Object` ↔ `runtime.Object` |
| `h.store.Get(ctx, namespace, name)` | Adapter's `Get()` calls `store.Get()` |
| `h.store.List(ctx, opts)` with label selector, shard selector, pagination | Adapter's `List()` translates `metainternalversion.ListOptions` → `storage.ListOptions` |
| `h.store.Update(ctx, clientObj)` with generation tracking, finalizer flow, optimistic concurrency | Adapter's `Update()` — generation/finalizer logic moves to `rest.Strategy.PrepareForUpdate()` |
| `h.store.Delete(ctx, namespace, name)` with finalizer soft-delete, propagation policies, dependent cleanup | Adapter's `Delete()` — finalizer logic in strategy, GC handled by existing `orlop/pkg/apiserver/gc/` |
| `h.handleWatch()` → `store.Watch()` → `<-chan ResourceEvent` → chunked JSON | Adapter's `Watch()` returns `watch.Interface` wrapping the channel; `GenericAPIServer` handles encoding |

**Object type translation**: gecko uses `client.Object` (controller-runtime) throughout; `GenericAPIServer` uses `runtime.Object` (apimachinery). Since `client.Object` embeds `runtime.Object`, the translation is straightforward — type assertion in most cases. The adapter needs to ensure GVK is set on returned objects.

**New files**:
- `orlop/pkg/apiserver/aggregated/storage.go` — `rest.Storage` adapter wrapping `storage.ResourceStore`
- `orlop/pkg/apiserver/aggregated/strategy.go` — `rest.Strategy` implementation (validation, defaulting, generation tracking, finalizer handling)
- `orlop/pkg/apiserver/aggregated/watch.go` — `watch.Interface` adapter wrapping `<-chan storage.ResourceEvent`

### 3. Implement `rest.Strategy`

**Current**: All request processing logic lives in `ResourceHandler` methods:
- Schema validation via `processor.Process()` (prune, default, validate using CRD-style structural schemas)
- Metadata stamping (UID, creationTimestamp, generation) in `Create()`
- Generation increment on spec change in `Update()`
- `CustomDefaulter` and `CustomValidator` interfaces called inline
- Finalizer-based deletion flow in `Update()` and `Delete()`
- Owner reference validation in `Create()` and `Update()`

**Target**: A `rest.Strategy` type implementing:
- `rest.RESTCreateStrategy` — `PrepareForCreate()` (metadata stamping, defaulting), `Validate()` (schema + custom validation)
- `rest.RESTUpdateStrategy` — `PrepareForUpdate()` (generation tracking, finalizer preservation), `ValidateUpdate()`
- `rest.RESTDeleteStrategy` — deletion semantics
- `runtime.ObjectTyper` — GVK resolution via scheme

The strategy holds a reference to the `schema.Processor` and delegates schema validation to it. Custom validation/defaulting via `CustomValidator`/`CustomDefaulter` interfaces is called from the strategy methods.

**Key migration details**:
- `processor.Process()` (which does prune → default → validate) is currently called on `map[string]interface{}`. With `GenericAPIServer`, the object arrives already deserialized as `runtime.Object`. The strategy would need to marshal to map, run the processor, and unmarshal back — or refactor the processor to work on typed objects directly.
- Owner reference validation (cross-namespace check, existence check) moves from handler to strategy's `Validate()`.
- The finalizer soft-delete flow (set deletionTimestamp, check on update) is handled by `GenericAPIServer`'s built-in finalizer support via `rest.GracefulDeleter` and `BeforeDelete` hooks.

### 4. Watch Protocol Adaptation

**Current** (`orlop/pkg/apiserver/handlers/watch.go`, `watch_common.go`):
- `store.Watch()` returns `(<-chan storage.ResourceEvent, func(), error)`
- `watchStreamer` writes chunked JSON to `http.ResponseWriter` via `json.Encoder`
- Supports `sendInitialEvents`, periodic bookmarks (30s), `allowWatchBookmarks`, `resourceVersionMatch`
- Events are `{"type": "ADDED/MODIFIED/DELETED", "object": {...}}` — matches kube watch wire format

**Target**: Implement `watch.Interface`:
```go
type Interface interface {
    Stop()
    ResultChan() <-chan Event
}
```

Adapter wraps `<-chan storage.ResourceEvent` into `<-chan watch.Event`, translating:
- `storage.EventAdded` → `watch.Added`
- `storage.EventModified` → `watch.Modified`
- `storage.EventDeleted` → `watch.Deleted`
- `storage.EventBookmark` → `watch.Bookmark`

The stop function from `store.Watch()` maps to `watch.Interface.Stop()`.

`GenericAPIServer` handles the HTTP streaming, content negotiation (JSON/protobuf/CBOR), timeout, and bookmark injection. The custom `watchStreamer`, `watchConfig`, and `streamWatch()` become unnecessary for the aggregated path.

**New file**: `orlop/pkg/apiserver/aggregated/watch.go`

### 5. OpenAPI / Schema Registration

**Current** (`orlop/pkg/apiserver/handlers/discovery.go`):
- Custom discovery endpoints (`/apis`, `/apis/{group}`, `/apis/{group}/{version}`)
- OpenAPI v2 built by hand from `ResourceInfo.SchemaYAML` using `kube-openapi/pkg/spec3`
- OpenAPI v3 built similarly
- Schemas include `x-kubernetes-group-version-kind` extensions

**Target**: `GenericAPIServer` provides built-in discovery and OpenAPI serving. Registration options:
- **Option A**: Implement `kube-openapi`'s `GetOpenAPIDefinitions()` function per resource type. This requires generating Go code that returns `common.OpenAPIDefinition` structs. `orlop-gen` could be extended to produce these alongside the existing `SchemaYAML` constants.
- **Option B**: Use `GenericAPIServer`'s CRD-style OpenAPI publishing, which accepts structural schemas (already available via `schema.Processor.GetValidationSchema()`).

Option B is less work since gecko already produces structural schemas from YAML. The structural schema can be registered via the `apiextensions-apiserver`'s OpenAPI builder.

**Files affected**:
- `orlop/pkg/generator/` — extend `orlop-gen` to produce OpenAPI definitions (Option A) or skip (Option B)
- Custom discovery handlers in `handlers/discovery.go` stay for the public API, become unused for the private/aggregated path

### 6. APIService Registration

Deploy an `APIService` resource per API group served by gecko:

```yaml
apiVersion: apiregistration.k8s.io/v1
kind: APIService
metadata:
  name: v1.platform.gcp-hcp.io
spec:
  group: platform.gcp-hcp.io
  version: v1
  service:
    name: gecko-platform-api
    namespace: gecko-system
  caBundle: <base64-encoded CA cert>
  groupPriorityMinimum: 100
  versionPriority: 100
```

The kube apiserver will proxy all requests to `/apis/platform.gcp-hcp.io/v1/...` to the gecko service.

### 7. TLS Configuration

Aggregated API servers must serve TLS. Options:
- **Self-signed cert**: generate a CA + serving cert, put CA in `APIService.spec.caBundle`. Simple but requires cert rotation.
- **cert-manager**: automate cert issuance and rotation. Standard approach on GKE.
- **`--tls-cert-file` / `--tls-private-key-file`**: `GenericAPIServer` natively supports these flags.

### 8. Health Endpoints

**Current**: No health endpoints.

**Target**: `GenericAPIServer` provides `/healthz`, `/livez`, `/readyz` with pluggable health checks. Add:
- Storage backend connectivity check (Postgres/Spanner connection health)
- Standard kube liveness/readiness probes in the pod spec

## What Stays Unchanged

### Public API Server
The public port (`chi.Router` + `http.Server` + `ConvertingResourceHandler`) is completely unaffected. It continues to:
- Serve the public resource view (private fields stripped via `conversion.Converter`)
- Share the same `ResourceStore` instances as the private/aggregated API
- Handle its own auth (separate service for external users)
- Serve custom discovery and OpenAPI endpoints

### Storage Layer
`storage.ResourceStore` interface and implementations (`MemoryStore`, `PostgresStore`) are untouched. The `rest.Storage` adapters sit above them.

### Code Generation
`orlop-gen` output (types, schemes, `SchemaYAML`, deepcopy) remains valid. Optional: extend to generate `GetOpenAPIDefinitions()` for kube-openapi integration.

### Conversion Layer
`conversion.Converter` and `ConvertingResourceHandler` stay for the public API.

### Server-Side Apply
`apply.Manager` already uses `k8s.io/apimachinery/pkg/util/managedfields`. Minor wiring change to plug into the `rest.Strategy` update flow instead of being called from the handler directly.

### Garbage Collection
`orlop/pkg/apiserver/gc/` (owner reference GC) stays. Could optionally integrate with kube's built-in GC, but not required.

## What Gets Removed (for the internal path)

| Component | Current location | Reason for removal |
|---|---|---|
| SA token authenticator | `orlop/pkg/apiserver/authn/` | Replaced by delegated authn via GKE |
| RBAC authorizer | `orlop/pkg/apiserver/rbac/` | Replaced by native kube RBAC |
| Auth setup | `orlop/pkg/apiserver/optional/optional.go` | No longer needed for internal path |
| Auth resource types | `rbac.orlop.gcp.managed.openshift.io/v1` (ServiceAccount, Secret, Role, etc.) | Managed by kube natively |
| Custom discovery handlers | `orlop/pkg/apiserver/handlers/discovery.go` | Built into `GenericAPIServer` |
| Custom watch streamer | `orlop/pkg/apiserver/handlers/watch_common.go` | Handled by `GenericAPIServer` |
| Chi router (private) | `orlop/pkg/apiserver/router.go` `setupRouter()` | Replaced by `GenericAPIServer` routing |

Note: chi router, discovery handlers, and watch streamer are kept for the public API. The standalone chi.Router mode for the private API is removed — `GenericAPIServer` is the only private API path.

## What We Gain Beyond AuthN/AuthZ

### Useful

| Capability | Current | With `GenericAPIServer` |
|---|---|---|
| Content negotiation | JSON only | JSON, protobuf (~60% smaller), CBOR |
| Audit logging | Request logging (method, path, status) | Structured kube audit events (who, what, when, which resource) |
| Health endpoints | None | `/healthz`, `/livez`, `/readyz` with pluggable checks |
| Graceful shutdown | Basic `Shutdown()` | Drain with in-flight request tracking |
| Metrics | None | Standard apiserver Prometheus metrics (latency, inflight, errors) |
| Token lifecycle | Manual provisioning/rotation of secrets | Projected SA tokens — auto-rotated, short-lived, audience-bound |
| API priority & fairness | None | Request throttling, queue isolation between consumers (unlikely to need within gecko, but available) |

### Comes for free (inherent to API aggregation or kube apiserver-side)

- OpenAPI aggregation into cluster's unified OpenAPI
- Admission webhooks (validating/mutating)
- Impersonation (`kubectl --as=`)
- Dry-run support

## Tradeoff: GKE API Dependency

Internal traffic flows through the GKE kube apiserver. If the GKE control plane is unavailable:
- Controllers cannot reach gecko through the proxy → reconciliation of user requests is delayed
- Gecko itself stays up → external users can still create and manage resources via the public API
- Requests are persisted in the datastore → controllers catch up once the kube apiserver recovers
- Similar to a controller pod restart — a delay, not an outage

GKE regional SLA is 99.95%.

## Implementation Order

Suggested phasing:

1. **Adapter layer** (can be developed in parallel with existing code):
   - `rest.Storage` adapters wrapping `ResourceStore`
   - `rest.Strategy` implementations (extract logic from `ResourceHandler`)
   - `watch.Interface` adapter
   - Unit tests: verify adapters produce correct behavior against `MemoryStore`

2. **`GenericAPIServer` integration**:
   - Create aggregated server configuration
   - Wire `rest.Storage` adapters via `InstallAPIGroup()`
   - TLS setup
   - Integration test: start aggregated server, CRUD via `client-go`

3. **Server refactoring**:
   - Refactor `Server` struct to use `GenericAPIServer` for the private API (standalone chi.Router mode removed)
   - `--disable-auth` flag for local development without kube-apiserver dependency

4. **Cleanup** (not yet deployed — no production validation gate):
   - Delete custom auth packages (`authn/`, `rbac/`, `optional/`) and auth resource types — zero remaining callers
   - Delete custom discovery and watch streamer if unused by public API path
   - Chi router and public API conversion layer are **kept** (still serve public API)
   - Update documentation and operational runbooks

5. **Deployment**:
   - `APIService` resource and RBAC configuration
   - TLS certificate management (cert-manager)
   - Health checks and monitoring
   - Kustomize base manifests + minikube overlay for local development
