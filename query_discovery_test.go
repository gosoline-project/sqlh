package sqlh

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gosoline-project/sqlr"
	sqlrmocks "github.com/gosoline-project/sqlr/mocks"
	"github.com/stretchr/testify/require"
)

type queryDiscoveryEntity struct {
	sqlr.Entity[int]
	Name      string     `db:"name" json:"wireName" xml:"xmlName" form:"formName" yaml:"yamlName"`
	Secret    string     `db:"secret" json:"-"`
	Alias     string     `db:"alias" json:"wireAlias" sqlh:"field:display-name"`
	Hidden    string     `db:"hidden" sqlh:"field:-"`
	Folded    string     `db:"folded" sqlh:"field:Σ"`
	Count     *int       `db:"count"`
	When      *time.Time `db:"when"`
	Transient string     `db:"-" sqlh:"field:display-name"`
}

func discoveryResolver[E any](t *testing.T, configured map[string]string) (*queryFieldResolver, error) {
	t.Helper()
	schema, err := sqlr.ParseSchema[E]()
	require.NoError(t, err)

	return newQueryFieldResolver(reflect.TypeFor[E](), schema, configured)
}

func TestAutomaticQueryFieldsIgnoreTransportTags(t *testing.T) {
	resolver, err := discoveryResolver[queryDiscoveryEntity](t, nil)
	require.NoError(t, err)
	for public, column := range map[string]string{"NAME": "name", "secret": "secret", "DISPLAY-NAME": "alias", "ς": "folded", "COUNT": "count", "WHEN": "when"} {
		resolved, err := resolver.resolve(JsonFilter{Type: "eq", Field: public, Value: "value"}, []JsonOrder{{Field: public}})
		require.NoError(t, err)
		qb := sqlr.NewQueryBuilderSelect()
		require.NoError(t, resolved.applyFilters(qb))
		resolved.applyOrder(qb)
		query, args, err := qb.ToSql()
		require.NoError(t, err)
		require.Contains(t, query, "`"+column+"` = ?")
		require.Contains(t, query, "ORDER BY")
		require.Equal(t, []any{"value"}, args)
	}
	for _, public := range []string{"wireName", "xmlName", "formName", "yamlName", "wireAlias", "Alias", "Hidden", "Transient", "display_name"} {
		_, err := resolver.resolve(JsonFilter{Type: "eq", Field: public, Value: "value"}, nil)
		require.Error(t, err, public)
	}
	resolver, err = discoveryResolver[queryDiscoveryEntity](t, map[string]string{"chosen": "Hidden", "other": "Alias"})
	require.NoError(t, err)
	require.Len(t, resolver.fields, 2)
	_, err = resolver.resolve(JsonFilter{Type: "eq", Field: "CHOSEN", Value: "value"}, nil)
	require.NoError(t, err)
	_, err = resolver.resolve(JsonFilter{Type: "eq", Field: "display-name", Value: "value"}, nil)
	require.Error(t, err)
	resolver, err = discoveryResolver[queryDiscoveryEntity](t, map[string]string{})
	require.NoError(t, err)
	_, err = resolver.resolve(JsonFilter{}, []JsonOrder{{Field: "Name"}})
	require.Error(t, err)
}

func TestQueryFieldNamesUseUnicodeSimpleFold(t *testing.T) {
	for _, pair := range [][2]string{{"Name", "nAME"}, {"Σ", "ς"}, {"σ", "Σ"}, {"ſ", "S"}, {"K", "k"}, {"é 名!#$%&()*+-./:;<=>?@[]^_{|}~", "É 名!#$%&()*+-./:;<=>?@[]^_{|}~"}} {
		require.True(t, strings.EqualFold(pair[0], pair[1]))
		resolver, err := discoveryResolver[queryDiscoveryEntity](t, map[string]string{pair[0]: "Name"})
		require.NoError(t, err)
		resolved, err := resolver.resolve(JsonFilter{Type: "eq", Field: pair[1], Value: "value"}, []JsonOrder{{Field: pair[1]}})
		require.NoError(t, err)
		qb := sqlr.NewQueryBuilderSelect()
		require.NoError(t, resolved.applyFilters(qb))
		resolved.applyOrder(qb)
		query, _, err := qb.ToSql()
		require.NoError(t, err)
		require.Contains(t, query, "`name` = ?")
		require.Contains(t, query, "ORDER BY")
	}
	resolver, err := discoveryResolver[queryDiscoveryEntity](t, map[string]string{"createdAt": "Name"})
	require.NoError(t, err)
	for _, name := range []string{"created_at", "created-at", "created at"} {
		_, err = resolver.resolve(JsonFilter{}, []JsonOrder{{Field: name}})
		require.Error(t, err)
	}
}

type queryAliasCollision struct {
	sqlr.Entity[int]
	Public string `db:"public" sqlh:"field:ID"`
}

type queryExcludedCollision struct {
	sqlr.Entity[int]
	Public  string `db:"public" sqlh:"field:-"`
	Ignored string `db:"-" sqlh:"field:ID"`
}

func TestQueryFieldCollisionsAreActiveAndDeterministic(t *testing.T) {
	_, err := discoveryResolver[queryAliasCollision](t, nil)
	require.ErrorContains(t, err, "ID")
	require.ErrorContains(t, err, "Entity.Id")
	require.ErrorContains(t, err, "Public")
	_, err = discoveryResolver[queryAliasCollision](t, map[string]string{"selected": "Public"})
	require.NoError(t, err)
	_, err = discoveryResolver[queryExcludedCollision](t, nil)
	require.NoError(t, err)
	for _, names := range [][2]string{{"Name", "NAME"}, {"Σ", "ς"}, {"S", "ſ"}, {"k", "K"}} {
		var first string
		for range 20 {
			_, err = discoveryResolver[queryDiscoveryEntity](t, map[string]string{names[0]: "Name", names[1]: "Secret"})
			require.Error(t, err)
			require.Contains(t, err.Error(), "Name")
			require.Contains(t, err.Error(), "Secret")
			if first == "" {
				first = err.Error()
			}
			require.Equal(t, first, err.Error())
		}
	}
}

