package spanner

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"sync"
	"time"

	"cloud.google.com/go/spanner"
	"github.com/openshift-online/gecko/orlop/pkg/apiserver/storage"
	"google.golang.org/api/iterator"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type SpannerBroadcaster struct {
	client    *spanner.Client
	tableName string
	ctx       context.Context
	cancel    context.CancelFunc
	scheme    *runtime.Scheme
	gvk       schema.GroupVersionKind

	mu          sync.RWMutex
	subscribers map[int]chan storage.ResourceEvent
	nextID      int
	closed      bool
	lastRV      int64
}

type SpannerBroadcasterConfig struct {
	Client    *spanner.Client
	TableName string
	Scheme    *runtime.Scheme
	GVK       schema.GroupVersionKind
}

func NewSpannerBroadcaster(ctx context.Context, config SpannerBroadcasterConfig) (*SpannerBroadcaster, error) {
	if config.Client == nil {
		return nil, fmt.Errorf("spanner client is required")
	}

	tableName := config.TableName
	if tableName == "" {
		tableName = "event_log"
	}

	bCtx, cancel := context.WithCancel(ctx)

	b := &SpannerBroadcaster{
		client:      config.Client,
		tableName:   tableName,
		ctx:         bCtx,
		cancel:      cancel,
		scheme:      config.Scheme,
		gvk:         config.GVK,
		subscribers: make(map[int]chan storage.ResourceEvent),
	}

	go b.pollForEvents()

	return b, nil
}

func (b *SpannerBroadcaster) pollForEvents() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-b.ctx.Done():
			return
		case <-ticker.C:
			b.fetchNewEvents()
		}
	}
}

func (b *SpannerBroadcaster) fetchNewEvents() {
	b.mu.RLock()
	lastRV := b.lastRV
	subscriberCount := len(b.subscribers)
	b.mu.RUnlock()

	if subscriberCount == 0 {
		return
	}

	stmt := spanner.Statement{
		SQL: fmt.Sprintf(
			"SELECT rv, event_type, object_data, context_filter FROM %s WHERE rv > @lastRV ORDER BY rv ASC LIMIT 100",
			b.tableName,
		),
		Params: map[string]interface{}{
			"lastRV": lastRV,
		},
	}

	iter := b.client.Single().Query(b.ctx, stmt)
	defer iter.Stop()

	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return
		}

		var rv int64
		var eventType string
		var objectDataJSON spanner.NullJSON
		var contextFilter string
		if err := row.Columns(&rv, &eventType, &objectDataJSON, &contextFilter); err != nil {
			continue
		}

		objectDataBytes, err := json.Marshal(objectDataJSON.Value)
		if err != nil {
			continue
		}

		obj, err := b.reconstructObject(objectDataBytes)
		if err != nil {
			continue
		}

		obj.SetResourceVersion(strconv.FormatInt(rv, 10))

		event := storage.ResourceEvent{
			Type:               storage.EventType(eventType),
			ResourceVersion:    strconv.FormatInt(rv, 10),
			Object:             obj,
			ContextFilterValue: contextFilter,
		}

		b.broadcastToSubscribers(event)

		b.mu.Lock()
		if rv > b.lastRV {
			b.lastRV = rv
		}
		b.mu.Unlock()
	}
}

func (b *SpannerBroadcaster) reconstructObject(data []byte) (client.Object, error) {
	if b.scheme == nil || b.gvk.Empty() {
		obj := &unstructured.Unstructured{}
		if err := json.Unmarshal(data, &obj.Object); err != nil {
			return nil, err
		}
		return obj, nil
	}

	obj, err := b.scheme.New(b.gvk)
	if err != nil {
		unstruct := &unstructured.Unstructured{}
		if err := json.Unmarshal(data, &unstruct.Object); err != nil {
			return nil, err
		}
		unstruct.SetGroupVersionKind(b.gvk)
		return unstruct, nil
	}

	if err := json.Unmarshal(data, obj); err != nil {
		return nil, fmt.Errorf("failed to unmarshal into typed object: %w", err)
	}

	obj.GetObjectKind().SetGroupVersionKind(b.gvk)

	clientObj, ok := obj.(client.Object)
	if !ok {
		return nil, fmt.Errorf("object does not implement client.Object")
	}

	return clientObj, nil
}

