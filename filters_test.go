package sqlh

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/validation"
	"github.com/stretchr/testify/require"
)

func TestListInputPublicOrdering(t *testing.T) {
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resolver, err := newQueryFieldResolver(reflect.TypeFor[resourceTestEntity](), schema, map[string]string{"id": "Id", "name": "Name"})
	require.NoError(t, err)
	nameColumn := "`" + schema.TableName + "`.`name`"
	idColumn := "`" + schema.TableName + "`.`id`"
	for _, tc := range []struct {
		name     string
		body     string
		expected string
	}{
		{"omitted", `{}`, "ORDER BY `existing` DESC"},
		{"empty", `{"orderBy":[]}`, "ORDER BY `existing` DESC"},
		{"default ascending", `{"orderBy":[{"field":"name"}]}`, "ORDER BY " + nameColumn + " ASC"},
		{"multiple keys", `{"orderBy":[{"field":"name","direction":"desc"},{"field":"id","direction":"ASC"}]}`, "ORDER BY " + nameColumn + " DESC, " + idColumn + " ASC"},
		{"mixed case", `{"orderBy":[{"field":"name","direction":"DeSc"}]}`, "ORDER BY " + nameColumn + " DESC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input ListInput
			require.NoError(t, json.Unmarshal([]byte(tc.body), &input))
			resolved, err := resolver.resolve(input.GetFilter(), input.GetOrderBy())
			require.NoError(t, err)
			qb := sqlr.NewQueryBuilderSelect().OrderBy(sqlc.Col("existing").Desc())
			resolved.applyOrder(qb)
			query, params, err := qb.ToSql()
			require.NoError(t, err)
			require.Equal(t, tc.expected, query)
			require.Empty(t, params)
		})
	}
}

func TestListInputRejectsInvalidPublicFields(t *testing.T) {
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resolver, err := newQueryFieldResolver(reflect.TypeFor[resourceTestEntity](), schema, nil)
	require.NoError(t, err)
	for _, tc := range []struct {
		name   string
		filter JsonFilter
		order  []JsonOrder
	}{
		{"invalid direction", JsonFilter{}, []JsonOrder{{Field: "name", Direction: "invalid"}}},
		{"database identifier", JsonFilter{}, []JsonOrder{{Field: schema.TableName + ".name"}}},
		{"unknown ordering", JsonFilter{}, []JsonOrder{{Field: "missing"}}},
		{"unknown filter", JsonFilter{Type: sqlc.JsonFilterEq, Field: "missing", Value: "active"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolver.resolve(tc.filter, tc.order)
			require.Error(t, err)
			require.True(t, validation.IsValidationError(err))
		})
	}
}

func TestListInputResolvesPublicFilterWithoutPagination(t *testing.T) {
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resolver, err := newQueryFieldResolver(reflect.TypeFor[resourceTestEntity](), schema, nil)
	require.NoError(t, err)
	input := ListInput{
		Filter: JsonFilter{Type: sqlc.JsonFilterEq, Field: "name", Value: "active"},
		Page:   ListPage{Limit: 10, Offset: 20},
	}
	resolved, err := resolver.resolve(input.GetFilter(), input.GetOrderBy())
	require.NoError(t, err)
	qb := sqlr.NewQueryBuilderSelect()
	require.NoError(t, resolved.applyFilters(qb))
	query, params, err := qb.ToSql()
	require.NoError(t, err)
	require.Equal(t, "WHERE `"+schema.TableName+"`.`name` = ?", query)
	require.Equal(t, []any{"active"}, params)
	require.NotContains(t, query, "LIMIT")
	require.NotContains(t, query, "OFFSET")
}

func TestQueryFieldsExplicitMapIsCompleteAllowlist(t *testing.T) {
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resolver, err := newQueryFieldResolver(reflect.TypeFor[resourceTestEntity](), schema, map[string]string{
		"displayName": "Name",
	})
	require.NoError(t, err)
	resolved, err := resolver.resolve(
		JsonFilter{Type: sqlc.JsonFilterEq, Field: "displayName", Value: "first"},
		[]JsonOrder{{Field: "displayName"}},
	)
	require.NoError(t, err)
	qb := sqlr.NewQueryBuilderSelect()
	require.NoError(t, resolved.applyFilters(qb))
	resolved.applyOrder(qb)
	query, params, err := qb.ToSql()
	require.NoError(t, err)
	require.Equal(t, "WHERE `"+schema.TableName+"`.`name` = ? ORDER BY `"+schema.TableName+"`.`name` ASC", query)
	require.Equal(t, []any{"first"}, params)
	_, err = resolver.resolve(JsonFilter{Type: sqlc.JsonFilterEq, Field: "name", Value: "first"}, nil)
	require.Error(t, err)
	_, err = resolver.resolve(JsonFilter{}, []JsonOrder{{Field: "id"}})
	require.Error(t, err)

	resolver, err = newQueryFieldResolver(reflect.TypeFor[resourceTestEntity](), schema, map[string]string{})
	require.NoError(t, err)
	_, err = resolver.resolve(JsonFilter{}, []JsonOrder{{Field: "name"}})
	require.Error(t, err)
	_, err = resolver.resolve(JsonFilter{}, nil)
	require.NoError(t, err)
}

func TestListInputAppliesPaginationSeparately(t *testing.T) {
	input := ListInput{Page: ListPage{Limit: 10, Offset: 20}}
	qb := sqlr.NewQueryBuilderSelect()

	input.ApplyPagination(qb)
	query, _, err := qb.ToSql()
	require.NoError(t, err)
	require.Contains(t, query, "LIMIT 10")
	require.Contains(t, query, "OFFSET 20")
}

func TestForceFiltersAreRequestLocalAndDefensive(t *testing.T) {
	input := ListInput{}
	filter := ForceFilter(func(*sqlr.QueryBuilderSelect) {})
	input.AddForceFilter(filter)

	filters := input.GetForceFilters()
	require.Len(t, filters, 1)
	filters[0] = nil
	require.NotNil(t, input.GetForceFilters()[0])
}

func TestListInputRejectsNegativePagination(t *testing.T) {
	require.Error(t, (ListInput{Page: ListPage{Limit: -1}}).ValidatePagination())
	require.Error(t, (ListInput{Page: ListPage{Offset: -1}}).ValidatePagination())
	require.Error(t, (ListInput{Page: ListPage{Offset: 1}}).ValidatePagination())
	require.NoError(t, (ListInput{}).ValidatePagination())
	require.NoError(t, (ListInput{Page: ListPage{Limit: 10, Offset: 20}}).ValidatePagination())
}

func TestJsonFilterDecodesNestedWireTreeStrictly(t *testing.T) {
	const depth = 48
	prefix := strings.Repeat(`{"type":"not","fields":[`, depth)
	suffix := strings.Repeat(`]}`, depth)
	var filter JsonFilter
	require.NoError(t, json.Unmarshal([]byte(prefix+`{"type":"eq","field":"display-name","value":"active"}`+suffix), &filter))
	for range depth {
		require.Equal(t, sqlc.JsonFilterNot, filter.Type)
		require.Len(t, filter.Fields, 1)
		filter = filter.Fields[0]
	}
	require.Equal(t, "display-name", filter.Field)
	require.Equal(t, "active", filter.Value)
	require.Error(t, json.Unmarshal([]byte(prefix+`{"type":"eq","field":"display-name","value":"active","column":"unsafe"}`+suffix), &filter))
}
