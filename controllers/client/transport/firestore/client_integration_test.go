//go:build integration

package firestore_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	fstransport "github.com/openshift-online/gecko/controllers/client/transport/firestore"
	"github.com/openshift-online/gecko/controllers/util/logger"
	"github.com/openshift-online/kube-applier-gcp/pkg/api/kubeapplier"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// testMCName is the GCP project ID that identifies the MC.
	testMCName    = "test-project"
	testClusterID = "cluster-abc"
)

func emulatorOpts(t *testing.T) []option.ClientOption {
	t.Helper()
	host := os.Getenv("FIRESTORE_EMULATOR_HOST")
	if host == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set — skipping integration test")
	}
	return []option.ClientOption{
		option.WithEndpoint(host),
		option.WithoutAuthentication(),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),
	}
}

func newTestClient(t *testing.T) *fstransport.Client {
	t.Helper()
	log, err := logger.NewLogger(logger.DefaultConfig())
	require.NoError(t, err)
	opts := emulatorOpts(t)
	return fstransport.New(log, opts...)
}

func hcManifest(t *testing.T, clusterID, clusterName string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"apiVersion": "hypershift.openshift.io/v1beta1",
		"kind":       "HostedCluster",
		"metadata":   map[string]any{"name": clusterName, "namespace": fmt.Sprintf("clusters-%s", clusterID)},
	})
	require.NoError(t, err)
	return raw
}

func npManifest(t *testing.T, clusterID, npName string) []byte {
	t.Helper()
	raw, err := json.Marshal(map[string]any{
		"apiVersion": "hypershift.openshift.io/v1beta1",
		"kind":       "NodePool",
		"metadata":   map[string]any{"name": npName, "namespace": fmt.Sprintf("clusters-%s", clusterID)},
	})
	require.NoError(t, err)
	return raw
}

// clearCollection deletes all documents in a collection (used between tests).
func clearCollection(ctx context.Context, t *testing.T, client *firestore.Client, coll string) {
	t.Helper()
	snaps, err := client.Collection(coll).Documents(ctx).GetAll()
	require.NoError(t, err)
	for _, snap := range snaps {
		_, err := snap.Ref.Delete(ctx)
		require.NoError(t, err)
	}
}

// applyDesireSpec builds a minimal ApplyDesireSpec for the given clusterID and resource name.
func applyDesireSpec(clusterID, name string) kubeapplier.ApplyDesireSpec {
	return kubeapplier.ApplyDesireSpec{
		ManagementCluster: testMCName,
		ClusterID:         clusterID,
		TargetItem: kubeapplier.ResourceReference{
			Group:     "hypershift.openshift.io",
			Version:   "v1beta1",
			Resource:  "hostedclusters",
			Namespace: fmt.Sprintf("clusters-%s", clusterID),
			Name:      name,
		},
	}
}

// specsApplyDesire returns an ApplyDesire suitable for writing to the specs DB.
func specsApplyDesire(clusterID, name string) kubeapplier.ApplyDesire {
	return kubeapplier.ApplyDesire{Spec: applyDesireSpec(clusterID, name)}
}

// statusApplyDesire returns an ApplyDesire as kube-applier-gcp writes it to the
// status DB: spec fields are empty, status carries the applied conditions.
func statusApplyDesire(conditions []metav1.Condition) kubeapplier.ApplyDesire {
	return kubeapplier.ApplyDesire{
		Status: kubeapplier.ApplyDesireStatus{Conditions: conditions},
	}
}

func TestIntegration_Apply_WritesApplyAndReadDesires(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)
	opts := emulatorOpts(t)

	specsClient, err := firestore.NewClientWithDatabase(ctx, testMCName, "specs", opts...)
	require.NoError(t, err)
	defer specsClient.Close()
	clearCollection(ctx, t, specsClient, "applydesires")
	clearCollection(ctx, t, specsClient, "readdesires")
	defer clearCollection(ctx, t, specsClient, "applydesires")
	defer clearCollection(ctx, t, specsClient, "readdesires")

	manifests := [][]byte{
		hcManifest(t, testClusterID, "my-hc"),
	}

	_, err = c.Apply(ctx, testMCName, testClusterID, manifests)
	require.NoError(t, err)

	snaps, err := specsClient.Collection("applydesires").
		Where("spec.clusterID", "==", testClusterID).
		Documents(ctx).GetAll()
	require.NoError(t, err)
	assert.Len(t, snaps, 1, "expected 1 ApplyDesire for the HostedCluster manifest")

	data := snaps[0].Data()
	assert.NotContains(t, data, "manifestWorkName")
	assert.NotNil(t, data["spec_kubeContent"])

	readSnaps, err := specsClient.Collection("readdesires").
		Where("spec.clusterID", "==", testClusterID).
		Documents(ctx).GetAll()
	require.NoError(t, err)
	assert.Len(t, readSnaps, 1, "expected 1 ReadDesire for the HostedCluster manifest")
}

