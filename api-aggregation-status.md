# API Aggregation Implementation Status

Tracks progress against the phases defined in [api-aggregation.md](api-aggregation.md).

## Phase 1: Adapter Layer — DONE

All adapter types implemented in `orlop/pkg/apiserver/aggregated/`:

| File | Purpose | Status |
|---|---|---|
| `config.go` | `Config` / `CompletedConfig` for aggregated server setup | Done |
| `apiserver.go` | `AggregatedServer` wrapping `GenericAPIServer`, API group installation, schema processor creation | Done |
| `storage.go` | `ResourceStorage` (`rest.Storage` adapter wrapping `storage.ResourceStore`), `StatusStorage` (`/status` subresource) | Done |
| `strategy.go` | `ResourceStrategy` (`rest.RESTCreateStrategy` + `rest.RESTUpdateStrategy`) — schema processing, generation tracking, metadata stamping, finalizer preservation, `CustomDefaulter`/`CustomValidator` support | Done |
| `watch.go` | `watchAdapter` (`watch.Interface` wrapping `<-chan storage.ResourceEvent`) with `sync.Once` stop and done-channel for goroutine safety | Done |

Tests: 41 tests passing (`storage_test.go`, `strategy_test.go`, `watch_test.go`). Covers CRUD, finalizer soft/hard delete, generation tracking, watch event translation, status-only updates, error discrimination.

`server.go` uses `GenericAPIServer` for the private API. Standalone mode was removed in Phase 6.

### Key design decisions made

- `New()`/`NewList()` panic on missing GVK (programming error, caught at startup)
- `Get()` distinguishes NotFound from internal errors (no masking DB failures as 404)
- `Update()` performs hard-delete when last finalizer is removed from a deleting object
- `PrepareForUpdate()` preserves deletionTimestamp, UID, creationTimestamp from old object
- Scheme registration in `New()` calls `metav1.AddToGroupVersion` and `metainternalversion.AddToScheme` directly (idempotent; guard removed in Phase 2)
- Schema processing (prune/default/validate) done via marshal→map→Process→unmarshal round-trip (same as existing handlers)
- Logger threaded through `ResourceStrategy` for error observability in void-return interface methods

## Phase 2: GenericAPIServer Integration — DONE

All GenericAPIServer integration features implemented in `orlop/pkg/apiserver/aggregated/`:

| Feature | File(s) | Status |
|---|---|---|
| Delegated authentication | `apiserver.go`, `config.go` | Done — `DelegatingAuthenticationOptions` with kubeconfig or in-cluster `TokenReview`, `DisableAuth` flag for testing |
| Delegated authorization | `apiserver.go`, `config.go` | Done — `DelegatingAuthorizationOptions` with kubeconfig or in-cluster `SubjectAccessReview`, health/readyz/livez always-allow paths |
| OpenAPI schema registration | `openapi.go`, `apiserver.go` | Done — Option B: structural schemas from `SchemaYAML` merged with standard k8s type definitions from `k8s.io/apiextensions-apiserver/pkg/generated/openapi` |
| Health checks | `healthcheck.go`, `config.go`, `apiserver.go` | Done — `StorageHealthChecker` probes storage backend via `List(limit=1)`, custom health checks supported via `Config.HealthCheckers` |
| EffectiveVersion setup | `apiserver.go` | Done — registers `DefaultKubeComponent` with `DefaultBuildEffectiveVersion()` for GenericAPIServer compatibility |
| Loopback client | `apiserver.go` | Done — `SecureServingOptions.WithLoopback().ApplyToConfig()` sets up loopback TLS + client config |

Tests: 56 tests passing (41 unit + 15 integration). Integration tests cover:
- Full CRUD lifecycle via HTTP (Create 201, Get, List, Update with generation tracking, Delete)
- Status subresource updates (only status field persisted, spec preserved)
- Finalizer soft-delete → hard-delete flow
- Watch event streaming (ADDED events via chunked JSON)
- Health endpoints (`/healthz`, `/livez`, `/readyz` + custom named checks)
- OpenAPI v3 serving with API group discovery
- API discovery (`/apis` lists registered groups)
- Anonymous access with `DisableAuth=true`
- `StorageHealthChecker` unit tests (success + factory error)

