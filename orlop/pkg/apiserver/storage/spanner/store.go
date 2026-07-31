package spanner

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/openshift-online/gecko/orlop/pkg/apiserver/storage"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SpannerStore struct {
	client           *spanner.Client
	resourceType     string
	scheme           *runtime.Scheme
	gvk              schema.GroupVersionKind
	broadcaster      storage.EventBroadcaster
	tableName        string
	countersTable    string
	counterID        string
	contextFilterKey any
}

type SpannerStoreConfig struct {
	Client           *spanner.Client
	ResourceType     string
	Scheme           *runtime.Scheme
	GVK              schema.GroupVersionKind
	Broadcaster      storage.EventBroadcaster
	TableName        string
	CountersTable    string
	ContextFilterKey any
}

func NewSpannerStore(config SpannerStoreConfig) (*SpannerStore, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("spanner client is required")
	}
	if config.ResourceType == "" {
		return nil, fmt.Errorf("resource type is required")
	}
	if config.Scheme == nil {
		return nil, fmt.Errorf("scheme is required")
	}

	tableName := config.TableName
	if tableName == "" {
		tableName = "resources_" + config.ResourceType
	}
	countersTable := config.CountersTable
	if countersTable == "" {
		countersTable = "counters"
	}

	return &SpannerStore{
		client:           config.Client,
		resourceType:     config.ResourceType,
		scheme:           config.Scheme,
		gvk:              config.GVK,
		broadcaster:      config.Broadcaster,
		tableName:        tableName,
		countersTable:    countersTable,
		counterID:        "rv_" + config.ResourceType,
		contextFilterKey: config.ContextFilterKey,
	}, nil
}

func (s *SpannerStore) nextResourceVersion(ctx context.Context) (int64, error) {
	var rv int64
	_, err := s.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		row, err := txn.ReadRow(ctx, s.countersTable, spanner.Key{s.counterID}, []string{"value"})
		if err != nil {
			if spanner.ErrCode(err) == codes.NotFound {
				rv = 1
				return txn.BufferWrite([]*spanner.Mutation{
					spanner.Insert(s.countersTable, []string{"counter_id", "value"}, []interface{}{s.counterID, int64(1)}),
				})
			}
			return err
		}
		var current int64
		if err := row.Column(0, &current); err != nil {
			return err
		}
		rv = current + 1
		return txn.BufferWrite([]*spanner.Mutation{
			spanner.Update(s.countersTable, []string{"counter_id", "value"}, []interface{}{s.counterID, rv}),
		})
	})
	return rv, err
}

func (s *SpannerStore) contextFilterValue(ctx context.Context) (string, error) {
	if s.contextFilterKey == nil {
		return "", nil
	}
	v := ctx.Value(s.contextFilterKey)
	if v == nil {
		return "", fmt.Errorf("context filter key %v not found in context", s.contextFilterKey)
	}
	str, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("context filter value must be a string, got %T", v)
	}
	return str, nil
}

func marshalData(obj client.Object) ([]byte, error) {
	rv := obj.GetResourceVersion()
	obj.SetResourceVersion("")
	data, err := json.Marshal(obj)
	obj.SetResourceVersion(rv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal object: %w", err)
	}
	return data, nil
}

func unmarshalData(data []byte, rv int64) (*unstructured.Unstructured, error) {
	obj := &unstructured.Unstructured{}
	if err := json.Unmarshal(data, obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal object: %w", err)
	}
	obj.SetResourceVersion(strconv.FormatInt(rv, 10))
	return obj, nil
}