func TestIntegration_GetStatus_AllSuccessful(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)
	opts := emulatorOpts(t)

	specsClient, err := firestore.NewClientWithDatabase(ctx, testMCName, "specs", opts...)
	require.NoError(t, err)
	defer specsClient.Close()

	statusClient, err := firestore.NewClientWithDatabase(ctx, testMCName, "status", opts...)
	require.NoError(t, err)
	defer statusClient.Close()

	clearCollection(ctx, t, specsClient, "applydesires")
	clearCollection(ctx, t, statusClient, "applydesires")
	clearCollection(ctx, t, statusClient, "readdesires")
	defer clearCollection(ctx, t, specsClient, "applydesires")
	defer clearCollection(ctx, t, statusClient, "applydesires")
	defer clearCollection(ctx, t, statusClient, "readdesires")

	const docID = "doc-1"

	_, err = specsClient.Collection("applydesires").Doc(docID).Set(ctx, specsApplyDesire(testClusterID, "my-hc"))
	require.NoError(t, err)

	_, err = statusClient.Collection("applydesires").Doc(docID).Set(ctx, statusApplyDesire([]metav1.Condition{
		{Type: "Successful", Status: metav1.ConditionTrue, Reason: "NoErrors"},
	}))
	require.NoError(t, err)

	status, err := c.GetStatus(ctx, testMCName, testClusterID)
	require.NoError(t, err)
	require.Len(t, status.Conditions, 1)
	assert.Equal(t, "Applied", status.Conditions[0].Type)
	assert.Equal(t, metav1.ConditionTrue, status.Conditions[0].Status)
}

// TestIntegration_GetStatus_MissingStatusDocReportsPending verifies that when
// some resources have no status document yet, GetStatus reports Applied=False
// rather than prematurely reporting Applied=True based on the subset that do.
func TestIntegration_GetStatus_MissingStatusDocReportsPending(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)
	opts := emulatorOpts(t)

	specsClient, err := firestore.NewClientWithDatabase(ctx, testMCName, "specs", opts...)
	require.NoError(t, err)
	defer specsClient.Close()

	statusClient, err := firestore.NewClientWithDatabase(ctx, testMCName, "status", opts...)
	require.NoError(t, err)
	defer statusClient.Close()

	clearCollection(ctx, t, specsClient, "applydesires")
	clearCollection(ctx, t, statusClient, "applydesires")
	defer clearCollection(ctx, t, specsClient, "applydesires")
	defer clearCollection(ctx, t, statusClient, "applydesires")

	// Two resources in specs DB.
	_, err = specsClient.Collection("applydesires").Doc("doc-1").Set(ctx, specsApplyDesire(testClusterID, "hc-1"))
	require.NoError(t, err)
	_, err = specsClient.Collection("applydesires").Doc("doc-2").Set(ctx, specsApplyDesire(testClusterID, "hc-2"))
	require.NoError(t, err)

	// Only doc-1 has a status doc with Successful=True; doc-2 is missing.
	_, err = statusClient.Collection("applydesires").Doc("doc-1").Set(ctx, statusApplyDesire([]metav1.Condition{
		{Type: "Successful", Status: metav1.ConditionTrue, Reason: "NoErrors"},
	}))
	require.NoError(t, err)

	status, err := c.GetStatus(ctx, testMCName, testClusterID)
	require.NoError(t, err)
	require.Len(t, status.Conditions, 1)
	assert.Equal(t, "Applied", status.Conditions[0].Type)
	// Must be False/Unknown — not True — because doc-2 has no status yet.
	assert.NotEqual(t, metav1.ConditionTrue, status.Conditions[0].Status)
}

