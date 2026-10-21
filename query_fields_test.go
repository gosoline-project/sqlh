package sqlh

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/validation"
	"github.com/stretchr/testify/require"
)

type queryContractApp struct {
	sqlr.Entity[int]
	Name string `db:"display_name" json:"name"`
}

type queryContractEntity struct {
	sqlr.Entity[int]
	PublicId  string             `db:"public_id" json:"id"`
	StartDate string             `db:"start_date" json:"startDate" sqlh:"field:startDate"`
	Secret    string             `db:"secret" json:"-" sqlh:"field:-"`
	AppId     *int               `db:"app_id" json:"appId"`
	App       queryContractApp   `db:"-" json:"app" sqlr:"belongsTo:app_id"`
	Apps      []queryContractApp `db:"-" json:"apps" sqlr:"foreignKey:app_id"`
}

func queryContractResolver(t *testing.T, fields map[string]string) *queryFieldResolver {
	t.Helper()
	schema, err := sqlr.ParseSchema[queryContractEntity]()
	require.NoError(t, err)
	resolver, err := newQueryFieldResolver(reflect.TypeFor[queryContractEntity](), schema, fields)
	require.NoError(t, err)

	return resolver
}

func TestPublicQueryFieldContract(t *testing.T) {
	resolver := queryContractResolver(t, map[string]string{"id": "PublicId", "app.name": "App.Name"})
	resolved, err := resolver.resolve(JsonFilter{Type: "or", Fields: []JsonFilter{
		{Type: "eq", Field: "id", Value: "public-7"},
		{Type: "not", Fields: []JsonFilter{{Type: "like", Field: "app.name", Value: "x%"}}},
	}}, []JsonOrder{{Field: "app.name"}, {Field: "id", Direction: "DESC"}})
	require.NoError(t, err)
	require.Equal(t, []string{"App"}, resolved.filterJoins)
	require.Empty(t, resolved.orderJoins)
	qb := sqlr.NewQueryBuilderSelect()
	require.NoError(t, resolved.applyFilters(qb))
	resolved.applyOrder(qb)
	query, args, err := qb.ToSql()
	require.NoError(t, err)
	require.Contains(t, query, "`public_id` = ?")
	require.Contains(t, query, "`App`.`display_name` LIKE ?")
	require.NotContains(t, query, "`id` = ?")
	require.Equal(t, []any{"public-7", "x%"}, args)
}

func TestPublicQueryFieldsRejectIdentifiersAndMalformedTrees(t *testing.T) {
	resolver := queryContractResolver(t, map[string]string{"id": "PublicId"})
	for _, field := range []string{"public_id", "query_contract_entities.public_id", "`public_id`) OR 1=1 --", "id desc", ""} {
		_, err := resolver.resolve(JsonFilter{Type: "eq", Field: field, Value: 1}, nil)
		require.Error(t, err, field)
		require.True(t, validation.IsValidationError(err))
	}
	for _, filter := range []JsonFilter{
		{Field: "id"},
		{Type: "and"},
		{Type: "not", Fields: []JsonFilter{{Type: "eq", Field: "id", Value: 1}, {Type: "eq", Field: "id", Value: 2}}},
		{Type: "and", Fields: []JsonFilter{{}}},
		{Type: "eq", Field: "id", Value: 1, Fields: []JsonFilter{{Type: "eq", Field: "id", Value: 2}}},
	} {
		_, err := resolver.resolve(filter, nil)
		require.Error(t, err)
	}
}

func TestPublicQueryFieldsAutomaticAndExplicitAllowlist(t *testing.T) {
	automatic := queryContractResolver(t, nil)
	require.Contains(t, automatic.fields, "id")
	require.Contains(t, automatic.fields, "startdate")
	require.Contains(t, automatic.fields, "publicid")
	require.NotContains(t, automatic.fields, "Secret")
	require.NotContains(t, automatic.fields, "secret")
	require.NotContains(t, automatic.fields, "app.name")
	require.NotContains(t, automatic.fields, "app")
	require.NotContains(t, automatic.fields, "apps")
	explicit := queryContractResolver(t, map[string]string{"id": "PublicId"})
	require.Len(t, explicit.fields, 1)
	require.Contains(t, explicit.fields["id"].column, "public_id")
	require.Empty(t, queryContractResolver(t, map[string]string{}).fields)
}

