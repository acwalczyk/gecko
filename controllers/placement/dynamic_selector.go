package placement

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"google.golang.org/api/iterator"
)

// mcRegistrationLabelFilter is the Secret Manager list filter used to discover
// mc-registration-* secrets created by gcp-hcp-infra
// (terraform/modules/management-cluster/region-registration.tf).
const mcRegistrationLabelFilter = "labels.mc-registration:true"

// secretLookup abstracts Secret Manager operations so they can be replaced in
// tests without a live GCP connection.
type secretLookup interface {
	listSecrets(ctx context.Context, parent, filter string) ([]*secretmanagerpb.Secret, error)
	accessSecretVersion(ctx context.Context, name string) ([]byte, error)
}

// realSMClient adapts *secretmanager.Client to the secretLookup interface.
type realSMClient struct {
	c *secretmanager.Client
}

func (r *realSMClient) listSecrets(ctx context.Context, parent, filter string) ([]*secretmanagerpb.Secret, error) {
	it := r.c.ListSecrets(ctx, &secretmanagerpb.ListSecretsRequest{
		Parent: parent,
		Filter: filter,
	})
	var secrets []*secretmanagerpb.Secret
	for {
		secret, err := it.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		secrets = append(secrets, secret)
	}
	return secrets, nil
}

func (r *realSMClient) accessSecretVersion(ctx context.Context, name string) ([]byte, error) {
	result, err := r.c.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	})
	if err != nil {
		return nil, err
	}
	return result.Payload.Data, nil
}

// mcRegistrationPayload is the JSON content of an mc-registration-* secret
// version, written by terraform/modules/management-cluster/region-registration.tf.
type mcRegistrationPayload struct {
	ProjectID string `json:"projectId"`
	Mode      string `json:"mode"` // "active" or "maintenance"
}

// DynamicSelector discovers eligible management clusters at selection time by
// reading mc-registration-* Secret Manager secrets, and round-robins across a
// statically configured list of HC DNS zone domains.
//
// MC discovery:
//  1. List SM secrets labeled mc-registration=true
//  2. Access each secret's latest version and parse the JSON payload
//     ({"projectId": "...", "mode": "active|maintenance"})
//  3. Eligible = secrets where mode == "active"
//
// DNS domains are injected at construction time (Helm value → CLI flag),
// since they are a region-level resource shared across all MCs, not
// discoverable per-MC.
//
// Selection is round-robin across eligible MCs and configured domains.
type DynamicSelector struct {
	smLookup secretLookup
	project  string
	domains  []string

	mcCounter  atomic.Uint64
	domCounter atomic.Uint64
}

// NewDynamicSelector creates a DynamicSelector.
// project is the GCP project ID for Secret Manager lookups.
// domains is the list of HC DNS zone domains to round-robin across; it must
// be non-empty.
func NewDynamicSelector(smClient *secretmanager.Client, project string, domains []string) (*DynamicSelector, error) {
	if len(domains) == 0 {
		return nil, fmt.Errorf("hc dns domains: at least one domain is required")
	}
	return &DynamicSelector{
		smLookup: &realSMClient{c: smClient},
		project:  project,
		domains:  domains,
	}, nil
}

// Select discovers eligible MCs dynamically, then picks one MC and one
// configured DNS domain using round-robin counters.
func (s *DynamicSelector) Select(ctx context.Context) (mcName, baseDomain string, err error) {
	eligible, err := s.eligibleMCs(ctx)
	if err != nil {
		return "", "", fmt.Errorf("discover eligible MCs: %w", err)
	}
	if len(eligible) == 0 {
		return "", "", fmt.Errorf("no eligible management clusters found (check mc-registration secrets and mode=active)")
	}
	if len(s.domains) == 0 {
		return "", "", fmt.Errorf("no HC DNS domains configured")
	}

	mc := eligible[s.mcCounter.Add(1)%uint64(len(eligible))]
	domain := s.domains[s.domCounter.Add(1)%uint64(len(s.domains))]

	return mc, domain, nil
}

// eligibleMCs lists mc-registration-* secrets, reads each secret's latest
// version, and returns the projectId of every secret whose mode is "active".
func (s *DynamicSelector) eligibleMCs(ctx context.Context) ([]string, error) {
	secrets, err := s.smLookup.listSecrets(ctx,
		fmt.Sprintf("projects/%s", s.project),
		mcRegistrationLabelFilter,
	)
	if err != nil {
		return nil, fmt.Errorf("list mc-registration secrets: %w", err)
	}

	var eligible []string
	for _, secret := range secrets {
		data, err := s.smLookup.accessSecretVersion(ctx, secret.Name+"/versions/latest")
		if err != nil {
			return nil, fmt.Errorf("access secret %s: %w", secret.Name, err)
		}

		var payload mcRegistrationPayload
		if err := json.Unmarshal(data, &payload); err != nil {
			return nil, fmt.Errorf("unmarshal secret %s payload: %w", secret.Name, err)
		}

		if payload.Mode == "active" && payload.ProjectID != "" {
			eligible = append(eligible, payload.ProjectID)
		}
	}
	return eligible, nil
}
