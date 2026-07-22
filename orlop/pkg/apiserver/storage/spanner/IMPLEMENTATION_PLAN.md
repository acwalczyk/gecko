# Plan: Implement Spanner-backed ResourceStore

## Context

The orlop API server currently has two `ResourceStore` implementations: in-memory (`storage/memory/`) and PostgreSQL (`storage/postgres/`). This plan describes a third implementation backed by Google Cloud Spanner, with local tests using the Spanner emulator. It mirrors the PostgreSQL implementation's structure and patterns, adapted for Spanner's SQL dialect and client library.

## File Structure

Create `pkg/apiserver/storage/spanner/` with:

```
store.go            -- SpannerStore: CRUD + Watch (mirrors postgres/store.go)
broadcaster.go      -- SpannerBroadcaster: in-memory fan-out + event log for replay
querybuilder.go     -- Spanner SQL query builder (@param syntax, JSON_VALUE for labels)
factory.go          -- NewStorageFactory wiring
store_test.go       -- Integration tests against emulator
broadcaster_test.go -- Broadcaster tests against emulator
```

## Spanner Schema

### Resources Table

```sql
CREATE TABLE resources_{type} (
    context_filter STRING(253) NOT NULL DEFAULT (''),
    namespace      STRING(253) NOT NULL,
    name           STRING(253) NOT NULL,
    resource_version INT64 NOT NULL,
    labels         JSON,
    data           JSON NOT NULL,
    created_at     TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp = true),
    updated_at     TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp = true),
) PRIMARY KEY (context_filter, namespace, name)
```