func (s *SpannerStore) Create(ctx context.Context, obj client.Object) error {
	filterValue, err := s.contextFilterValue(ctx)
	if err != nil {
		return err
	}

	namespace := obj.GetNamespace()
	name := obj.GetName()
	useGenerateName := name == "" && obj.GetGenerateName() != ""

	maxAttempts := 1
	if useGenerateName {
		maxAttempts = 5
	}

	for attempt := range maxAttempts {
		if useGenerateName {
			name = storage.GenerateName(obj.GetGenerateName())
			obj.SetName(name)
		}

		now := time.Now()
		creationTime := obj.GetCreationTimestamp()
		if creationTime.IsZero() {
			obj.SetCreationTimestamp(metav1.NewTime(now))
		}

		data, err := marshalData(obj)
		if err != nil {
			return err
		}

		labelsJSON, err := json.Marshal(obj.GetLabels())
		if err != nil {
			return fmt.Errorf("failed to marshal labels: %w", err)
		}

		var rv int64
		_, err = s.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
			// Get next resource version
			row, readErr := txn.ReadRow(ctx, s.countersTable, spanner.Key{s.counterID}, []string{"value"})
			if readErr != nil {
				if spanner.ErrCode(readErr) == codes.NotFound {
					rv = 1
				} else {
					return readErr
				}
			} else {
				var current int64
				if err := row.Column(0, &current); err != nil {
					return err
				}
				rv = current + 1
			}

			mutations := []*spanner.Mutation{
				spanner.InsertOrUpdate(s.countersTable, []string{"counter_id", "value"}, []interface{}{s.counterID, rv}),
				spanner.Insert(s.tableName,
					[]string{"context_filter", "namespace", "name", "resource_version", "labels", "data", "created_at", "updated_at"},
					[]interface{}{filterValue, namespace, name, rv, spanner.NullJSON{Value: json.RawMessage(labelsJSON), Valid: true}, spanner.NullJSON{Value: json.RawMessage(data), Valid: true}, now, now},
				),
			}
			return txn.BufferWrite(mutations)
		})
		if err != nil {
			if spanner.ErrCode(err) == codes.AlreadyExists {
				if useGenerateName && attempt < maxAttempts-1 {
					continue
				}
				return errors.NewAlreadyExists(
					schema.GroupResource{Resource: s.resourceType},
					name,
				)
			}
			return fmt.Errorf("failed to create object: %w", err)
		}

		obj.SetResourceVersion(strconv.FormatInt(rv, 10))

		if s.broadcaster != nil {
			s.broadcaster.Broadcast(storage.ResourceEvent{
				Type:               storage.EventAdded,
				Object:             obj.DeepCopyObject().(client.Object),
				ResourceVersion:    strconv.FormatInt(rv, 10),
				ContextFilterValue: filterValue,
			})
		}

		return nil
	}

	return fmt.Errorf("failed to generate unique name after retries")
}

func (s *SpannerStore) Get(ctx context.Context, namespace, name string) (client.Object, error) {
	filterValue, err := s.contextFilterValue(ctx)
	if err != nil {
		return nil, err
	}

	row, err := s.client.Single().ReadRow(ctx, s.tableName,
		spanner.Key{filterValue, namespace, name},
		[]string{"data", "resource_version"},
	)
	if err != nil {
		if spanner.ErrCode(err) == codes.NotFound {
			return nil, errors.NewNotFound(
				schema.GroupResource{Resource: s.resourceType},
				name,
			)
		}
		return nil, fmt.Errorf("failed to get object: %w", err)
	}

	var dataJSON spanner.NullJSON
	var rv int64
	if err := row.Columns(&dataJSON, &rv); err != nil {
		return nil, fmt.Errorf("failed to scan row: %w", err)
	}

	dataBytes, err := json.Marshal(dataJSON.Value)
	if err != nil {
		return nil, fmt.Errorf("failed to re-marshal data: %w", err)
	}

	obj, err := unmarshalData(dataBytes, rv)
	if err != nil {
		return nil, err
	}

	return obj, nil
}

