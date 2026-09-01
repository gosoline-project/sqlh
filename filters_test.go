package sqlh

import (
	"strings"
	"testing"

	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/require"
)

func TestListInputAppliesUserAndForceFiltersWithoutPagination(t *testing.T) {
	input := ListInput{
		Filter: sqlc.JsonFilter{
			Type:   "eq",
			Column: "status",
			Value:  "active",
		},
	}
	input.AddForceFilter(func(qb *sqlr.QueryBuilderSelect) {
		qb.Where(sqlc.Col("account_id").Eq(42))
	})

	qb := sqlr.NewQueryBuilderSelect()
	require.NoError(t, input.ApplyFilters(qb))

	query, params, err := qb.ToSql()
	require.NoError(t, err)
	require.Contains(t, query, "status")
	require.Contains(t, query, "account_id")
	require.ElementsMatch(t, []any{"active", 42}, params)

	input.Page.Limit = 10
	input.Page.Offset = 20
	query, params, err = qb.ToSql()
	require.NoError(t, err)
	require.NotContains(t, query, "LIMIT")
	require.NotContains(t, query, "OFFSET")
	require.Len(t, params, 2)
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
	require.NoError(t, (ListInput{}).ValidatePagination())
}