type QueryPromotionLeft struct {
	Value string `db:"left_value" sqlh:"field:left"`
}
type QueryPromotionRight struct {
	Value string `db:"right_value" sqlh:"field:right"`
}
type queryPromotionAmbiguous struct {
	sqlr.Entity[int]
	QueryPromotionLeft
	QueryPromotionRight
}
type queryPromotionShadow struct {
	sqlr.Entity[int]
	QueryPromotionLeft
	Value string `db:"value" sqlh:"field:selected"`
}

func TestQueryFieldsUseGoStructuralVisibilityBeforeAliases(t *testing.T) {
	resolver, err := discoveryResolver[queryPromotionAmbiguous](t, nil)
	require.NoError(t, err)
	for _, name := range []string{"Value", "left", "right"} {
		_, err = resolver.resolve(JsonFilter{}, []JsonOrder{{Field: name}})
		require.Error(t, err)
	}
	resolver, err = discoveryResolver[queryPromotionShadow](t, nil)
	require.NoError(t, err)
	_, err = resolver.resolve(JsonFilter{}, []JsonOrder{{Field: "selected"}})
	require.NoError(t, err)
	_, err = resolver.resolve(JsonFilter{}, []JsonOrder{{Field: "left"}})
	require.Error(t, err)
}

func TestCrudConstructionPropagatesFieldMappingCollision(t *testing.T) {
	repository := sqlrmocks.NewRepositoryTx[int, crudTestEntity](t)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)
	definition := newCrudTestDefinitionWithList[ListInput]()
	definition.FieldMappings = map[string]string{"Name": "Name", "NAME": "Id"}
	handler, err := newCrudHandler(repository, &TxRunner{}, schema, definition)
	require.Nil(t, handler)
	require.ErrorContains(t, err, "failed to resolve query fields")
	require.ErrorContains(t, err, "ambiguous query fields")
	require.ErrorContains(t, err, "Name")
	require.ErrorContains(t, err, "Id")
}

type queryTwoAliasCollision struct {
	sqlr.Entity[int]
	First  string `db:"first" sqlh:"field:chosen"`
	Second string `db:"second" sqlh:"field:CHOSEN"`
}

func TestAutomaticAliasesCannotHideCollisions(t *testing.T) {
	_, err := discoveryResolver[queryTwoAliasCollision](t, nil)
	require.ErrorContains(t, err, "First")
	require.ErrorContains(t, err, "Second")
	require.ErrorContains(t, err, "chosen")
	require.ErrorContains(t, err, "CHOSEN")
	resolver, err := discoveryResolver[queryTwoAliasCollision](t, map[string]string{"only": "Second"})
	require.NoError(t, err)
	_, err = resolver.resolve(JsonFilter{Type: "eq", Field: "ONLY", Value: "value"}, nil)
	require.NoError(t, err)
}

type QueryShadowedAssociation struct {
	ChildId int              `db:"child_id"`
	Child   queryContractApp `db:"-" sqlr:"belongsTo:child_id"`
}

type queryShadowAlias struct {
	sqlr.Entity[int]
	QueryShadowedAssociation
	Child string `db:"child_label" sqlh:"field:chosen"`
}

type queryShadowExcluded struct {
	sqlr.Entity[int]
	QueryShadowedAssociation
	Child string `db:"child_label" sqlh:"field:-"`
}

func TestScalarFieldTagsOnShadowedAssociationConstructResources(t *testing.T) {
	aliasSchema, err := sqlr.ParseSchema[queryShadowAlias]()
	require.NoError(t, err)
	aliasRepository := sqlrmocks.NewRepositoryTx[int, queryShadowAlias](t)
	aliasResource, err := newResource(aliasRepository, aliasSchema, nil)
	require.NoError(t, err)
	resolved, err := aliasResource.queryFields.resolve(JsonFilter{Type: "eq", Field: "CHOSEN", Value: "label"}, nil)
	require.NoError(t, err)
	qb := sqlr.NewQueryBuilderSelect()
	require.NoError(t, resolved.applyFilters(qb))
	query, args, err := qb.ToSql()
	require.NoError(t, err)
	require.Contains(t, query, "`child_label` = ?")
	require.Equal(t, []any{"label"}, args)
	require.Empty(t, resolved.filterJoins)

	excludedSchema, err := sqlr.ParseSchema[queryShadowExcluded]()
	require.NoError(t, err)
	excludedRepository := sqlrmocks.NewRepositoryTx[int, queryShadowExcluded](t)
	excludedResource, err := newResource(excludedRepository, excludedSchema, nil)
	require.NoError(t, err)
	_, err = excludedResource.queryFields.resolve(JsonFilter{}, []JsonOrder{{Field: "Child"}})
	require.Error(t, err)
	explicitResource, err := newResource(excludedRepository, excludedSchema, map[string]string{"selected": "Child"})
	require.NoError(t, err)
	_, err = explicitResource.queryFields.resolve(JsonFilter{Type: "eq", Field: "SELECTED", Value: "label"}, nil)
	require.NoError(t, err)
}