### Key design decisions made

- `DisableAuth` flag allows testing without a kube apiserver for token/SAR delegation
- Auth kubeconfig paths are optional — falls back to in-cluster config with tolerated lookup failure
- OpenAPI uses `generatedopenapi.GetOpenAPIDefinitions` from `orlop/pkg/generated/openapi` as base, custom type schemas merged on top via `buildOpenAPIDefinitions()`
- `StorageHealthChecker` creates a store per health check call via the factory (stateless probe)
- `EffectiveVersion` registered once via `sync`-safe `DefaultComponentGlobalsRegistry`
- Storage errors consistently wrapped with `errors.NewInternalError` for proper `metav1.Status` API responses
- Resource grouping in `New()` uses plain `map[string]map[string][]types.ResourceInfo` (no wrapper types)
- `specChanged()` uses `json.RawMessage` + `bytes.Equal` to avoid redundant re-marshaling of spec fields

## Phase 3: Server Refactoring — DONE

All server refactoring completed in `orlop/pkg/apiserver/` and `platform-api/cmd/platform-api-server/`:

| Feature | File(s) | Status |
|---|---|---|
| Dual-mode Server struct | `server.go` | Done — `PrivateAPIOptions.Aggregated` selects mode |
| Run/Shutdown/PrivateAddress dual paths | `server.go` | Done — aggregated uses `GenericAPIServer.PrepareRun().Run()`, standalone uses `chi.Router` |
| Shared store fix (aggregated + public API) | `server.go` | Done — memoizing `StorageFactory` with `sync.Mutex` ensures aggregated server and public API conversion layer share same store instances |
| stopCh race fix | `server.go` | Done — `stopCh` initialized in `New()` instead of `Run()` |
| CLI flags | `platform-api/cmd/platform-api-server/main.go` | Done — `--aggregated`, `--tls-cert-file`, `--tls-key-file`, `--authentication-kubeconfig`, `--authorization-kubeconfig`, `--disable-auth` |
| Integration tests | `server_test.go` | Done — 16 tests covering both modes, CRUD, public API conversion, shared store validation, shutdown, error paths |

Tests: 458 tests passing across all 28 packages (with race detector).

### Key design decisions made

- Memoizing StorageFactory wraps the user-provided factory to cache store instances by `resourceType/group/version/kind` composite key; thread-safe via `sync.Mutex`
- Health checker intentionally uses the unmemoized factory — each `Check()` call creates a fresh store to probe backend connectivity
- `stopCh` created unconditionally in `New()` to avoid data race between `Run()` and `Shutdown()`
- Aggregated `Run()` propagates server errors via `errCh` channel instead of swallowing them in a goroutine
- Standalone mode removed in Phase 6

## Phase 4: Cleanup — DONE

Deleted unused code fully superseded by `GenericAPIServer` in aggregated mode. Each component verified to have zero remaining callers before deletion. Full test suite passing after removal.

The chi router and public API conversion layer are **kept** — they still serve the public API.