func TestIntegration_GetStatus_ExtractsHCKubeContent(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)
	opts := emulatorOpts(t)

	specsClient, err := firestore.NewClientWithDatabase(ctx, testMCName, "specs", opts...)
	require.NoError(t, err)
	defer specsClient.Close()

	statusClient, err := firestore.NewClientWithDatabase(ctx, testMCName, "status", opts...)
	require.NoError(t, err)
	defer statusClient.Close()

	clearCollection(ctx, t, specsClient, "readdesires")
	clearCollection(ctx, t, statusClient, "readdesires")
	defer clearCollection(ctx, t, specsClient, "readdesires")
	defer clearCollection(ctx, t, statusClient, "readdesires")

	const docID = "rd-1"

	specsReadDesire := kubeapplier.ReadDesire{
		Spec: kubeapplier.ReadDesireSpec{
			ManagementCluster: testMCName,
			ClusterID:         testClusterID,
			TargetItem: kubeapplier.ResourceReference{
				Group:     "hypershift.openshift.io",
				Version:   "v1beta1",
				Resource:  "hostedclusters",
				Namespace: "clusters-abc",
				Name:      "my-hc",
			},
		},
	}
	_, err = specsClient.Collection("readdesires").Doc(docID).Set(ctx, specsReadDesire)
	require.NoError(t, err)

	// Status DB: spec fields empty (kube-applier-gcp doesn't copy them),
	// status_kubeContent carries the live object — stored at doc root, not in the struct.
	hcLiveObject := map[string]any{
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Available", "status": "True"},
			},
			"controlPlaneEndpoint": map[string]any{"host": "api.my-hc.example.com"},
			"version": map[string]any{
				"history": []any{map[string]any{"version": "4.14.1", "state": "Completed"}},
			},
		},
	}
	_, err = statusClient.Collection("readdesires").Doc(docID).Set(ctx, map[string]any{
		"spec":               kubeapplier.ReadDesireSpec{},
		"status":             kubeapplier.ReadDesireStatus{},
		"status_kubeContent": hcLiveObject,
	})
	require.NoError(t, err)

	status, err := c.GetStatus(ctx, testMCName, testClusterID)
	require.NoError(t, err)

	key := "hypershift.openshift.io/v1beta1/hostedclusters/clusters-abc/my-hc"
	require.Contains(t, status.ResourceStatuses, key)
	assert.Equal(t, "True", status.ResourceStatuses[key]["availableCondition"])
	assert.Equal(t, "api.my-hc.example.com", status.ResourceStatuses[key]["controlPlaneEndpoint"])
	assert.Equal(t, "4.14.1", status.ResourceStatuses[key]["version"])
}

func TestIntegration_Delete_WritesDeleteDesireAndRemovesApplyRead(t *testing.T) {
	ctx := context.Background()
	c := newTestClient(t)
	opts := emulatorOpts(t)

	specsClient, err := firestore.NewClientWithDatabase(ctx, testMCName, "specs", opts...)
	require.NoError(t, err)
	defer specsClient.Close()

	clearCollection(ctx, t, specsClient, "applydesires")
	clearCollection(ctx, t, specsClient, "readdesires")
	clearCollection(ctx, t, specsClient, "deletedesires")
	defer clearCollection(ctx, t, specsClient, "applydesires")
	defer clearCollection(ctx, t, specsClient, "readdesires")
	defer clearCollection(ctx, t, specsClient, "deletedesires")

	manifests := [][]byte{
		npManifest(t, testClusterID, "my-np"),
	}

	_, err = c.Apply(ctx, testMCName, testClusterID, manifests)
	require.NoError(t, err)

	err = c.Delete(ctx, testMCName, testClusterID)
	require.NoError(t, err)

	applySnaps, err := specsClient.Collection("applydesires").
		Where("spec.clusterID", "==", testClusterID).
		Documents(ctx).GetAll()
	require.NoError(t, err)
	assert.Empty(t, applySnaps, "ApplyDesires should be deleted")

	readSnaps, err := specsClient.Collection("readdesires").
		Where("spec.clusterID", "==", testClusterID).
		Documents(ctx).GetAll()
	require.NoError(t, err)
	assert.Empty(t, readSnaps, "ReadDesires should be deleted")

	deleteSnaps, err := specsClient.Collection("deletedesires").Documents(ctx).GetAll()
	require.NoError(t, err)
	assert.Len(t, deleteSnaps, 1, "DeleteDesire should have been written")

	var dd kubeapplier.DeleteDesire
	require.NoError(t, deleteSnaps[0].DataTo(&dd))
	assert.Equal(t, testClusterID, dd.Spec.ClusterID)
}
