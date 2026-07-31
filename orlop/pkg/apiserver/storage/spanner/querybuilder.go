package spanner

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/openshift-online/gecko/orlop/pkg/apiserver/storage"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
)

var validLabelKeyRe = regexp.MustCompile(`^[a-zA-Z0-9._\-/]+$`)

type QueryBuilder struct {
	tableName string
	columns   []string
	where     []string
	params    map[string]interface{}
	orderBy   []string
	limit     int64
	paramSeq  int
}

func NewQueryBuilder(tableName string, columns ...string) *QueryBuilder {
	if len(columns) == 0 {
		columns = []string{"*"}
	}
	return &QueryBuilder{
		tableName: tableName,
		columns:   columns,
		params:    map[string]interface{}{},
	}
}

func (qb *QueryBuilder) nextParam(value interface{}) string {
	name := fmt.Sprintf("p%d", qb.paramSeq)
	qb.paramSeq++
	qb.params[name] = value
	return "@" + name
}

func (qb *QueryBuilder) Where(condition string) *QueryBuilder {
	qb.where = append(qb.where, condition)
	return qb
}

func (qb *QueryBuilder) WhereContextFilter(value string) *QueryBuilder {
	p := qb.nextParam(value)
	qb.Where(fmt.Sprintf("context_filter = %s", p))
	return qb
}

func (qb *QueryBuilder) WhereNamespace(namespace string) *QueryBuilder {
	if namespace == "" {
		return qb
	}
	p := qb.nextParam(namespace)
	qb.Where(fmt.Sprintf("namespace = %s", p))
	return qb
}

func (qb *QueryBuilder) WhereLabelSelector(selector labels.Selector) *QueryBuilder {
	if selector == nil {
		return qb
	}
	requirements, _ := selector.Requirements()
	for _, req := range requirements {
		qb.addLabelRequirement(req)
	}
	return qb
}

func (qb *QueryBuilder) addLabelRequirement(req labels.Requirement) {
	key := req.Key()
	if !validLabelKeyRe.MatchString(key) {
		return
	}

	jsonPath := fmt.Sprintf(`JSON_VALUE(labels, '$["%s"]')`, key)
	values := req.Values()

	switch req.Operator() {
	case selection.Exists:
		qb.Where(fmt.Sprintf("%s IS NOT NULL", jsonPath))

	case selection.DoesNotExist:
		qb.Where(fmt.Sprintf("%s IS NULL", jsonPath))

	case selection.Equals, selection.DoubleEquals, selection.In:
		if values.Len() == 1 {
			p := qb.nextParam(values.List()[0])
			qb.Where(fmt.Sprintf("%s = %s", jsonPath, p))
		} else {
			p := qb.nextParam(values.List())
			qb.Where(fmt.Sprintf("%s IN UNNEST(%s)", jsonPath, p))
		}

	case selection.NotEquals, selection.NotIn:
		if values.Len() == 1 {
			p := qb.nextParam(values.List()[0])
			qb.Where(fmt.Sprintf("(%s IS NULL OR %s != %s)", jsonPath, jsonPath, p))
		} else {
			p := qb.nextParam(values.List())
			qb.Where(fmt.Sprintf("(%s IS NULL OR %s NOT IN UNNEST(%s))", jsonPath, jsonPath, p))
		}
	}
}

func (qb *QueryBuilder) WhereShardSelector(selector *storage.ShardSelector) *QueryBuilder {
	if selector == nil {
		return qb
	}
	hashSQL := buildShardHashSQL()
	pCount := qb.nextParam(int64(selector.Count))
	pIndex := qb.nextParam(int64(selector.Index))
	qb.Where(fmt.Sprintf("MOD(MOD(%s, %s) + %s, %s) = %s", hashSQL, pCount, pCount, pCount, pIndex))
	return qb
}

func buildShardHashSQL() string {
	hashExpr := "TO_CODE_POINTS(SHA256(CAST(CONCAT(namespace, '/', name) AS BYTES)))"
	var parts []string
	for i := range 8 {
		shift := 56 - (i * 8)
		parts = append(parts, fmt.Sprintf("CAST(%s[OFFSET(%d)] AS INT64) << %d", hashExpr, i, shift))
	}
	return "(" + strings.Join(parts, " | ") + ")"
}

func (qb *QueryBuilder) WhereFieldFilters(filters map[string]string) *QueryBuilder {
	for path, value := range filters {
		p := qb.nextParam(value)
		qb.Where(fmt.Sprintf("JSON_VALUE(data, '$.%s') = %s", path, p))
	}
	return qb
}

func (qb *QueryBuilder) WhereContinueToken(token *storage.ContinueToken) *QueryBuilder {
	if token == nil {
		return qb
	}
	if token.Namespace != "" {
		pNs := qb.nextParam(token.Namespace)
		pName := qb.nextParam(token.Name)
		qb.Where(fmt.Sprintf("(namespace > %s OR (namespace = %s AND name > %s))", pNs, pNs, pName))
	} else {
		pName := qb.nextParam(token.Name)
		qb.Where(fmt.Sprintf("name > %s", pName))
	}
	return qb
}

func (qb *QueryBuilder) OrderBy(columns ...string) *QueryBuilder {
	qb.orderBy = append(qb.orderBy, columns...)
	return qb
}

func (qb *QueryBuilder) Limit(limit int64) *QueryBuilder {
	if limit > 0 {
		qb.limit = limit
	}
	return qb
}

func (qb *QueryBuilder) Build() (string, map[string]interface{}) {
	query := fmt.Sprintf("SELECT %s FROM %s", strings.Join(qb.columns, ", "), qb.tableName)

	if len(qb.where) > 0 {
		query += " WHERE " + strings.Join(qb.where, " AND ")
	}

	if len(qb.orderBy) > 0 {
		query += " ORDER BY " + strings.Join(qb.orderBy, ", ")
	}

	if qb.limit > 0 {
		p := qb.nextParam(qb.limit)
		query += fmt.Sprintf(" LIMIT %s", p)
	}

	return query, qb.params
}
