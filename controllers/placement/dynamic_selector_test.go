package placement

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─── mock secretLookup ────────────────────────────────────────────────────────

// mockSMLookup implements secretLookup for tests.
// listResponses maps filter string to the secrets returned for that filter.
// accessResponses/accessErrs map a secret version name to its payload/error.
// A single listErr short-circuits all listSecrets calls.
type mockSMLookup struct {
	listResponses   map[string][]*secretmanagerpb.Secret
	listErr         error
	accessResponses map[string][]byte
	accessErrs      map[string]error
	accessedNames   []string // names passed to accessSecretVersion, in order
}

func (m *mockSMLookup) listSecrets(_ context.Context, _, filter string) ([]*secretmanagerpb.Secret, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.listResponses[filter], nil
}

func (m *mockSMLookup) accessSecretVersion(_ context.Context, name string) ([]byte, error) {
	m.accessedNames = append(m.accessedNames, name)
	if err, ok := m.accessErrs[name]; ok {
		return nil, err
	}
	return m.accessResponses[name], nil
}

// ─── helpers ──────────────────────────────────────────────────────────────────

const testProject = "my-project"

// smSecret builds a minimal secretmanagerpb.Secret with the given name and labels.
func smSecret(name string, labels map[string]string) *secretmanagerpb.Secret {
	return &secretmanagerpb.Secret{Name: name, Labels: labels}
}

// mcRegistrationJSON encodes an mc-registration secret payload.
func mcRegistrationJSON(projectID, mode string) []byte {
	b, _ := json.Marshal(mcRegistrationPayload{ProjectID: projectID, Mode: mode})
	return b
}

// newSelector constructs a DynamicSelector using the given mock SM lookup and
// static DNS domain list.
func newSelector(sm secretLookup, domains []string) *DynamicSelector {
	return &DynamicSelector{
		smLookup: sm,
		project:  testProject,
		domains:  domains,
	}
}

// registrationSecret builds a secret + its accessSecretVersion registration
// for a given mc-registration secret name, project ID, and mode.
func registrationSecret(sm *mockSMLookup, name, projectID, mode string) *secretmanagerpb.Secret {
	secret := smSecret(name, map[string]string{"mc-registration": "true"})
	if sm.accessResponses == nil {
		sm.accessResponses = map[string][]byte{}
	}
	sm.accessResponses[name+"/versions/latest"] = mcRegistrationJSON(projectID, mode)
	return secret
}

// ─── NewDynamicSelector ─────────────────────────────────────────────────────────

func TestNewDynamicSelector(t *testing.T) {
	t.Run("errors when domains empty", func(t *testing.T) {
		_, err := NewDynamicSelector(nil, testProject, nil)
		require.Error(t, err)
	})

	t.Run("succeeds with domains", func(t *testing.T) {
		s, err := NewDynamicSelector(nil, testProject, []string{"a.example.com"})
		require.NoError(t, err)
		assert.Equal(t, []string{"a.example.com"}, s.domains)
		assert.Equal(t, testProject, s.project)
	})
}

// ─── eligibleMCs ──────────────────────────────────────────────────────────────

func TestDynamicSelector_eligibleMCs(t *testing.T) {
	ctx := context.Background()

	t.Run("returns projectIds for secrets with mode=active", func(t *testing.T) {
		sm := &mockSMLookup{}
		s1 := registrationSecret(sm, "projects/p/secrets/mc-registration-a", "proj-a", "active")
		s2 := registrationSecret(sm, "projects/p/secrets/mc-registration-b", "proj-b", "active")
		sm.listResponses = map[string][]*secretmanagerpb.Secret{
			mcRegistrationLabelFilter: {s1, s2},
		}

		s := newSelector(sm, []string{"example.com"})
		eligible, err := s.eligibleMCs(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"proj-a", "proj-b"}, eligible)
	})

	t.Run("filters out secrets with mode=maintenance", func(t *testing.T) {
		sm := &mockSMLookup{}
		s1 := registrationSecret(sm, "projects/p/secrets/mc-registration-a", "proj-a", "active")
		s2 := registrationSecret(sm, "projects/p/secrets/mc-registration-b", "proj-b", "maintenance")
		sm.listResponses = map[string][]*secretmanagerpb.Secret{
			mcRegistrationLabelFilter: {s1, s2},
		}

		s := newSelector(sm, []string{"example.com"})
		eligible, err := s.eligibleMCs(ctx)
		require.NoError(t, err)
		assert.Equal(t, []string{"proj-a"}, eligible)
	})

	t.Run("no secrets → empty slice, no error", func(t *testing.T) {
		sm := &mockSMLookup{listResponses: map[string][]*secretmanagerpb.Secret{}}
		s := newSelector(sm, []string{"example.com"})
		eligible, err := s.eligibleMCs(ctx)
		require.NoError(t, err)
		assert.Empty(t, eligible)
	})

	t.Run("listSecrets error → wrapped error", func(t *testing.T) {
		sm := &mockSMLookup{listErr: fmt.Errorf("gcp unavailable")}
		s := newSelector(sm, []string{"example.com"})
		_, err := s.eligibleMCs(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "list mc-registration secrets")
		assert.Contains(t, err.Error(), "gcp unavailable")
	})

	t.Run("accessSecretVersion error → wrapped error mentioning secret name", func(t *testing.T) {
		sm := &mockSMLookup{
			listResponses: map[string][]*secretmanagerpb.Secret{
				mcRegistrationLabelFilter: {smSecret("projects/p/secrets/mc-registration-a", nil)},
			},
			accessErrs: map[string]error{
				"projects/p/secrets/mc-registration-a/versions/latest": fmt.Errorf("permission denied"),
			},
		}
		s := newSelector(sm, []string{"example.com"})
		_, err := s.eligibleMCs(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "access secret")
		assert.Contains(t, err.Error(), "permission denied")
	})

	t.Run("invalid JSON payload → error", func(t *testing.T) {
		sm := &mockSMLookup{
			listResponses: map[string][]*secretmanagerpb.Secret{
				mcRegistrationLabelFilter: {smSecret("projects/p/secrets/mc-registration-a", nil)},
			},
			accessResponses: map[string][]byte{
				"projects/p/secrets/mc-registration-a/versions/latest": []byte("not-json"),
			},
		}
		s := newSelector(sm, []string{"example.com"})
		_, err := s.eligibleMCs(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unmarshal")
	})

	t.Run("secret with empty projectId is excluded even if active", func(t *testing.T) {
		sm := &mockSMLookup{
			listResponses: map[string][]*secretmanagerpb.Secret{
				mcRegistrationLabelFilter: {smSecret("projects/p/secrets/mc-registration-a", nil)},
			},
			accessResponses: map[string][]byte{
				"projects/p/secrets/mc-registration-a/versions/latest": mcRegistrationJSON("", "active"),
			},
		}
		s := newSelector(sm, []string{"example.com"})
		eligible, err := s.eligibleMCs(ctx)
		require.NoError(t, err)
		assert.Empty(t, eligible)
	})
}

