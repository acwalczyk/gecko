// Package firestore implements transport.Client using Google Cloud Firestore
// as the transport layer via kube-applier-gcp desire documents.
package firestore

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/option"

	"github.com/openshift-online/gecko/controllers/client/transport"
	"github.com/openshift-online/gecko/controllers/util/logger"
	"github.com/openshift-online/kube-applier-gcp/pkg/api/kubeapplier"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
)

const (
	collectionApplyDesires  = "applydesires"
	collectionReadDesires   = "readdesires"
	collectionDeleteDesires = "deletedesires"
)

const (
	specsDBName  = "specs"
	statusDBName = "status"
)

// mcClients caches the Firestore client pair for one management cluster.
type mcClients struct {
	specs  *firestore.Client // specs DB (apply/read/delete desires written here)
	status *firestore.Client // status DB (status read back from here)
}

// Client implements transport.Client using Firestore as the transport.
// One pair of *firestore.Client is maintained per management cluster.
// MCs are identified by their GCP project ID.
type Client struct {
	mu    sync.RWMutex
	cache map[string]*mcClients
	log   logger.Logger
	// dialOpts are extra grpc/firestore client options, used to inject emulator settings in tests.
	dialOpts []option.ClientOption
}

// Ensure Client implements transport.Client.
var _ transport.Client = (*Client)(nil)

// New creates a new Firestore transport client.
// The management cluster name passed to Apply/GetStatus/Delete is used directly
// as the GCP project ID.
// Use opts to inject emulator settings in tests (e.g. option.WithEndpoint).
func New(log logger.Logger, opts ...option.ClientOption) *Client {
	return &Client{
		cache:    make(map[string]*mcClients),
		log:      log,
		dialOpts: opts,
	}
}

// clients returns (or lazily creates) the Firestore client pair for the given MC.
func (c *Client) clients(ctx context.Context, mcName string) (*mcClients, error) {
	c.mu.RLock()
	if mc, ok := c.cache[mcName]; ok {
		c.mu.RUnlock()
		return mc, nil
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Double-check after acquiring write lock.
	if mc, ok := c.cache[mcName]; ok {
		return mc, nil
	}

	// MCs are identified by their GCP project ID.
	specsClient, err := firestore.NewClientWithDatabase(ctx, mcName, specsDBName, c.dialOpts...)
	if err != nil {
		return nil, fmt.Errorf("firestore transport: create specs client for MC %q: %w", mcName, err)
	}

	statusClient, err := firestore.NewClientWithDatabase(ctx, mcName, statusDBName, c.dialOpts...)
	if err != nil {
		specsClient.Close() //nolint:errcheck
		return nil, fmt.Errorf("firestore transport: create status client for MC %q: %w", mcName, err)
	}

	mc := &mcClients{specs: specsClient, status: statusClient}
	c.cache[mcName] = mc
	return mc, nil
}

// Apply decomposes the manifests into individual resources and writes one
// ApplyDesire + one ReadDesire document per resource to the specs DB.
// Returns the current status by calling GetStatus after writing.
func (c *Client) Apply(ctx context.Context, targetCluster, clusterID string, manifests [][]byte) (*transport.Status, error) {
	mc, err := c.clients(ctx, targetCluster)
	if err != nil {
		return nil, err
	}

	batch := mc.specs.BulkWriter(ctx)
	var jobs []*firestore.BulkWriterJob

	for _, raw := range manifests {
		if len(raw) == 0 {
			continue
		}

		ref, unknownKind, err := parseManifest(raw)
		if err != nil {
			return nil, fmt.Errorf("firestore transport: Apply %s/%s: %w", targetCluster, clusterID, err)
		}
		if unknownKind {
			c.log.Infof(ctx, "firestore transport: Apply %s/%s: unknown Kind for resource %s/%s — using fallback pluralization, add it to kindToResource if incorrect", targetCluster, clusterID, ref.Namespace, ref.Name)
		}

		// Write ApplyDesire
		applyID, applyData, err := buildApplyDesireDoc(clusterID, targetCluster, ref, raw)
		if err != nil {
			return nil, fmt.Errorf("firestore transport: Apply %s/%s build apply desire: %w", targetCluster, clusterID, err)
		}
		applyRef := mc.specs.Collection(collectionApplyDesires).Doc(applyID)
		job, err := batch.Set(applyRef, applyData)
		if err != nil {
			return nil, fmt.Errorf("firestore transport: Apply %s/%s set apply desire: %w", targetCluster, clusterID, err)
		}
		jobs = append(jobs, job)

		// Write ReadDesire
		readID, readData := buildReadDesireDoc(clusterID, targetCluster, ref)
		readRef := mc.specs.Collection(collectionReadDesires).Doc(readID)
		job, err = batch.Set(readRef, readData)
		if err != nil {
			return nil, fmt.Errorf("firestore transport: Apply %s/%s set read desire: %w", targetCluster, clusterID, err)
		}
		jobs = append(jobs, job)
	}

	batch.Flush()

	// Check job results for write errors.
	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return nil, fmt.Errorf("firestore transport: Apply %s/%s write error: %w", targetCluster, clusterID, err)
		}
	}

	c.log.Infof(ctx, "firestore transport: applied %d manifests for %s/%s", len(manifests), targetCluster, clusterID)

	return c.GetStatus(ctx, targetCluster, clusterID)
}

