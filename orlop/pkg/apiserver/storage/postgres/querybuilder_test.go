package postgres

import (
	"strings"
	"testing"
)

func TestWhereFieldFilters_SingleField(t *testing.T) {
	qb := NewQueryBuilder("test_table", "data")
	qb.WhereFieldFilters(map[string]string{
		"spec.clusterID": "c1",
	})

	query, args := qb.Build()

	if !strings.Contains(query, "data->'spec'->>'clusterID' = $1") {
		t.Fatalf("expected JSONB path query, got: %s", query)
	}
	if len(args) != 1 || args[0] != "c1" {
		t.Fatalf("expected args [c1], got: %v", args)
	}
}

func TestWhereFieldFilters_MultiLevelPath(t *testing.T) {
	qb := NewQueryBuilder("test_table", "data")
	qb.WhereFieldFilters(map[string]string{
		"spec.platform.type": "aws",
	})

	query, args := qb.Build()

	if !strings.Contains(query, "data->'spec'->'platform'->>'type' = $1") {
		t.Fatalf("expected multi-level JSONB path, got: %s", query)
	}
	if len(args) != 1 || args[0] != "aws" {
		t.Fatalf("expected args [aws], got: %v", args)
	}
}

func TestWhereFieldFilters_EmptyFilters(t *testing.T) {
	qb := NewQueryBuilder("test_table", "data")
	qb.WhereFieldFilters(nil)

	query, args := qb.Build()

	if strings.Contains(query, "data->") {
		t.Fatalf("expected no JSONB conditions for nil filters, got: %s", query)
	}
	if len(args) != 0 {
		t.Fatalf("expected no args, got: %v", args)
	}

	qb2 := NewQueryBuilder("test_table", "data")
	qb2.WhereFieldFilters(map[string]string{})

	query2, args2 := qb2.Build()

	if strings.Contains(query2, "data->") {
		t.Fatalf("expected no JSONB conditions for empty filters, got: %s", query2)
	}
	if len(args2) != 0 {
		t.Fatalf("expected no args, got: %v", args2)
	}
}

func TestWhereFieldFilters_SingleLevelPath(t *testing.T) {
	qb := NewQueryBuilder("test_table", "data")
	qb.WhereFieldFilters(map[string]string{
		"name": "test",
	})

	query, args := qb.Build()

	if !strings.Contains(query, "data->>'name' = $1") {
		t.Fatalf("expected single-level JSONB path, got: %s", query)
	}
	if len(args) != 1 || args[0] != "test" {
		t.Fatalf("expected args [test], got: %v", args)
	}
}

func TestWhereFieldFilters_CombinesWithOtherClauses(t *testing.T) {
	qb := NewQueryBuilder("test_table", "data")
	qb.WhereNamespace("default")
	qb.WhereFieldFilters(map[string]string{
		"spec.clusterID": "c1",
	})

	query, args := qb.Build()

	if !strings.Contains(query, "namespace = $1") {
		t.Fatalf("expected namespace clause, got: %s", query)
	}
	if !strings.Contains(query, "data->'spec'->>'clusterID' = $2") {
		t.Fatalf("expected field filter with correct param number, got: %s", query)
	}
	if len(args) != 2 || args[0] != "default" || args[1] != "c1" {
		t.Fatalf("expected args [default c1], got: %v", args)
	}
}
