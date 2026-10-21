package sqlh

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	sqlrmocks "github.com/gosoline-project/sqlr/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type resourceTestEntity struct {
	sqlr.Entity[int]
	Name string `db:"name" json:"name"`
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

type resourceTestValidatedListInput struct {
	ListInput
	validationErr error
}

func (i resourceTestValidatedListInput) ValidateQueryModifiers() error {
	return i.validationErr
}

func TestResourceValidatesCustomInputAndPointerInput(t *testing.T) {
	repository := sqlrmocks.NewRepositoryTx[int, resourceTestEntity](t)
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resource, err := newResource(repository, schema, nil)
	require.NoError(t, err)
	validationErr := errors.New("column is not sortable")
	input := resourceTestValidatedListInput{validationErr: validationErr}
	mapper := func(context.Context, *resourceTestEntity) (resourceTestOutput, error) {
		t.Fatal("invalid input must not reach output mapping")

		return resourceTestOutput{}, nil
	}
	output, err := resource.list(context.Background(), sqlr.TTx{}, &input, nil, nil, nil, mapper)
	require.ErrorIs(t, err, validationErr)
	require.Zero(t, output)
	inputPointer := &input
	output, err = resource.list(context.Background(), sqlr.TTx{}, &inputPointer, nil, nil, nil, mapper)
	require.ErrorIs(t, err, validationErr)
	require.Zero(t, output)
}

type resourceTestListInput struct {
	ForceFilters
	Page ListPage
}

func (resourceTestListInput) GetFilter() JsonFilter {
	return JsonFilter{}
}

func (resourceTestListInput) GetOrderBy() []JsonOrder {
	return nil
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

func (resourceTestListInput) ValidateQueryModifiers() error {
	return nil
}

type resourceTestPointerListInput struct {
	ListInput
	filterCalls   int
	modifierCalls int
}

func (i *resourceTestPointerListInput) ApplyFilters(qb *sqlr.QueryBuilderSelect) error {
	i.filterCalls++
	qb.Where("account_id = ?", 42)

	return nil
}

func (i *resourceTestPointerListInput) ApplyQueryModifiers(qb *sqlr.QueryBuilderSelect) {
	i.modifierCalls++
	qb.OrderBy("name")
}

func TestResourceListAppliesPointerReceiverTrustedPhases(t *testing.T) {
	repository := sqlrmocks.NewRepositoryTx[int, resourceTestEntity](t)
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resource, err := newResource(repository, schema, nil)
	require.NoError(t, err)
	input := resourceTestPointerListInput{
		ListInput: ListInput{Page: ListPage{Limit: 2, Offset: 1}},
	}
	repository.EXPECT().Query(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, options ...func(*sqlr.QueryBuilderSelect)) ([]resourceTestEntity, error) {
		qb := sqlr.NewQueryBuilderSelect()
		for _, option := range options {
			option(qb)
		}

		query, params, err := qb.ToSql()
		require.NoError(t, err)
		require.Contains(t, query, "account_id = ?")
		require.Contains(t, query, "ORDER BY `name`")
		require.Contains(t, query, "LIMIT 2")
		require.Contains(t, query, "OFFSET 1")
		require.Equal(t, []any{42}, params)

		return []resourceTestEntity{{Entity: sqlr.Entity[int]{Id: 1}, Name: "first"}}, nil
	}).Once()
	repository.EXPECT().Count(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, qb *sqlr.QueryBuilderSelect) (int, error) {
		query, params, err := qb.ToSql()
		require.NoError(t, err)
		require.Contains(t, query, "account_id = ?")
		require.NotContains(t, query, "ORDER BY")
		require.NotContains(t, query, "LIMIT")
		require.NotContains(t, query, "OFFSET")
		require.Equal(t, []any{42}, params)

		return 3, nil
	}).Once()

	output, err := resource.list(context.Background(), sqlr.TTx{}, &input, nil, nil, nil,
		func(_ context.Context, entity *resourceTestEntity) (resourceTestOutput, error) {
			return resourceTestOutput{Id: entity.Id, Name: entity.Name}, nil
		})
	require.NoError(t, err)
	require.Equal(t, ListOutput[resourceTestOutput]{
		Results: []resourceTestOutput{{Id: 1, Name: "first"}},
		Total:   3,
	}, output)
	require.Equal(t, 2, input.filterCalls)
	require.Equal(t, 1, input.modifierCalls)
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