// GetStatus looks up document IDs from the specs DB by clusterID, then fetches
// the corresponding status documents from the status DB by document ID.
// This two-step approach is needed because kube-applier-gcp does not copy the
// spec fields into the status DB — only the document IDs match across DBs.
func (c *Client) GetStatus(ctx context.Context, targetCluster, clusterID string) (*transport.Status, error) {
	mc, err := c.clients(ctx, targetCluster)
	if err != nil {
		return nil, err
	}

	// Step 1: Query specs DB to discover document IDs for this clusterID.
	specsApplySnaps, err := mc.specs.Collection(collectionApplyDesires).
		Where("spec.clusterID", "==", clusterID).
		Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("firestore transport: GetStatus %s/%s query specs apply desires: %w", targetCluster, clusterID, err)
	}

	specsReadSnaps, err := mc.specs.Collection(collectionReadDesires).
		Where("spec.clusterID", "==", clusterID).
		Documents(ctx).GetAll()
	if err != nil {
		return nil, fmt.Errorf("firestore transport: GetStatus %s/%s query specs read desires: %w", targetCluster, clusterID, err)
	}

	// Step 2: Fetch the corresponding documents from the status DB by ID.
	// GetAll preserves input order, so statusSnaps[i] corresponds to specsSnaps[i].
	// The spec fields in status docs are empty (kube-applier-gcp doesn't copy them),
	// so we take the spec from the specs DB snapshot instead.
	applyDesires := make([]kubeapplier.ApplyDesire, 0, len(specsApplySnaps))
	if len(specsApplySnaps) > 0 {
		applyRefs := make([]*firestore.DocumentRef, len(specsApplySnaps))
		for i, snap := range specsApplySnaps {
			applyRefs[i] = mc.status.Collection(collectionApplyDesires).Doc(snap.Ref.ID)
		}
		statusApplySnaps, err := mc.status.GetAll(ctx, applyRefs)
		if err != nil {
			return nil, fmt.Errorf("firestore transport: GetStatus %s/%s fetch status apply desires: %w", targetCluster, clusterID, err)
		}
		for i, snap := range statusApplySnaps {
			if !snap.Exists() {
				continue
			}
			var ad kubeapplier.ApplyDesire
			if err := snap.DataTo(&ad); err != nil {
				return nil, fmt.Errorf("firestore transport: GetStatus %s/%s decode apply desire %s: %w", targetCluster, clusterID, snap.Ref.ID, err)
			}
			// Spec from status DB is empty — use the spec from the specs DB.
			var specsAD kubeapplier.ApplyDesire
			if err := specsApplySnaps[i].DataTo(&specsAD); err != nil {
				return nil, fmt.Errorf("firestore transport: GetStatus %s/%s decode specs apply desire %s: %w", targetCluster, clusterID, snap.Ref.ID, err)
			}
			ad.Spec = specsAD.Spec
			applyDesires = append(applyDesires, ad)
		}
	}

	readDesires := make([]kubeapplier.ReadDesire, 0, len(specsReadSnaps))
	if len(specsReadSnaps) > 0 {
		readRefs := make([]*firestore.DocumentRef, len(specsReadSnaps))
		for i, snap := range specsReadSnaps {
			readRefs[i] = mc.status.Collection(collectionReadDesires).Doc(snap.Ref.ID)
		}
		statusReadSnaps, err := mc.status.GetAll(ctx, readRefs)
		if err != nil {
			return nil, fmt.Errorf("firestore transport: GetStatus %s/%s fetch status read desires: %w", targetCluster, clusterID, err)
		}
		for i, snap := range statusReadSnaps {
			if !snap.Exists() {
				continue
			}
			var rd kubeapplier.ReadDesire
			if err := snap.DataTo(&rd); err != nil {
				return nil, fmt.Errorf("firestore transport: GetStatus %s/%s decode read desire %s: %w", targetCluster, clusterID, snap.Ref.ID, err)
			}
			// Spec from status DB is empty — use the spec from the specs DB.
			var specsRD kubeapplier.ReadDesire
			if err := specsReadSnaps[i].DataTo(&specsRD); err != nil {
				return nil, fmt.Errorf("firestore transport: GetStatus %s/%s decode specs read desire %s: %w", targetCluster, clusterID, snap.Ref.ID, err)
			}
			rd.Spec = specsRD.Spec
			// Manually decode status_kubeContent (stored as map[string]any at doc root).
			if v, ok := snap.Data()["status_kubeContent"]; ok && v != nil {
				raw, err := json.Marshal(v)
				if err != nil {
					return nil, fmt.Errorf("firestore transport: GetStatus marshal status_kubeContent: %w", err)
				}
				rd.Status.KubeContent = &k8sruntime.RawExtension{Raw: raw}
			}
			readDesires = append(readDesires, rd)
		}
	}

	return &transport.Status{
		Conditions:       aggregateConditions(applyDesires),
		ResourceStatuses: extractResourceStatuses(readDesires),
	}, nil
}