// ─── Select ───────────────────────────────────────────────────────────────────

func TestDynamicSelector_Select(t *testing.T) {
	ctx := context.Background()

	// buildMock wires up SM with mc-registration secrets for the given
	// (projectID, mode) pairs, all active unless mode is overridden.
	buildMock := func(projectIDs ...string) *mockSMLookup {
		sm := &mockSMLookup{}
		secrets := make([]*secretmanagerpb.Secret, 0, len(projectIDs))
		for i, p := range projectIDs {
			name := fmt.Sprintf("projects/p/secrets/mc-registration-%d", i)
			secrets = append(secrets, registrationSecret(sm, name, p, "active"))
		}
		sm.listResponses = map[string][]*secretmanagerpb.Secret{
			mcRegistrationLabelFilter: secrets,
		}
		return sm
	}

	t.Run("happy path: returns single MC and domain", func(t *testing.T) {
		sm := buildMock("proj-a")
		s := newSelector(sm, []string{"us-central1.example.com"})

		mc, domain, err := s.Select(ctx)
		require.NoError(t, err)
		assert.Equal(t, "proj-a", mc)
		assert.Equal(t, "us-central1.example.com", domain)
	})

	t.Run("round-robins across multiple eligible MCs", func(t *testing.T) {
		sm := buildMock("proj-a", "proj-b", "proj-c")
		s := newSelector(sm, []string{"example.com"})

		seen := map[string]bool{}
		for i := 0; i < 6; i++ {
			mc, _, err := s.Select(ctx)
			require.NoError(t, err)
			seen[mc] = true
		}
		assert.True(t, seen["proj-a"], "proj-a should be selected at least once")
		assert.True(t, seen["proj-b"], "proj-b should be selected at least once")
		assert.True(t, seen["proj-c"], "proj-c should be selected at least once")
	})

	t.Run("round-robins across multiple DNS domains independently", func(t *testing.T) {
		sm := buildMock("proj-a")
		s := newSelector(sm, []string{"zone1.example.com", "zone2.example.com"})

		seen := map[string]bool{}
		for i := 0; i < 4; i++ {
			_, domain, err := s.Select(ctx)
			require.NoError(t, err)
			seen[domain] = true
		}
		assert.True(t, seen["zone1.example.com"])
		assert.True(t, seen["zone2.example.com"])
	})

	t.Run("no eligible MCs → error mentioning check hint", func(t *testing.T) {
		sm := &mockSMLookup{listResponses: map[string][]*secretmanagerpb.Secret{}}
		s := newSelector(sm, []string{"example.com"})

		_, _, err := s.Select(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no eligible management clusters")
	})

	t.Run("all MCs in maintenance → no eligible MCs error", func(t *testing.T) {
		sm := &mockSMLookup{}
		secret := registrationSecret(sm, "projects/p/secrets/mc-registration-a", "proj-a", "maintenance")
		sm.listResponses = map[string][]*secretmanagerpb.Secret{
			mcRegistrationLabelFilter: {secret},
		}
		s := newSelector(sm, []string{"example.com"})

		_, _, err := s.Select(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no eligible management clusters")
	})

	t.Run("no DNS domains configured → error", func(t *testing.T) {
		sm := buildMock("proj-a")
		s := newSelector(sm, nil)

		_, _, err := s.Select(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "no HC DNS domains")
	})

	t.Run("SM error during eligible MC discovery → error propagated", func(t *testing.T) {
		sm := &mockSMLookup{listErr: fmt.Errorf("gcp outage")}
		s := newSelector(sm, []string{"example.com"})

		_, _, err := s.Select(ctx)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "discover eligible MCs")
	})
}
