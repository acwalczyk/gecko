# Platform API Architecture

## Dual API Surface with Shared Storage

The server exposes two API surfaces sharing the same storage backend. The private API uses GenericAPIServer (HTTPS). The public API uses chi.Router (HTTP).

```
                          API Clients
                              │
               ┌──────────────┴──────────────┐
               │                             │
       ┌───────▼────────┐            ┌───────▼────────┐
       │  Private API   │            │   Public API   │
       │   Port 8080    │            │   Port 8081    │
       │  GenericAPI    │            │  chi.Router    │
       │  Server        │            │  (always HTTP) │
       │  (HTTPS)       │            └───────┬────────┘
       └───────┬────────┘                    │
               │                     ┌───────▼────────┐
               │                     │   Converter    │
               │                     │ PrivateToPublic│
               │                     │ PublicToPrivate│
               │                     └───────┬────────┘
               │                             │
               └──────────┬──────────────────┘
                          │
                  ┌───────▼───────┐
                  │ Shared Store  │
                  │  ResourceStore│
                  │               │
                  │  Memory /     │
                  │  PostgreSQL / │
                  │  Spanner      │
                  └───────────────┘
```

### Private API

Private API uses Kubernetes `GenericAPIServer` with `rest.Storage` adapters wrapping `ResourceStore`. HTTPS with TLS.

```
Client → GenericAPIServer → rest.Storage adapter → ResourceStore
              │
              ├── Content negotiation (JSON, protobuf, CBOR)
              ├── OpenAPI v3 serving
              ├── API discovery (/apis)
              ├── Health endpoints (/healthz, /livez, /readyz)
              ├── Watch protocol (streaming)
              ├── Delegated authn (TokenReview)
              └── Delegated authz (SubjectAccessReview)
```

When deployed on a cluster with an `APIService` registration:

```
Client (kubectl, controller)
    → GKE kube-apiserver (authn, authz, audit)
        → proxy (APIService routing)
            → Gecko GenericAPIServer
                → rest.Storage adapter
                    → ResourceStore (PostgreSQL / Spanner)
```

## Private/Public Field Filtering

Source types in `api/private/` use `+orlop:public` markers. The `orlop-gen` code generator produces filtered types in `api/public/` containing only marked fields.

```go
// Private (all fields)
type ObjectSpec struct {
    PublicField   string `json:"publicField"`   // +orlop:public
    InternalField string `json:"internalField"` // not exposed
}

// Public (generated, filtered)
type ObjectSpec struct {
    PublicField string `json:"publicField"`
}
```

### Create via public API

```
Client → POST public object
    → Converter.PublicToPrivate(public, nil)
        → Private object with public fields set, internal fields zero-valued
    → Store (private type)
```

### Read via public API

```
Store (private type) → full object with all fields
    → Converter.PrivateToPublic()
        → JSON round-trip drops unexported fields
        → filterPrivateMetadata() removes private.orlop.gcp.managed.openshift.io/ labels/annotations
        → filterPrivateConditions() removes private.orlop.gcp.managed.openshift.io/ conditions
    → Client sees filtered object
```

### Update via public API

```
Client → PUT public object
    → Converter.PublicToPrivate(public, existing)
        → Start with existing private object (preserves internal fields)
        → Overlay public fields from request
    → Store (private type) — internal fields preserved
```

## Key Components

### Storage layer (`orlop/pkg/apiserver/storage/`)

`ResourceStore` interface with pluggable backends:
- **MemoryStore** — in-memory, for development and testing
- **PostgresStore** — PostgreSQL with JSON column storage
- **Spanner** — planned

Both API surfaces share the same store instances via a memoizing `StorageFactory`.

### Adapter layer (`orlop/pkg/apiserver/aggregated/`)

Bridges `ResourceStore` to GenericAPIServer's `rest.Storage` interfaces:
- **ResourceStorage** — `rest.Storage` adapter wrapping `ResourceStore` (CRUD, watch, table conversion)
- **StatusStorage** — `/status` subresource (status-only updates)
- **ResourceStrategy** — `rest.RESTCreateStrategy` + `rest.RESTUpdateStrategy` (schema validation, generation tracking, finalizer handling, custom defaulting/validation)
- **watchAdapter** — `watch.Interface` wrapping `<-chan storage.ResourceEvent`

### Converter (`orlop/pkg/apiserver/conversion/`)

Bidirectional conversion between private and public types. Uses JSON round-trip for field filtering plus explicit metadata/condition filtering for `private.orlop.gcp.managed.openshift.io/` prefixed keys.

### Code generator (`orlop/pkg/generator/`)

`orlop-gen` produces from private API types:
- Filtered public types (only `+orlop:public` fields)
- DeepCopy methods
- OpenAPI structural schemas (YAML)
- Conversion functions (private ↔ public)
- GroupVersion registration

## Security Model

### Local development

With `--disable-auth`: authn/authz skipped, all requests accepted. HTTPS still required.

### On cluster

Authentication and authorization delegated to the GKE kube-apiserver:
- **Authn**: tokens validated via `TokenReview` API (projected ServiceAccount tokens, short-lived, auto-rotated)
- **Authz**: permissions checked via `SubjectAccessReview` API (native Kubernetes RBAC)
- **Audit**: structured audit events logged by the kube-apiserver

The custom auth stack (SA token authenticator, RBAC authorizer, auth resource types) was removed in Phase 4 — replaced entirely by kube-native mechanisms.

### Public API

Separate auth (not covered by API aggregation). Intended for external consumers with its own authentication layer.