func (b *SpannerBroadcaster) broadcastToSubscribers(event storage.ResourceEvent) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for _, ch := range b.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

func (b *SpannerBroadcaster) Broadcast(event storage.ResourceEvent) {
	b.mu.RLock()
	if b.closed {
		b.mu.RUnlock()
		return
	}
	b.mu.RUnlock()

	rv, _ := strconv.ParseInt(event.ResourceVersion, 10, 64)

	objectData, err := marshalData(event.Object)
	if err != nil {
		return
	}

	contextFilter := ""
	if event.ContextFilterValue != "" {
		contextFilter = event.ContextFilterValue
	}

	now := time.Now()
	_, err = b.client.Apply(b.ctx, []*spanner.Mutation{
		spanner.Insert(b.tableName,
			[]string{"rv", "event_type", "object_data", "context_filter", "created_at"},
			[]interface{}{rv, string(event.Type), spanner.NullJSON{Value: json.RawMessage(objectData), Valid: true}, contextFilter, now},
		),
	})
	if err != nil {
		fmt.Printf("Failed to insert event: %v\n", err)
	}
}

func (b *SpannerBroadcaster) Subscribe(sinceResourceVersion string) (<-chan storage.ResourceEvent, func(), error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil, nil, fmt.Errorf("broadcaster is closed")
	}

	id := b.nextID
	b.nextID++

	ch := make(chan storage.ResourceEvent, 100)
	b.subscribers[id] = ch

	if sinceResourceVersion != "" {
		go b.sendHistoricalEvents(ch, sinceResourceVersion)
	}

	stopFunc := func() {
		b.unsubscribe(id)
	}

	return ch, stopFunc, nil
}

func (b *SpannerBroadcaster) sendHistoricalEvents(ch chan storage.ResourceEvent, sinceResourceVersion string) {
	rv, err := strconv.ParseInt(sinceResourceVersion, 10, 64)
	if err != nil {
		return
	}

	stmt := spanner.Statement{
		SQL: fmt.Sprintf(
			"SELECT rv, event_type, object_data, context_filter FROM %s WHERE rv > @sinceRV ORDER BY rv ASC LIMIT 1000",
			b.tableName,
		),
		Params: map[string]interface{}{
			"sinceRV": rv,
		},
	}

	iter := b.client.Single().Query(context.Background(), stmt)
	defer iter.Stop()

	for {
		row, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return
		}

		var eventRV int64
		var eventType string
		var objectDataJSON spanner.NullJSON
		var contextFilter string
		if err := row.Columns(&eventRV, &eventType, &objectDataJSON, &contextFilter); err != nil {
			continue
		}

		objectDataBytes, err := json.Marshal(objectDataJSON.Value)
		if err != nil {
			continue
		}

		obj, err := b.reconstructObject(objectDataBytes)
		if err != nil {
			continue
		}

		obj.SetResourceVersion(strconv.FormatInt(eventRV, 10))

		event := storage.ResourceEvent{
			Type:               storage.EventType(eventType),
			ResourceVersion:    strconv.FormatInt(eventRV, 10),
			Object:             obj,
			ContextFilterValue: contextFilter,
		}

		select {
		case ch <- event:
		default:
			return
		}
	}
}

func (b *SpannerBroadcaster) unsubscribe(id int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if ch, exists := b.subscribers[id]; exists {
		close(ch)
		delete(b.subscribers, id)
	}
}

func (b *SpannerBroadcaster) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true
	b.cancel()

	for id, ch := range b.subscribers {
		close(ch)
		delete(b.subscribers, id)
	}

	return nil
}

func (b *SpannerBroadcaster) PruneOldEvents(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().Add(-olderThan)

	stmt := spanner.Statement{
		SQL: fmt.Sprintf("DELETE FROM %s WHERE created_at < @cutoff", b.tableName),
		Params: map[string]interface{}{
			"cutoff": cutoff,
		},
	}

	count, err := b.client.PartitionedUpdate(ctx, stmt)
	if err != nil {
		return fmt.Errorf("failed to prune events: %w", err)
	}

	fmt.Printf("Pruned %d old events\n", count)
	return nil
}

var (
	_ storage.EventBroadcaster = (*SpannerBroadcaster)(nil)
	_ storage.EventPruner      = (*SpannerBroadcaster)(nil)
)
