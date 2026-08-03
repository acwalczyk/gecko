package spanner

import (
	"context"
	"fmt"
	"strings"

	"cloud.google.com/go/spanner"
	database "cloud.google.com/go/spanner/admin/database/apiv1"
	"cloud.google.com/go/spanner/admin/database/apiv1/databasepb"
	"github.com/go-logr/logr"
	"github.com/openshift-online/gecko/orlop/pkg/apiserver/storage"
	"google.golang.org/api/option"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type StorageFactoryConfig struct {
	Database    string
	Client      *spanner.Client
	AdminClient *database.DatabaseAdminClient
	TablePrefix string
	Context     context.Context
	ClientOpts  []option.ClientOption
	Logger      logr.Logger
}

func NewStorageFactory(config StorageFactoryConfig) (func(string, *runtime.Scheme, schema.GroupVersionKind) (storage.ResourceStore, error), error) {
	ctx := config.Context
	if ctx == nil {
		ctx = context.Background()
	}

	logger := config.Logger
	if logger.GetSink() == nil {
		logger = logr.Discard()
	}

	client := config.Client
	if client == nil {
		var err error
		client, err = spanner.NewClient(ctx, config.Database, config.ClientOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create spanner client: %w", err)
		}
	}

	adminClient := config.AdminClient
	if adminClient == nil {
		var err error
		adminClient, err = database.NewDatabaseAdminClient(ctx, config.ClientOpts...)
		if err != nil {
			return nil, fmt.Errorf("failed to create database admin client: %w", err)
		}
	}

	countersTable := config.TablePrefix + "counters"
	if err := ensureTable(ctx, adminClient, config.Database, countersTable, countersSchema(countersTable)); err != nil {
		return nil, fmt.Errorf("failed to create counters table: %w", err)
	}

	factory := func(resourceType string, scheme *runtime.Scheme, gvk schema.GroupVersionKind) (storage.ResourceStore, error) {
		resourceLogger := logger.WithValues("resource", resourceType)
		safeType := sanitizeTableName(resourceType)
		resourcesTable := config.TablePrefix + "resources_" + safeType
		eventLogTable := config.TablePrefix + "event_log_" + safeType

		if err := ensureTable(ctx, adminClient, config.Database, resourcesTable, resourcesSchema(resourcesTable)); err != nil {
			return nil, fmt.Errorf("failed to create resources table: %w", err)
		}

		ensureSearchIndex(ctx, adminClient, config.Database, resourcesTable, resourceLogger)

		if err := ensureTable(ctx, adminClient, config.Database, eventLogTable, eventLogSchema(eventLogTable)); err != nil {
			return nil, fmt.Errorf("failed to create event log table: %w", err)
		}

		changeStreamName := "cs_" + eventLogTable
		if err := ensureChangeStream(ctx, adminClient, config.Database, changeStreamName, changeStreamSchema(changeStreamName, eventLogTable)); err != nil {
			return nil, fmt.Errorf("failed to create change stream: %w", err)
		}

		broadcaster, err := newSpannerBroadcaster(ctx, spannerBroadcasterConfig{
			Client:           client,
			TableName:        eventLogTable,
			ChangeStreamName: changeStreamName,
			Scheme:           scheme,
			GVK:              gvk,
			Logger:           resourceLogger,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create broadcaster: %w", err)
		}

		store, err := NewSpannerStore(SpannerStoreConfig{
			Client:        client,
			ResourceType:  resourceType,
			Scheme:        scheme,
			GVK:           gvk,
			Broadcaster:   broadcaster,
			TableName:     resourcesTable,
			EventLogTable: eventLogTable,
			CountersTable: countersTable,
			Logger:        resourceLogger,
		})
		if err != nil {
			broadcaster.Close()
			return nil, fmt.Errorf("failed to create store: %w", err)
		}

		return store, nil
	}

	return factory, nil
}

func sanitizeTableName(name string) string {
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

func countersSchema(tableName string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE %s (
			counter_id STRING(253) NOT NULL,
			value INT64 NOT NULL,
		) PRIMARY KEY (counter_id)`, tableName),
	}
}

func resourcesSchema(tableName string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE %s (
			context_filter STRING(253) NOT NULL,
			namespace STRING(253) NOT NULL,
			name STRING(253) NOT NULL,
			resource_version INT64 NOT NULL,
			labels JSON,
			data JSON NOT NULL,
			created_at TIMESTAMP NOT NULL,
			updated_at TIMESTAMP NOT NULL,
		) PRIMARY KEY (context_filter, namespace, name)`, tableName),
		fmt.Sprintf(`CREATE INDEX idx_%s_namespace ON %s(namespace)`, tableName, tableName),
		fmt.Sprintf(`CREATE INDEX idx_%s_rv ON %s(resource_version)`, tableName, tableName),
	}
}

func eventLogSchema(tableName string) []string {
	return []string{
		fmt.Sprintf(`CREATE TABLE %s (
			rv INT64 NOT NULL,
			event_type STRING(20) NOT NULL,
			object_data JSON NOT NULL,
			context_filter STRING(253) NOT NULL,
			created_at TIMESTAMP NOT NULL,
		) PRIMARY KEY (rv)`, tableName),
		fmt.Sprintf(`CREATE INDEX idx_%s_created_at ON %s(created_at)`, tableName, tableName),
	}
}

func ensureTable(ctx context.Context, adminClient *database.DatabaseAdminClient, dbPath string, tableName string, ddlStatements []string) error {
	// Check if the table already exists by getting the DDL
	resp, err := adminClient.GetDatabaseDdl(ctx, &databasepb.GetDatabaseDdlRequest{
		Database: dbPath,
	})
	if err != nil {
		return fmt.Errorf("failed to get database DDL: %w", err)
	}

	for _, stmt := range resp.Statements {
		if strings.Contains(strings.ToLower(stmt), strings.ToLower("CREATE TABLE "+tableName)) {
			return nil
		}
	}

	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbPath,
		Statements: ddlStatements,
	})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.AlreadyExists {
			return nil
		}
		return fmt.Errorf("failed to update DDL: %w", err)
	}

	if err := op.Wait(ctx); err != nil {
		return fmt.Errorf("DDL update failed: %w", err)
	}

	return nil
}