func TestResourceDefaultCountOmitsRowModifiers(t *testing.T) {
	repository := sqlrmocks.NewRepositoryTx[int, resourceTestEntity](t)
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resource, err := newResource(repository, schema, nil)
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
		require.NotContains(t, query, "GROUP BY")
		require.NotContains(t, query, "ORDER BY")
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

func TestResourcePublicOrderingWithDefaultAndCustomCallbacks(t *testing.T) {
	for _, custom := range []bool{false, true} {
		t.Run(fmt.Sprintf("custom=%t", custom), func(t *testing.T) {
			repository := sqlrmocks.NewRepositoryTx[int, resourceTestEntity](t)
			schema, err := sqlr.ParseSchema[resourceTestEntity]()
			require.NoError(t, err)
			resource, err := newResource(repository, schema, map[string]string{"id": "Id", "name": "Name"})
			require.NoError(t, err)
			input := ListInput{
				Filter:  JsonFilter{Type: sqlc.JsonFilterEq, Field: "name", Value: "first"},
				OrderBy: []JsonOrder{{Field: "name", Direction: "desc"}, {Field: "id"}},
				Page:    ListPage{Limit: 2, Offset: 1},
			}
			repository.EXPECT().Query(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, options ...func(*sqlr.QueryBuilderSelect)) ([]resourceTestEntity, error) {
				qb := sqlr.NewQueryBuilderSelect()
				require.Len(t, options, 1)
				options[0](qb)
				query, params, err := qb.ToSql()
				require.NoError(t, err)
				require.Equal(t, "WHERE `"+schema.TableName+"`.`name` = ? ORDER BY `"+schema.TableName+"`.`name` DESC, `"+schema.TableName+"`.`id` ASC LIMIT 2 OFFSET 1", query)
				require.Equal(t, []any{"first"}, params)

				return []resourceTestEntity{{Entity: sqlr.Entity[int]{Id: 1}, Name: "first"}}, nil
			}).Once()
			repository.EXPECT().Count(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, qb *sqlr.QueryBuilderSelect) (int, error) {
				query, params, err := qb.ToSql()
				require.NoError(t, err)
				require.Equal(t, "WHERE `"+schema.TableName+"`.`name` = ?", query)
				require.Equal(t, []any{"first"}, params)

				return 3, nil
			}).Once()
			var query ListQuery[int, resourceTestEntity, ListInput]
			var count ListCount[int, resourceTestEntity, ListInput]
			if custom {
				query = func(_ context.Context, tx sqlr.TTx, repo sqlr.RepositoryTx[int, resourceTestEntity], _ *ListInput, plan QueryPlan) ([]resourceTestEntity, error) {
					qb := sqlr.NewQueryBuilderSelect()
					plan.ApplyBuilder(qb)
					if err := plan.ApplyScope(qb); err != nil {
						return nil, err
					}
					plan.ApplyQueryModifiers(qb)
					plan.ApplyPagination(qb)

					return repo.Query(tx, func(target *sqlr.QueryBuilderSelect) { *target = *qb })
				}
				count = func(_ context.Context, tx sqlr.TTx, repo sqlr.RepositoryTx[int, resourceTestEntity], _ *ListInput, plan QueryPlan) (int, error) {
					qb := sqlr.NewQueryBuilderSelect()
					plan.ApplyBuilder(qb)
					if err := plan.ApplyScope(qb); err != nil {
						return 0, err
					}

					return repo.Count(tx, qb)
				}
			}
			output, err := resource.list(context.Background(), sqlr.TTx{}, &input, nil, query, count,
				func(_ context.Context, entity *resourceTestEntity) (resourceTestOutput, error) {
					return resourceTestOutput{Id: entity.Id, Name: entity.Name}, nil
				})
			require.NoError(t, err)
			require.Equal(t, ListOutput[resourceTestOutput]{Results: []resourceTestOutput{{Id: 1, Name: "first"}}, Total: 3}, output)
		})
	}
}

func TestResourcePatchEmptyDocumentReturnsCurrentEntityWithoutUpdate(t *testing.T) {
	repository := sqlrmocks.NewRepositoryTx[int, resourceTestEntity](t)
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resource, err := newResource(repository, schema, nil)
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

func TestResourceDeleteLookupLocksBeforeDelete(t *testing.T) {
	repository := sqlrmocks.NewRepositoryTx[int, resourceTestEntity](t)
	schema, err := sqlr.ParseSchema[resourceTestEntity]()
	require.NoError(t, err)
	resource, err := newResource(repository, schema, nil)
	require.NoError(t, err)

	entity := resourceTestEntity{Entity: sqlr.Entity[int]{Id: 7}, Name: "current"}
	repository.EXPECT().Query(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, options ...func(*sqlr.QueryBuilderSelect)) ([]resourceTestEntity, error) {
		qb := sqlr.NewQueryBuilderSelect()
		for _, option := range options {
			option(qb)
		}

		query, _, err := qb.ToSql()
		require.NoError(t, err)
		require.Contains(t, query, "FOR UPDATE")

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
