package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/openshift-online/gecko/orlop/pkg/apiserver/storage"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ParentFilter holds the parent resource filter extracted from a nested route.
type ParentFilter struct {
	IDField string
	ID      string
}

type parentFilterKey struct{}

// WithParentFilter returns a new context carrying the given ParentFilter.
func WithParentFilter(ctx context.Context, pf ParentFilter) context.Context {
	return context.WithValue(ctx, parentFilterKey{}, pf)
}

func parentFilterFromContext(ctx context.Context) *ParentFilter {
	pf, ok := ctx.Value(parentFilterKey{}).(ParentFilter)
	if !ok {
		return nil
	}
	return &pf
}

func applyParentFilterToListOpts(ctx context.Context, opts *storage.ListOptions) {
	pf := parentFilterFromContext(ctx)
	if pf == nil {
		return
	}
	if opts.FieldFilters == nil {
		opts.FieldFilters = make(map[string]string)
	}
	opts.FieldFilters[pf.IDField] = pf.ID
}

func validateParentOnCreate(ctx context.Context, objMap map[string]interface{}) error {
	pf := parentFilterFromContext(ctx)
	if pf == nil {
		return nil
	}
	if fieldValueFromMap(objMap, pf.IDField) != pf.ID {
		return fmt.Errorf("field %s must be %q when creating via nested route", pf.IDField, pf.ID)
	}
	return nil
}

func validateParentOwnership(ctx context.Context, obj client.Object) bool {
	pf := parentFilterFromContext(ctx)
	if pf == nil {
		return true
	}
	data, err := json.Marshal(obj)
	if err != nil {
		return false
	}
	var objMap map[string]interface{}
	if err := json.Unmarshal(data, &objMap); err != nil {
		return false
	}
	return fieldValueFromMap(objMap, pf.IDField) == pf.ID
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