func changeStreamSchema(streamName, tableName string) []string {
	return []string{
		fmt.Sprintf(`CREATE CHANGE STREAM %s FOR %s OPTIONS (value_capture_type = 'NEW_ROW', retention_period = '24h')`, streamName, tableName),
	}
}

func ensureChangeStream(ctx context.Context, adminClient *database.DatabaseAdminClient, dbPath string, streamName string, ddlStatements []string) error {
	resp, err := adminClient.GetDatabaseDdl(ctx, &databasepb.GetDatabaseDdlRequest{
		Database: dbPath,
	})
	if err != nil {
		return fmt.Errorf("failed to get database DDL: %w", err)
	}

	for _, stmt := range resp.Statements {
		if strings.Contains(strings.ToLower(stmt), strings.ToLower("CREATE CHANGE STREAM "+streamName)) {
			return nil
		}
	}

	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbPath,
		Statements: ddlStatements,
	})
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.AlreadyExists {
			return nil
		}
		return fmt.Errorf("failed to update DDL: %w", err)
	}

	if err := op.Wait(ctx); err != nil {
		return fmt.Errorf("DDL update failed: %w", err)
	}

	return nil
}

func searchIndexSchema(tableName string) []string {
	return []string{
		fmt.Sprintf(`ALTER TABLE %s ADD COLUMN labels_tokens TOKENLIST AS (TOKENIZE_JSON_OBJECT(labels)) HIDDEN`, tableName),
		fmt.Sprintf(`CREATE SEARCH INDEX idx_%s_labels ON %s(labels_tokens)`, tableName, tableName),
	}
}

func ensureSearchIndex(ctx context.Context, adminClient *database.DatabaseAdminClient, dbPath string, tableName string, logger logr.Logger) {
	resp, err := adminClient.GetDatabaseDdl(ctx, &databasepb.GetDatabaseDdlRequest{
		Database: dbPath,
	})
	if err != nil {
		return
	}

	indexName := fmt.Sprintf("idx_%s_labels", tableName)
	for _, stmt := range resp.Statements {
		if strings.Contains(strings.ToLower(stmt), strings.ToLower("CREATE SEARCH INDEX "+indexName)) {
			return
		}
	}

	op, err := adminClient.UpdateDatabaseDdl(ctx, &databasepb.UpdateDatabaseDdlRequest{
		Database:   dbPath,
		Statements: searchIndexSchema(tableName),
	})
	if err != nil {
		logger.V(1).Info("Search index not supported, label queries will work without index", "error", err)
		return
	}

	if err := op.Wait(ctx); err != nil {
		logger.V(1).Info("Search index creation failed, label queries will work without index", "error", err)
	}
}