func (s *SpannerStore) List(ctx context.Context, opts storage.ListOptions) (client.ObjectList, error) {
	filterValue, err := s.contextFilterValue(ctx)
	if err != nil {
		return nil, err
	}

	qb := NewQueryBuilder(s.tableName, "namespace", "name", "resource_version", "data")

	if s.contextFilterKey != nil {
		qb.WhereContextFilter(filterValue)
	}
	qb.WhereNamespace(opts.Namespace)

	if opts.LabelSelector != "" {
		selector, err := labels.Parse(opts.LabelSelector)
		if err != nil {
			return nil, err
		}
		qb.WhereLabelSelector(selector)
	}

	qb.WhereShardSelector(opts.ShardSelector)
	qb.WhereFieldFilters(opts.FieldFilters)

	if opts.Continue != "" {
		token, err := storage.DecodeContinueToken(opts.Continue)
		if err == nil {
			qb.WhereContinueToken(token)
		}
	}

	qb.OrderBy("namespace", "name")

	if opts.Limit > 0 {
		qb.Limit(opts.Limit + 1)
	}

	query, params := qb.Build()
	stmt := spanner.Statement{SQL: query, Params: params}
	iter := s.client.Single().Query(ctx, stmt)
	defer iter.Stop()

	var items []unstructured.Unstructured
	var maxRV int64
	rowCount := int64(0)

	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to iterate rows: %w", err)
		}

		rowCount++

		if opts.Limit > 0 && rowCount > opts.Limit {
			break
		}

		var namespace, name string
		var rv int64
		var dataJSON spanner.NullJSON
		if err := row.Columns(&namespace, &name, &rv, &dataJSON); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		dataBytes, err := json.Marshal(dataJSON.Value)
		if err != nil {
			return nil, fmt.Errorf("failed to re-marshal data: %w", err)
		}

		obj, err := unmarshalData(dataBytes, rv)
		if err != nil {
			return nil, err
		}

		items = append(items, *obj)
		if rv > maxRV {
			maxRV = rv
		}
	}

	listGVK := s.gvk.GroupVersion().WithKind(s.gvk.Kind + "List")
	listObj, err := s.scheme.New(listGVK)
	if err != nil {
		return nil, fmt.Errorf("failed to create list object: %w", err)
	}

	list := listObj.(*unstructured.UnstructuredList)
	list.SetResourceVersion(strconv.FormatInt(maxRV, 10))
	list.Items = items

	hasMore := opts.Limit > 0 && rowCount > opts.Limit
	if hasMore && len(items) > 0 {
		listMeta, err := meta.ListAccessor(list)
		if err == nil {
			lastItem := &items[len(items)-1]
			token := &storage.ContinueToken{
				Namespace:       lastItem.GetNamespace(),
				Name:            lastItem.GetName(),
				ResourceVersion: strconv.FormatInt(maxRV, 10),
			}
			continueStr, err := storage.EncodeContinueToken(token)
			if err == nil {
				listMeta.SetContinue(continueStr)
			}
		}
	}

	return list, nil
}

func (s *SpannerStore) Update(ctx context.Context, obj client.Object) error {
	filterValue, err := s.contextFilterValue(ctx)
	if err != nil {
		return err
	}

	namespace := obj.GetNamespace()
	name := obj.GetName()

	_, err = s.Get(ctx, namespace, name)
	if err != nil {
		return err
	}

	data, err := marshalData(obj)
	if err != nil {
		return err
	}

	labelsJSON, err := json.Marshal(obj.GetLabels())
	if err != nil {
		return fmt.Errorf("failed to marshal labels: %w", err)
	}

	var rv int64
	_, err = s.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		// Verify object still exists
		_, readErr := txn.ReadRow(ctx, s.tableName,
			spanner.Key{filterValue, namespace, name},
			[]string{"resource_version"},
		)
		if readErr != nil {
			if spanner.ErrCode(readErr) == codes.NotFound {
				return errors.NewNotFound(schema.GroupResource{Resource: s.resourceType}, name)
			}
			return readErr
		}

		// Get next resource version
		counterRow, readErr := txn.ReadRow(ctx, s.countersTable, spanner.Key{s.counterID}, []string{"value"})
		if readErr != nil {
			if spanner.ErrCode(readErr) == codes.NotFound {
				rv = 1
			} else {
				return readErr
			}
		} else {
			var current int64
			if err := counterRow.Column(0, &current); err != nil {
				return err
			}
			rv = current + 1
		}

		now := time.Now()
		return txn.BufferWrite([]*spanner.Mutation{
			spanner.InsertOrUpdate(s.countersTable, []string{"counter_id", "value"}, []interface{}{s.counterID, rv}),
			spanner.Update(s.tableName,
				[]string{"context_filter", "namespace", "name", "resource_version", "labels", "data", "updated_at"},
				[]interface{}{filterValue, namespace, name, rv, spanner.NullJSON{Value: json.RawMessage(labelsJSON), Valid: true}, spanner.NullJSON{Value: json.RawMessage(data), Valid: true}, now},
			),
		})
	})
	if err != nil {
		return fmt.Errorf("failed to update object: %w", err)
	}

	obj.SetResourceVersion(strconv.FormatInt(rv, 10))

	if s.broadcaster != nil {
		s.broadcaster.Broadcast(storage.ResourceEvent{
			Type:               storage.EventModified,
			Object:             obj.DeepCopyObject().(client.Object),
			ResourceVersion:    strconv.FormatInt(rv, 10),
			ContextFilterValue: filterValue,
		})
	}

	return nil
}