func TestPublicQueryFieldsInvalidConfiguration(t *testing.T) {
	schema, err := sqlr.ParseSchema[queryContractEntity]()
	require.NoError(t, err)
	for _, path := range []string{"Missing", "public_id", "Apps.Name", "App.App.Name", "App", "App.Missing"} {
		_, err := newQueryFieldResolver(reflect.TypeFor[queryContractEntity](), schema, map[string]string{"field": path})
		require.Error(t, err, path)
	}
}

func TestPublicQuerySortOnlyJoinsAreSeparateFromCount(t *testing.T) {
	resolver := queryContractResolver(t, map[string]string{"id": "PublicId", "app.name": "App.Name"})
	resolved, err := resolver.resolve(JsonFilter{Type: "eq", Field: "id", Value: "p"}, []JsonOrder{{Field: "app.name"}, {Field: "app.name", Direction: "desc"}})
	require.NoError(t, err)
	require.Empty(t, resolved.filterJoins)
	require.Equal(t, []string{"App"}, resolved.orderJoins)
	count := sqlr.NewQueryBuilderSelect()
	require.NoError(t, resolved.applyFilters(count))
	query, _, err := count.ToSql()
	require.NoError(t, err)
	require.NotContains(t, query, "ORDER BY")
}

func TestPublicQueryRejectsObsoleteJSONDialects(t *testing.T) {
	for _, body := range []string{
		`{"filter":{"column":"id"}}`,
		`{"filter":{"type":"eq","column":"id","value":1}}`,
		`{"filter":{"type":"and","fields":[{"type":"eq","column":"id","value":1}]}}`,
		`{"filter":{"bool":"and","matches":[]}}`,
		`{"orderBy":[{"column":"id"}]}`,
	} {
		var input ListInput
		require.Error(t, json.Unmarshal([]byte(body), &input), body)
	}
}

type queryContractHyphenEntity struct {
	sqlr.Entity[int]
	Name string `db:"display_name" json:"display-name" sqlh:"field:display-name"`
}

func TestPublicQueryFieldsAcceptPublicAliasesWithoutSQLFallback(t *testing.T) {
	schema, err := sqlr.ParseSchema[queryContractHyphenEntity]()
	require.NoError(t, err)
	for _, configured := range []map[string]string{nil, {"display-name": "Name"}} {
		t.Run(fmt.Sprintf("explicit=%t", configured != nil), func(t *testing.T) {
			resolver, err := newQueryFieldResolver(reflect.TypeFor[queryContractHyphenEntity](), schema, configured)
			require.NoError(t, err)
			var input ListInput
			require.NoError(t, json.Unmarshal([]byte(`{"filter":{"type":"eq","field":"display-name","value":"first"},"orderBy":[{"field":"display-name","direction":"desc"}]}`), &input))
			resolved, err := resolver.resolve(input.GetFilter(), input.GetOrderBy())
			require.NoError(t, err)
			qb := sqlr.NewQueryBuilderSelect()
			require.NoError(t, resolved.applyFilters(qb))
			resolved.applyOrder(qb)
			query, params, err := qb.ToSql()
			require.NoError(t, err)
			column := "`" + schema.TableName + "`.`display_name`"
			require.Equal(t, "WHERE "+column+" = ? ORDER BY "+column+" DESC", query)
			require.Equal(t, []any{"first"}, params)
			for _, field := range []string{"display_name", schema.TableName + ".display_name", "display-name desc", "Name"} {
				_, err = resolver.resolve(JsonFilter{Type: "eq", Field: field, Value: "first"}, nil)
				require.Error(t, err)
				_, err = resolver.resolve(JsonFilter{}, []JsonOrder{{Field: field}})
				require.Error(t, err)
			}
		})
	}
}