Indexes on `namespace`, `resource_version`, `context_filter`. No GIN index (Spanner doesn't support it); label filtering uses `JSON_VALUE()` in queries.

### Event Log Table

```sql
CREATE TABLE event_log_{type} (
    id               STRING(36) NOT NULL,
    event_type       STRING(20) NOT NULL,
    resource_version INT64 NOT NULL,
    object_data      JSON NOT NULL,
    created_at       TIMESTAMP NOT NULL OPTIONS (allow_commit_timestamp = true),
) PRIMARY KEY (id)
```

Indexes on `resource_version` and `created_at`.

## Implementation Details

### `store.go` — SpannerStore

**Struct**: Same fields as `PostgresStore` but with `*spanner.Client` instead of `*sql.DB`. Also needs `adminClient *database.DatabaseAdminClient` and `databaseName string` for DDL operations.

**Config**:
```go
type SpannerStoreConfig struct {
    Client           *spanner.Client
    AdminClient      *database.DatabaseAdminClient
    DatabaseName     string
    ResourceType     string
    Scheme           *runtime.Scheme
    GVK              schema.GroupVersionKind
    Broadcaster      storage.EventBroadcaster
    TableName        string
    ContextFilterKey any
}
```

**Constructor** (`NewSpannerStore`): Same flow as PostgreSQL — validate config, create schema, init RV counter. Schema creation differs: query `INFORMATION_SCHEMA.TABLES` to check existence (no `IF NOT EXISTS` in Spanner DDL), then issue `adminClient.UpdateDatabaseDdl()` which is an async LRO that must be awaited.

**CRUD operations** — Follow PostgreSQL patterns exactly, with these Spanner adaptations:
- **Create**: Use `client.Apply(ctx, []*spanner.Mutation{spanner.Insert(...)})`. Detect duplicates via `spanner.ErrCode(err) == codes.AlreadyExists`. Use `spanner.CommitTimestamp` for timestamp fields.
- **Get**: Use `client.Single().ReadRow(ctx, table, spanner.Key{filter, ns, name}, columns)`. Map `codes.NotFound` to `errors.NewNotFound`.
- **List**: Use `client.Single().Query(ctx, spanner.Statement{SQL, Params})` with the Spanner query builder. Shard filtering done in Go post-fetch (not in SQL) for the initial implementation.
- **Update**: Use `client.Apply(ctx, []*spanner.Mutation{spanner.Update(...)})`. Check existence first via Get.
- **Delete**: Use `client.Apply(ctx, []*spanner.Mutation{spanner.Delete(table, key)})`. Get object first for the event payload.
- **Watch**: Identical to PostgreSQL — subscribes to broadcaster, filtering goroutine with context filter, namespace, labels, shard.

**JSON handling**: Use `spanner.NullJSON` for reading/writing JSON columns. Marshal `client.Object` to `json.RawMessage`, wrap in `spanner.NullJSON{Value: rawMsg, Valid: true}`.

**Resource version**: Same in-memory counter pattern as PostgreSQL, initialized from `MAX(resource_version)`.

### `broadcaster.go` — SpannerBroadcaster

Since Spanner has no LISTEN/NOTIFY, use **in-memory fan-out** (like the memory store's `Watcher`) plus **event log table** for historical replay on Subscribe.

**Struct**: `*spanner.Client`, subscriber map, event log table name, scheme/GVK for object reconstruction.

**Broadcast**: Insert event into the event log table via `client.Apply()`, then fan-out to local in-memory subscribers (non-blocking send).

**Subscribe**: Create buffered channel (100), register subscriber, replay historical events from event log (`WHERE resource_version > @rv ORDER BY resource_version ASC LIMIT 1000`).

**PruneOldEvents**: Delete events via DML in a `ReadWriteTransaction` where `created_at < @cutoff`.

**Close**: Close all subscriber channels, set closed flag.

### `querybuilder.go` — Spanner Query Builder

Mirrors the PostgreSQL `QueryBuilder` with these key differences:

| PostgreSQL | Spanner |
|---|---|
| `$1, $2` positional params | `@p1, @p2` named params |
| `[]interface{}` args slice | `map[string]interface{}` params map |
| `labels ? $1` (key exists) | `JSON_VALUE(labels, '$.key') IS NOT NULL` |
| `labels->>$1 = $2` | `JSON_VALUE(labels, '$.key') = @p1` |
| `= ANY($1)` with `pq.Array()` | `IN UNNEST(@p1)` with `[]string` |
| `sha256()+get_byte()` shard SQL | No-op (shard filtering in Go) |
| `LIMIT $N` | `LIMIT @pN` |

**Label key injection prevention**: Kubernetes label keys are restricted to `[a-zA-Z0-9._-/]`. Validate keys before embedding in `JSON_VALUE(labels, '$.key')` paths.

**Shard filtering**: `WhereShardSelector` is a no-op in the initial implementation. The store's `List` method applies `storage.MatchesShard` in Go on each returned row. This avoids the complexity of replicating SHA-256 byte extraction in Spanner SQL.

### `factory.go` — StorageFactory

```go
type StorageFactoryConfig struct {
    Client       *spanner.Client
    AdminClient  *database.DatabaseAdminClient
    DatabaseName string
    Context      context.Context
}
```

Creates paired broadcaster + store per resource type, same pattern as PostgreSQL factory.

## Test Setup

### Emulator Connection

Tests connect to the Spanner emulator at `SPANNER_EMULATOR_HOST` (defaults to `localhost:9010`). Skip with `t.Skipf` if emulator is unreachable, matching the PostgreSQL test pattern.

**Test setup helper** (`setupTestSpanner`):
1. Check emulator reachability via gRPC dial with 2s timeout
2. Create instance via `instance.NewInstanceAdminClient` (idempotent)
3. Create unique test database per test run via `database.NewDatabaseAdminClient`
4. Create `*spanner.Client` for the database
5. Return client, admin client, and cleanup function (drops database)

### Test Coverage

Mirror PostgreSQL tests:
- **Store**: Create (with RV, duplicates, namespaces, generateName), Get, List (namespace/label filtering, pagination), Update, Delete, RV monotonic increment, persistence, concurrent creates, schema verification via `INFORMATION_SCHEMA`
- **Broadcaster**: Event delivery, historical replay, stop/close, prune

Use same `objectOption`/`newTestObject` helper pattern as PostgreSQL tests.

## Dependencies

Add to `go.mod`:
- `cloud.google.com/go/spanner` — Spanner client
- `cloud.google.com/go/spanner/admin/database` — DDL operations
- `cloud.google.com/go/spanner/admin/instance` — test instance creation
- `github.com/google/uuid` — event log IDs (may already be indirect)
- `google.golang.org/grpc` and `google.golang.org/grpc/codes` — already indirect, will be promoted

## Verification

1. Start emulator: `docker run -p 9010:9010 -p 9020:9020 gcr.io/cloud-spanner-emulator/emulator`
2. `SPANNER_EMULATOR_HOST=localhost:9010 go test ./pkg/apiserver/storage/spanner/...`
3. `go build ./...` — ensure no compilation errors
4. `go vet ./pkg/apiserver/storage/spanner/...`
