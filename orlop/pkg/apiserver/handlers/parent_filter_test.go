package handlers

import (
	"context"
	"testing"

	"github.com/openshift-online/gecko/orlop/pkg/apiserver/storage"
)

func TestParentFilter_ContextRoundTrip(t *testing.T) {
	ctx := context.Background()
	pf := ParentFilter{IDField: "spec.clusterID", ID: "c1"}
	ctx = WithParentFilter(ctx, pf)

	got := parentFilterFromContext(ctx)
	if got == nil {
		t.Fatal("expected parent filter in context, got nil")
	}
	if got.IDField != "spec.clusterID" || got.ID != "c1" {
		t.Fatalf("expected {spec.clusterID c1}, got {%s %s}", got.IDField, got.ID)
	}
}

func TestParentFilter_MissingContext(t *testing.T) {
	ctx := context.Background()
	got := parentFilterFromContext(ctx)
	if got != nil {
		t.Fatalf("expected nil, got %+v", got)
	}
}

func TestApplyParentFilterToListOpts(t *testing.T) {
	ctx := WithParentFilter(context.Background(), ParentFilter{IDField: "spec.clusterID", ID: "c1"})
	opts := &storage.ListOptions{}
	applyParentFilterToListOpts(ctx, opts)

	if opts.FieldFilters == nil {
		t.Fatal("expected FieldFilters to be set")
	}
	if opts.FieldFilters["spec.clusterID"] != "c1" {
		t.Fatalf("expected spec.clusterID=c1, got %v", opts.FieldFilters)
	}
}

func TestApplyParentFilterToListOpts_NoFilter(t *testing.T) {
	ctx := context.Background()
	opts := &storage.ListOptions{}
	applyParentFilterToListOpts(ctx, opts)

	if opts.FieldFilters != nil {
		t.Fatalf("expected nil FieldFilters, got %v", opts.FieldFilters)
	}
}

func TestValidateParentOnCreate(t *testing.T) {
	ctx := WithParentFilter(context.Background(), ParentFilter{IDField: "spec.clusterID", ID: "c1"})

	t.Run("matching field passes", func(t *testing.T) {
		objMap := map[string]interface{}{
			"spec": map[string]interface{}{"clusterID": "c1"},
		}
		if err := validateParentOnCreate(ctx, objMap); err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
	})

	t.Run("mismatched field fails", func(t *testing.T) {
		objMap := map[string]interface{}{
			"spec": map[string]interface{}{"clusterID": "c2"},
		}
		if err := validateParentOnCreate(ctx, objMap); err == nil {
			t.Fatal("expected error for mismatched field")
		}
	})

	t.Run("missing field fails", func(t *testing.T) {
		objMap := map[string]interface{}{
			"spec": map[string]interface{}{},
		}
		if err := validateParentOnCreate(ctx, objMap); err == nil {
			t.Fatal("expected error for missing field")
		}
	})

	t.Run("no parent filter passes", func(t *testing.T) {
		objMap := map[string]interface{}{
			"spec": map[string]interface{}{"clusterID": "anything"},
		}
		if err := validateParentOnCreate(context.Background(), objMap); err != nil {
			t.Fatalf("expected no error without parent filter, got %v", err)
		}
	})
}

func TestFieldValueFromMap(t *testing.T) {
	m := map[string]interface{}{
		"spec": map[string]interface{}{
			"clusterID": "c1",
			"platform": map[string]interface{}{
				"type": "aws",
			},
		},
		"name": "test",
	}

	tests := []struct {
		path     string
		expected string
	}{
		{"spec.clusterID", "c1"},
		{"spec.platform.type", "aws"},
		{"name", "test"},
		{"spec.missing", ""},
		{"nonexistent.path", ""},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := fieldValueFromMap(m, tt.path)
			if got != tt.expected {
				t.Fatalf("fieldValueFromMap(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}
