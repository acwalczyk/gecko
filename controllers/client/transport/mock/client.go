package mock

import (
	"context"
	"sync"

	"github.com/openshift-online/gecko/controllers/client/transport"
)

// ApplyCall records arguments from an Apply call.
type ApplyCall struct {
	TargetCluster string
	ClusterID     string
	Manifests     [][]byte
}

// DeleteCall records arguments from a Delete call.
type DeleteCall struct {
	TargetCluster string
	ClusterID     string
}

// Client is an in-memory implementation of transport.Client for use in tests.
type Client struct {
	mu sync.RWMutex

	// StatusOverrides allows tests to inject a specific status for Apply/GetStatus calls.
	// Key format: "targetCluster/clusterID".
	StatusOverrides map[string]*transport.Status

	// ApplyCalls records all Apply invocations for test assertions.
	ApplyCalls []ApplyCall

	// DeleteCalls records all Delete invocations for test assertions.
	DeleteCalls []DeleteCall

	// DeleteErr, if non-nil, is returned by Delete.
	DeleteErr error
}

// Ensure Client implements transport.Client.
var _ transport.Client = (*Client)(nil)

// New creates a new in-memory mock Client.
func New() *Client {
	return &Client{
		StatusOverrides: make(map[string]*transport.Status),
	}
}

func storeKey(targetCluster, clusterID string) string {
	return targetCluster + "/" + clusterID
}

// Apply records the call and returns any configured status override.
func (c *Client) Apply(ctx context.Context, targetCluster, clusterID string, manifests [][]byte) (*transport.Status, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.ApplyCalls = append(c.ApplyCalls, ApplyCall{
		TargetCluster: targetCluster,
		ClusterID:     clusterID,
		Manifests:     manifests,
	})

	key := storeKey(targetCluster, clusterID)
	if override, ok := c.StatusOverrides[key]; ok {
		return override, nil
	}
	return &transport.Status{}, nil
}

// GetStatus returns any configured status override, or an empty status.
func (c *Client) GetStatus(ctx context.Context, targetCluster, clusterID string) (*transport.Status, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	key := storeKey(targetCluster, clusterID)
	if override, ok := c.StatusOverrides[key]; ok {
		return override, nil
	}
	return &transport.Status{}, nil
}

// Delete records the call. Always succeeds.
func (c *Client) Delete(ctx context.Context, targetCluster, clusterID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.DeleteCalls = append(c.DeleteCalls, DeleteCall{
		TargetCluster: targetCluster,
		ClusterID:     clusterID,
	})
	return c.DeleteErr
}

// Reset clears all stored state and recorded calls. Useful between test cases.
func (c *Client) Reset() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.StatusOverrides = make(map[string]*transport.Status)
	c.ApplyCalls = nil
	c.DeleteCalls = nil
	c.DeleteErr = nil
}