// Delete writes one DeleteDesire document per resource and removes the
// corresponding ApplyDesire and ReadDesire documents from the specs DB.
func (c *Client) Delete(ctx context.Context, targetCluster, clusterID string) error {
	mc, err := c.clients(ctx, targetCluster)
	if err != nil {
		return err
	}

	// Query ApplyDesire specs docs to find all resources for this clusterID.
	applySnaps, err := mc.specs.Collection(collectionApplyDesires).
		Where("spec.clusterID", "==", clusterID).
		Documents(ctx).GetAll()
	if err != nil {
		return fmt.Errorf("firestore transport: Delete %s/%s query apply desires: %w", targetCluster, clusterID, err)
	}

	if len(applySnaps) == 0 {
		c.log.Infof(ctx, "firestore transport: Delete %s/%s: no apply desires found, nothing to delete", targetCluster, clusterID)
		return nil
	}

	batch := mc.specs.BulkWriter(ctx)
	var jobs []*firestore.BulkWriterJob

	for _, snap := range applySnaps {
		var ad kubeapplier.ApplyDesire
		if err := snap.DataTo(&ad); err != nil {
			return fmt.Errorf("firestore transport: Delete %s/%s decode apply desire: %w", targetCluster, clusterID, err)
		}

		ref := ad.Spec.TargetItem
		taskKey := ad.Spec.ClusterID

		// Write DeleteDesire
		deleteID, deleteData := buildDeleteDesireDoc(taskKey, targetCluster, ref)
		deleteRef := mc.specs.Collection(collectionDeleteDesires).Doc(deleteID)
		job, err := batch.Set(deleteRef, deleteData)
		if err != nil {
			return fmt.Errorf("firestore transport: Delete %s/%s set delete desire: %w", targetCluster, clusterID, err)
		}
		jobs = append(jobs, job)

		// Delete the ReadDesire doc (same document ID as the ApplyDesire).
		readRef := mc.specs.Collection(collectionReadDesires).Doc(snap.Ref.ID)
		job, err = batch.Delete(readRef)
		if err != nil {
			return fmt.Errorf("firestore transport: Delete %s/%s delete read desire: %w", targetCluster, clusterID, err)
		}
		jobs = append(jobs, job)

		// Delete the ApplyDesire doc.
		job, err = batch.Delete(snap.Ref)
		if err != nil {
			return fmt.Errorf("firestore transport: Delete %s/%s delete apply desire: %w", targetCluster, clusterID, err)
		}
		jobs = append(jobs, job)
	}

	batch.Flush()

	// Check job results for write errors.
	for _, job := range jobs {
		if _, err := job.Results(); err != nil {
			return fmt.Errorf("firestore transport: Delete %s/%s write error: %w", targetCluster, clusterID, err)
		}
	}

	c.log.Infof(ctx, "firestore transport: deleted %d resources for %s/%s", len(applySnaps), targetCluster, clusterID)
	return nil
}

// Close closes all cached Firestore clients. Call on shutdown.
func (c *Client) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, mc := range c.cache {
		mc.specs.Close()  //nolint:errcheck
		mc.status.Close() //nolint:errcheck
	}
	c.cache = make(map[string]*mcClients)
}