| Component | Location | Action | Status |
|---|---|---|---|
| SA token authenticator | `orlop/pkg/apiserver/authn/` | Deleted package (4 files) — replaced by delegated authn via GKE | Done |
| RBAC authorizer | `orlop/pkg/apiserver/rbac/` | Deleted package (4 files) — replaced by native kube RBAC | Done |
| Auth setup | `orlop/pkg/apiserver/optional/optional.go` | Deleted — sole caller was `orlop-server/main.go` | Done |
| Auth resource types | `orlop/apis/private/rbac/` | Deleted directory (11 files) — `rbac.orlop.gcp.managed.openshift.io/v1` types managed by kube natively | Done |
| `orlop-server` CLI flags | `orlop/cmd/orlop-server/main.go` | Removed `--enable-rbac`, `--enable-authentication` flags and `optional` import | Done |
| `orlop-server` scheme | `orlop/cmd/orlop-server/resources.go` | Removed `rbacv1.AddToScheme()` call and import | Done |
| Custom discovery handlers | `orlop/pkg/apiserver/handlers/discovery.go` | **Kept** — used by public API (`setupConvertingRouter`) | N/A |
| Custom watch streamer | `orlop/pkg/apiserver/handlers/watch_common.go` | **Kept** — used by public API (`ConvertingResourceHandler`) | N/A |
| Chi router | `orlop/pkg/apiserver/router.go` | **Kept** — `setupConvertingRouter` still used by public API; `setupRouter` removed in Phase 6 | N/A |

Tests: 373 tests passing across 23 packages (with race detector). 5 packages removed (authn, rbac, optional, rbac/v1 types, rbac parent).

## Phase 5: Deployment — DONE

Deployment manifests implemented in `deploy/`:

| Component | File(s) | Status |
|---|---|---|
| Containerfile | `deploy/platform-api/Containerfile` | Done — multi-stage build, scratch runtime, podman/docker compatible |
| Namespace + ServiceAccount | `deploy/platform-api/base/namespace.yaml`, `serviceaccount.yaml` | Done — `gecko-system` namespace |
| Deployment | `deploy/platform-api/base/deployment.yaml` | Done — TLS volume mount, health probes on `/livez`, `/readyz` |
| Service | `deploy/platform-api/base/service.yaml` | Done — HTTPS on port 443 |
| APIService | `deploy/platform-api/base/apiservice.yaml` | Done — `v1.gcp.managed.openshift.io` with cert-manager CA injection |
| Certificate | `deploy/platform-api/base/certificate.yaml` | Done — cert-manager Certificate for serving TLS |
| Auth-delegator RBAC | `deploy/platform-api/base/rbac.yaml` | Done — `system:auth-delegator` + `extension-apiserver-authentication-reader` bindings |
| Kustomize base | `deploy/platform-api/base/kustomization.yaml` | Done |
| Minikube overlay | `deploy/minikube/` | Done — kustomization, ClusterIssuer, setup.sh, teardown.sh |

Verified on minikube:
- cert-manager TLS with self-signed ClusterIssuer
- Anonymous requests receive 403 Forbidden (auth enforced)
- Controller RBAC: 403 without ClusterRole, works after granting permissions

## Phase 6: Remove Standalone Private API Mode — DONE

Removed the chi.Router standalone mode for the private API. `GenericAPIServer` is the only private API path. `--disable-auth` covers local development.

| Change | File(s) | Status |
|---|---|---|
| Remove `Aggregated` flag from `PrivateAPIOptions` | `server.go` | Done |
| Remove `Registry` and `Middleware` fields (standalone-only) | `server.go` | Done |
| Remove `privateRouter` and `privateServer` from `Server` struct | `server.go` | Done |
| Flatten `New()` to aggregated-only path | `server.go` | Done |
| Simplify `Run()`, `Shutdown()`, `PrivateAddress()` | `server.go` | Done |
| Remove `setupRouter` (dead code, kept `setupConvertingRouter`) | `router.go` | Done |
| Remove `--aggregated` CLI flag | `platform-api-server/main.go` | Done |
| Convert `orlop-server` to aggregated mode | `orlop/cmd/orlop-server/main.go` | Done |
| Remove standalone-mode tests | `server_test.go` | Done |
| Update docs | `ARCHITECTURE.md`, `README.md`, `api-aggregation-status.md` | Done |

Tests: 327 tests passing across all apiserver packages (with race detector).

Note: `orlop/test/` integration tests need follow-up adaptation for GenericAPIServer behavioral differences (validation messages, CORS, shard selectors, version conversion). These were standalone-mode-specific tests.
