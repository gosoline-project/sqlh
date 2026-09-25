package sqlh

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/gosoline-project/sqlr"
	sqlrmocks "github.com/gosoline-project/sqlr/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type resourceTestEntity struct {
	sqlr.Entity[int]
	Name string `db:"name"`
}

type resourceTestUpdateInput struct {
	InputById[int]
	Name string `json:"name"`
}

type resourceTestOutput struct {
	Id   int
	Name string
}
type resourceTestStringUpdateInput struct {
	InputById[string]
}

type resourceTestListInput struct {
	ForceFilters
	Page ListPage
}

func (resourceTestListInput) ApplyFilters(*sqlr.QueryBuilderSelect) error {
	return nil
}

func (resourceTestListInput) ApplyQueryModifiers(qb *sqlr.QueryBuilderSelect) {
	qb.GroupBy("name").OrderBy("name")
}

func (i resourceTestListInput) ApplyPagination(qb *sqlr.QueryBuilderSelect) {
	if i.Page.Limit > 0 {
		qb.Limit(i.Page.Limit)
	}
	if i.Page.Offset > 0 {
		qb.Offset(i.Page.Offset)
	}
}

func (resourceTestListInput) ValidatePagination() error {
	return nil
}

func TestNewCrudHandlerRejectsMismatchedIdentityAndKeyTypes(t *testing.T) {
	repository := sqlrmocks.NewRepositoryTx[int, resourceTestEntity](t)
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)

	_, err = newCrudHandler(
		repository,
		&TxRunner{},
		schema,
		CrudDefinition[int, resourceTestEntity, string, struct{}, resourceTestStringUpdateInput, ListInput, resourceTestOutput]{},
	)
	require.EqualError(t, err, "CRUD identity lookup is required when input ID type string differs from entity key type int")
}

func TestResourceDefaultCountAppliesQueryModifiers(t *testing.T) {
	repository := sqlrmocks.NewRepositoryTx[int, resourceTestEntity](t)
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resource, err := newResource(repository, schema)
	require.NoError(t, err)

	repository.EXPECT().Query(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, options ...func(*sqlr.QueryBuilderSelect)) ([]resourceTestEntity, error) {
		qb := sqlr.NewQueryBuilderSelect()
		for _, option := range options {
			option(qb)
		}

		query, _, err := qb.ToSql()
		require.NoError(t, err)
		require.Contains(t, query, "GROUP BY `name`")
		require.Contains(t, query, "ORDER BY `name`")
		require.Contains(t, query, "LIMIT 2")
		require.Contains(t, query, "OFFSET 1")

		return []resourceTestEntity{{Entity: sqlr.Entity[int]{Id: 1}, Name: "first"}}, nil
	}).Once()
	repository.EXPECT().Count(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, qb *sqlr.QueryBuilderSelect) (int, error) {
		query, _, err := qb.ToSql()
		require.NoError(t, err)
		require.Contains(t, query, "GROUP BY `name`")
		require.Contains(t, query, "ORDER BY `name`")
		require.NotContains(t, query, "LIMIT")
		require.NotContains(t, query, "OFFSET")

		return 1, nil
	}).Once()

	output, err := resource.list(
		context.Background(),
		sqlr.TTx{},
		&resourceTestListInput{Page: ListPage{Limit: 2, Offset: 1}},
		nil,
		nil,
		nil,
		func(_ context.Context, entity *resourceTestEntity) (resourceTestOutput, error) {
			return resourceTestOutput{Id: entity.Id, Name: entity.Name}, nil
		},
	)
	require.NoError(t, err)
	require.Equal(t, ListOutput[resourceTestOutput]{
		Results: []resourceTestOutput{{Id: 1, Name: "first"}},
		Total:   1,
	}, output)
}

func TestResourcePatchEmptyDocumentReturnsCurrentEntityWithoutUpdate(t *testing.T) {
	repository := sqlrmocks.NewRepositoryTx[int, resourceTestEntity](t)
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resource, err := newResource(repository, schema)
	require.NoError(t, err)

	current := resourceTestEntity{Entity: sqlr.Entity[int]{Id: 7}, Name: "current"}
	repository.EXPECT().Query(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, options ...func(*sqlr.QueryBuilderSelect)) ([]resourceTestEntity, error) {
		return []resourceTestEntity{current}, nil
	}).Once()

	var input PatchInput[int]
	require.NoError(t, json.Unmarshal([]byte(`{}`), &input))
	input.Id = current.Id

	output, err := resource.patch(
		context.Background(),
		sqlr.TTx{},
		&input,
		nil,
		nil,
		func(context.Context, *resourceTestEntity) (*resourceTestUpdateInput, error) {
			t.Fatal("empty patch must not create an update input")

			return nil, nil
		},
		func(context.Context, *resourceTestEntity, *resourceTestUpdateInput) (*resourceTestEntity, error) {
			t.Fatal("empty patch must not transform an update")

			return nil, nil
		},
		func(_ context.Context, entity *resourceTestEntity) (resourceTestOutput, error) {
			return resourceTestOutput{Id: entity.Id, Name: entity.Name}, nil
		},
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, resourceTestOutput{Id: 7, Name: "current"}, output)
}

func TestResourceDeleteLookupDoesNotLock(t *testing.T) {
	repository := sqlrmocks.NewRepositoryTx[int, resourceTestEntity](t)
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resource, err := newResource(repository, schema)
	require.NoError(t, err)

	entity := resourceTestEntity{Entity: sqlr.Entity[int]{Id: 7}, Name: "current"}
	repository.EXPECT().Query(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, options ...func(*sqlr.QueryBuilderSelect)) ([]resourceTestEntity, error) {
		qb := sqlr.NewQueryBuilderSelect()
		for _, option := range options {
			option(qb)
		}

		query, _, err := qb.ToSql()
		require.NoError(t, err)
		require.NotContains(t, query, "FOR UPDATE")

		return []resourceTestEntity{entity}, nil
	}).Once()
	repository.EXPECT().Delete(mock.Anything, entity.Id, mock.Anything).Return(nil).Once()

	deleted, err := resource.deleteEntity(
		context.Background(),
		sqlr.TTx{},
		&InputById[int]{Id: entity.Id},
		nil,
		nil,
		nil,
	)
	require.NoError(t, err)
	require.Equal(t, &entity, deleted)
}