func (s *SpannerStore) Delete(ctx context.Context, namespace, name string) error {
	filterValue, err := s.contextFilterValue(ctx)
	if err != nil {
		return err
	}

	obj, err := s.Get(ctx, namespace, name)
	if err != nil {
		return err
	}

	var rv int64
	_, err = s.client.ReadWriteTransaction(ctx, func(ctx context.Context, txn *spanner.ReadWriteTransaction) error {
		// Verify still exists
		_, readErr := txn.ReadRow(ctx, s.tableName,
			spanner.Key{filterValue, namespace, name},
			[]string{"resource_version"},
		)
		if readErr != nil {
			if spanner.ErrCode(readErr) == codes.NotFound {
				return errors.NewNotFound(schema.GroupResource{Resource: s.resourceType}, name)
			}
			return readErr
		}

		// Get next resource version
		counterRow, readErr := txn.ReadRow(ctx, s.countersTable, spanner.Key{s.counterID}, []string{"value"})
		if readErr != nil {
			if spanner.ErrCode(readErr) == codes.NotFound {
				rv = 1
			} else {
				return readErr
			}
		} else {
			var current int64
			if err := counterRow.Column(0, &current); err != nil {
				return err
			}
			rv = current + 1
		}

		return txn.BufferWrite([]*spanner.Mutation{
			spanner.InsertOrUpdate(s.countersTable, []string{"counter_id", "value"}, []interface{}{s.counterID, rv}),
			spanner.Delete(s.tableName, spanner.Key{filterValue, namespace, name}),
		})
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	if s.broadcaster != nil {
		s.broadcaster.Broadcast(storage.ResourceEvent{
			Type:               storage.EventDeleted,
			Object:             obj,
			ResourceVersion:    strconv.FormatInt(rv, 10),
			ContextFilterValue: filterValue,
		})
	}

	return nil
}

func (s *SpannerStore) Watch(ctx context.Context, opts storage.ListOptions, resourceVersion string) (<-chan storage.ResourceEvent, func(), error) {
	filterValue, err := s.contextFilterValue(ctx)
	if err != nil {
		return nil, nil, err
	}

	if s.broadcaster == nil {
		return nil, nil, fmt.Errorf("broadcaster not configured")
	}

	eventCh, stopSubscription, err := s.broadcaster.Subscribe(resourceVersion)
	if err != nil {
		return nil, nil, err
	}

	outCh := make(chan storage.ResourceEvent, 100)
	stopCh := make(chan struct{})

	go func() {
		defer close(outCh)
		defer stopSubscription()

		var labelSelector labels.Selector
		if opts.LabelSelector != "" {
			var err error
			labelSelector, err = labels.Parse(opts.LabelSelector)
			if err != nil {
				labelSelector = nil
			}
		}

		for {
			select {
			case <-stopCh:
				return
			case event, ok := <-eventCh:
				if !ok {
					return
				}

				if s.contextFilterKey != nil {
					if event.ContextFilterValue != filterValue {
						continue
					}
				}

				clientObj, ok := event.Object.(client.Object)
				if !ok {
					continue
				}

				if opts.Namespace != "" && clientObj.GetNamespace() != opts.Namespace {
					continue
				}

				if labelSelector != nil && !labelSelector.Matches(labels.Set(clientObj.GetLabels())) {
					continue
				}

				if opts.ShardSelector != nil {
					matches, err := storage.MatchesShard(clientObj, opts.ShardSelector)
					if err != nil || !matches {
						continue
					}
				}

				if len(opts.FieldFilters) > 0 && !matchesFieldFilters(clientObj, opts.FieldFilters) {
					continue
				}

				select {
				case outCh <- event:
				case <-stopCh:
					return
				}
			}
		}
	}()

	stopFunc := func() {
		close(stopCh)
	}

	return outCh, stopFunc, nil
}

func matchesFieldFilters(obj client.Object, filters map[string]string) bool {
	data, err := json.Marshal(obj)
	if err != nil {
		return false
	}
	var objMap map[string]interface{}
	if err := json.Unmarshal(data, &objMap); err != nil {
		return false
	}
	for path, expected := range filters {
		if fieldValueFromMap(objMap, path) != expected {
			return false
		}
	}
	return true
}

func fieldValueFromMap(m map[string]interface{}, path string) string {
	parts := strings.Split(path, ".")
	current := interface{}(m)
	for _, part := range parts {
		cm, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current = cm[part]
	}
	s, _ := current.(string)
	return s
}

var _ storage.ResourceStore = (*SpannerStore)(nil)
