package sqlh

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/validation"
	"github.com/stretchr/testify/require"
)

func TestListInputNativeOrdering(t *testing.T) {
	for _, tc := range []struct {
		name     string
		body     string
		expected string
	}{
		{"omitted", `{}`, "ORDER BY `existing` DESC"},
		{"empty", `{"orderBy":[]}`, "ORDER BY `existing` DESC"},
		{"default ascending", `{"orderBy":[{"column":"name"}]}`, "ORDER BY `name` ASC"},
		{"multiple keys", `{"orderBy":[{"column":"items.created_at","direction":"desc"},{"column":"items.id","direction":"ASC"}]}`, "ORDER BY `items`.`created_at` DESC, `items`.`id` ASC"},
		{"mixed case", `{"orderBy":[{"column":"name","direction":"DeSc"}]}`, "ORDER BY `name` DESC"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var input ListInput
			require.NoError(t, json.Unmarshal([]byte(tc.body), &input))
			require.NoError(t, input.ValidateQueryModifiers())
			qb := sqlr.NewQueryBuilderSelect().OrderBy(sqlc.Col("existing").Desc())
			input.ApplyQueryModifiers(qb)
			query, params, err := qb.ToSql()
			require.NoError(t, err)
			require.Equal(t, tc.expected, query)
			require.Empty(t, params)
			input.ApplyQueryModifiers(nil)
		})
	}
}

func TestListInputValidatesNativeOrdering(t *testing.T) {
	input := ListInput{OrderBy: []sqlc.JsonOrder{
		{Column: "name"},
		{Column: "id", Direction: "invalid"},
	}}
	err := input.ValidateQueryModifiers()
	require.EqualError(t, err, "validation: orderBy[1]: direction must be asc or desc")
	require.True(t, validation.IsValidationError(err))
	qb := sqlr.NewQueryBuilderSelect().OrderBy("existing")
	input.ApplyQueryModifiers(qb)
	query, _, err := qb.ToSql()
	require.NoError(t, err)
	require.Equal(t, "ORDER BY `existing`", query)
}

func TestListInputAppliesUserFilterWithoutPagination(t *testing.T) {
	input := ListInput{
		Filter: sqlc.JsonFilter{
			Type:   "eq",
			Column: "status",
			Value:  "active",
		},
	}

	qb := sqlr.NewQueryBuilderSelect()
	require.NoError(t, input.ApplyFilters(qb))

	query, params, err := qb.ToSql()
	require.NoError(t, err)
	require.Contains(t, query, "status")
	require.Equal(t, []any{"active"}, params)

	input.Page.Limit = 10
	input.Page.Offset = 20
	query, params, err = qb.ToSql()
	require.NoError(t, err)
	require.NotContains(t, query, "LIMIT")
	require.NotContains(t, query, "OFFSET")
	require.Len(t, params, 1)
}

func TestListInputAppliesPaginationSeparately(t *testing.T) {
	input := ListInput{Page: ListPage{Limit: 10, Offset: 20}}
	qb := sqlr.NewQueryBuilderSelect()

	input.ApplyPagination(qb)
	query, _, err := qb.ToSql()
	require.NoError(t, err)
	require.True(t, strings.Contains(query, "LIMIT 10"))
	require.True(t, strings.Contains(query, "OFFSET 20"))
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
