package sqlh

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gosoline-project/sqlc"
	sqlcmocks "github.com/gosoline-project/sqlc/mocks"
	"github.com/gosoline-project/sqlr"
	sqlrmocks "github.com/gosoline-project/sqlr/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type crudTestEntity struct {
	sqlr.Entity[int]
	Name string `db:"name"`
}

type crudTestCreateInput struct {
	Name string `json:"name"`
}

type crudTestUpdateInput struct {
	InputById[int]
	Name string `json:"name"`
}

type crudTestOutput struct {
	Id   int    `json:"id"`
	Name string `json:"name"`
}

type crudTestListInput struct {
	ForceFilters
	applyFiltersCalls     *int
	applyFiltersErr       error
	validatePaginationErr error
}

func (i crudTestListInput) ApplyFilters(qb *sqlr.QueryBuilderSelect) error {
	if i.applyFiltersCalls != nil {
		(*i.applyFiltersCalls)++
	}
	if i.applyFiltersErr != nil {
		return i.applyFiltersErr
	}
	qb.Where("domain = ?", "allowed")

	return nil
}

func (crudTestListInput) ApplyPagination(*sqlr.QueryBuilderSelect) {}

func (i crudTestListInput) ValidatePagination() error {
	return i.validatePaginationErr
}

func TestCrudHandlerListCustomQueryAndCountApplySharedPlan(t *testing.T) {
	repository := sqlrmocks.NewCountingRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: newTestTx(t)}
	tx.EXPECT().Commit().Return(nil).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)
	definition := newCrudTestDefinition()
	definition.DeleteScope = func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("deleted = ?", false)
	}

	assertPlan := func(qb *sqlr.QueryBuilderSelect, paginated bool) {
		query, arguments, queryErr := qb.ToSql()
		require.NoError(t, queryErr)
		require.Contains(t, query, "deleted")
		require.Contains(t, query, "account_id")
		require.Contains(t, query, "status")
		require.Equal(t, []any{false, 42, "active"}, arguments)
		if paginated {
			require.Contains(t, query, "LIMIT 2")
			require.Contains(t, query, "OFFSET 1")

			return
		}
		require.NotContains(t, query, "LIMIT")
		require.NotContains(t, query, "OFFSET")
	}
	definition.Query = func(_ context.Context, _ sqlr.TTx, _ sqlr.RepositoryTx[int, crudTestEntity], _ *ListInput, plan QueryPlan) ([]crudTestEntity, error) {
		require.NotNil(t, plan.ApplyBuilder)
		qb := sqlr.NewQueryBuilderSelect()
		plan.ApplyBuilder(qb)
		require.NoError(t, plan.ApplyScope(qb))
		plan.ApplyPagination(qb)
		assertPlan(qb, true)

		return []crudTestEntity{{Entity: sqlr.Entity[int]{Id: 1}, Name: "first"}}, nil
	}
	definition.Count = func(_ context.Context, _ sqlr.TTx, _ sqlr.RepositoryTx[int, crudTestEntity], _ *ListInput, plan QueryPlan) (int, error) {
		require.NotNil(t, plan.ApplyBuilder)
		qb := sqlr.NewQueryBuilderSelect()
		plan.ApplyBuilder(qb)
		require.NoError(t, plan.ApplyScope(qb))
		assertPlan(qb, false)

		return 3, nil
	}

	handler, err := newCrudHandler(repository, runner, schema, definition)
	require.NoError(t, err)

	input := ListInput{
		Filter: sqlc.JsonFilter{Type: "eq", Column: "status", Value: "active"},
		Page:   ListPage{Limit: 2, Offset: 1},
	}
	input.AddForceFilter(func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("account_id = ?", 42)
	})

	output, err := handler.List(context.Background(), &input)
	require.NoError(t, err)
	require.Equal(t, ListOutput[crudTestOutput]{
		Results: []crudTestOutput{{Id: 1, Name: "first"}},
		Total:   3,
	}, output)
}

func TestCrudHandlerListPaginationErrorRollsBack(t *testing.T) {
	repository := sqlrmocks.NewCountingRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: newTestTx(t)}
	tx.EXPECT().Rollback().Return(nil).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)
	handler, err := newCrudHandler(repository, runner, schema, newCrudTestDefinitionWithList[crudTestListInput]())
	require.NoError(t, err)

	paginationErr := errors.New("pagination failed")
	applyFiltersCalls := 0
	output, err := handler.List(context.Background(), &crudTestListInput{
		applyFiltersCalls:     &applyFiltersCalls,
		validatePaginationErr: paginationErr,
	})
	require.Zero(t, output)
	require.ErrorIs(t, err, paginationErr)
	require.EqualError(t, err, paginationErr.Error())
	require.Zero(t, applyFiltersCalls)
}

func TestCrudHandlerListApplyFiltersErrorRollsBack(t *testing.T) {
	repository := sqlrmocks.NewCountingRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: newTestTx(t)}
	tx.EXPECT().Rollback().Return(nil).Once()

	queryCalls := 0
	repository.EXPECT().Query(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, options ...func(*sqlr.QueryBuilderSelect)) ([]crudTestEntity, error) {
		queryCalls++
		qb := sqlr.NewQueryBuilderSelect()
		for _, option := range options {
			option(qb)
		}

		return nil, nil
	}).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)
	handler, err := newCrudHandler(repository, runner, schema, newCrudTestDefinitionWithList[crudTestListInput]())
	require.NoError(t, err)

	filtersErr := errors.New("filters failed")
	applyFiltersCalls := 0
	output, err := handler.List(context.Background(), &crudTestListInput{
		applyFiltersCalls: &applyFiltersCalls,
		applyFiltersErr:   filtersErr,
	})
	require.Zero(t, output)
	require.ErrorIs(t, err, filtersErr)
	require.EqualError(t, err, filtersErr.Error())
	require.Equal(t, 1, queryCalls)
	require.Equal(t, 1, applyFiltersCalls)
}

func TestCrudHandlerCreateCommitsBeforeReturningTypedOutput(t *testing.T) {
	repository := sqlrmocks.NewCountingRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: newTestTx(t)}
	tx.EXPECT().Commit().Return(nil).Once()

	repository.EXPECT().Create(mock.Anything, mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, entity *crudTestEntity, _ ...func(*sqlr.QueryBuilderCreate)) error {
		entity.Id = 7

		return nil
	}).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)

	handler, err := newCrudHandler(repository, runner, schema, NewCrudDefinition(
		func(_ context.Context, input *crudTestCreateInput) (*crudTestEntity, error) {
			return &crudTestEntity{Name: input.Name}, nil
		},
		func(_ context.Context, entity *crudTestEntity, input *crudTestUpdateInput) (*crudTestEntity, error) {
			entity.Name = input.Name

			return entity, nil
		},
		crudTestPatchInputFromEntity,
		func(_ context.Context, entity *crudTestEntity) (crudTestOutput, error) {
			return crudTestOutput{Id: entity.Id, Name: entity.Name}, nil
		},
	))
	require.NoError(t, err)

	output, err := handler.Create(context.Background(), &crudTestCreateInput{Name: "created"})
	require.NoError(t, err)
	require.Equal(t, crudTestOutput{Id: 7, Name: "created"}, output)
}

func TestCrudHandlerReadAppliesForceFiltersToIdentityLookup(t *testing.T) {
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)

	repository := sqlrmocks.NewCountingRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: newTestTx(t)}
	tx.EXPECT().Commit().Return(nil).Once()

	repository.EXPECT().Query(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, opts ...func(*sqlr.QueryBuilderSelect)) ([]crudTestEntity, error) {
		qb := sqlr.NewQueryBuilderSelect()
		for _, option := range opts {
			option(qb)
		}

		query, arguments, err := qb.ToSql()
		if err != nil {
			return nil, err
		}
		if !strings.Contains(query, "`"+schema.TableName+"`.`"+schema.PrimaryKey.Name+"`") {
			return nil, errors.New("primary-key lookup is not table qualified")
		}
		if !strings.Contains(query, "account_id") {
			return nil, errors.New("force filter missing from lookup")
		}
		require.Equal(t, []any{3, 42}, arguments)

		return []crudTestEntity{{Entity: sqlr.Entity[int]{Id: 3}, Name: "scoped"}}, nil
	}).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	definition := newCrudTestDefinition()

	handler, err := newCrudHandler(repository, runner, schema, definition)
	require.NoError(t, err)

	input := &InputById[int]{Id: 3}
	input.AddForceFilter(func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("account_id = ?", 42)
	})

	output, err := handler.Read(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, crudTestOutput{Id: 3, Name: "scoped"}, output)
}

func TestCrudHandlerListAppliesScopesAndPaginationToDefaultQueryAndCount(t *testing.T) {
	repository := sqlrmocks.NewCountingRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: newTestTx(t)}
	tx.EXPECT().Commit().Return(nil).Once()

	repository.EXPECT().Query(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, options ...func(*sqlr.QueryBuilderSelect)) ([]crudTestEntity, error) {
		qb := sqlr.NewQueryBuilderSelect()
		for _, option := range options {
			option(qb)
		}

		query, arguments, err := qb.ToSql()
		if err != nil {
			return nil, err
		}
		require.Contains(t, query, "deleted")
		require.Contains(t, query, "account_id")
		require.Contains(t, query, "status")
		require.Contains(t, query, "LIMIT 2")
		require.Contains(t, query, "OFFSET 1")
		require.Equal(t, []any{false, 42, "active"}, arguments)

		return []crudTestEntity{{Entity: sqlr.Entity[int]{Id: 1}, Name: "first"}}, nil
	}).Once()
	repository.EXPECT().Count(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, qb *sqlr.QueryBuilderSelect) (int, error) {
		query, arguments, err := qb.ToSql()
		if err != nil {
			return 0, err
		}
		require.Contains(t, query, "deleted")
		require.Contains(t, query, "account_id")
		require.Contains(t, query, "status")
		require.NotContains(t, query, "LIMIT")
		require.NotContains(t, query, "OFFSET")
		require.Equal(t, []any{false, 42, "active"}, arguments)

		return 3, nil
	}).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)
	definition := newCrudTestDefinition()
	definition.DeleteScope = func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("deleted = ?", false)
	}

	handler, err := newCrudHandler(repository, runner, schema, definition)
	require.NoError(t, err)

	input := ListInput{
		Filter: sqlc.JsonFilter{Type: "eq", Column: "status", Value: "active"},
		Page:   ListPage{Limit: 2, Offset: 1},
	}
	input.AddForceFilter(func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("account_id = ?", 42)
	})

	output, err := handler.List(context.Background(), &input)
	require.NoError(t, err)
	require.Equal(t, ListOutput[crudTestOutput]{
		Results: []crudTestOutput{{Id: 1, Name: "first"}},
		Total:   3,
	}, output)
}

func TestCrudHandlerListAppliesForceFiltersToCustomInputOncePerScope(t *testing.T) {
	repository := sqlrmocks.NewCountingRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: newTestTx(t)}
	tx.EXPECT().Commit().Return(nil).Once()

	assertScope := func(qb *sqlr.QueryBuilderSelect) error {
		query, arguments, err := qb.ToSql()
		if err != nil {
			return err
		}
		require.Contains(t, query, "account_id")
		require.Contains(t, query, "domain")
		require.Equal(t, []any{7, "allowed"}, arguments)

		return nil
	}
	repository.EXPECT().Query(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, options ...func(*sqlr.QueryBuilderSelect)) ([]crudTestEntity, error) {
		qb := sqlr.NewQueryBuilderSelect()
		for _, option := range options {
			option(qb)
		}

		return nil, assertScope(qb)
	}).Once()
	repository.EXPECT().Count(mock.Anything, mock.Anything).RunAndReturn(func(_ sqlr.TTx, qb *sqlr.QueryBuilderSelect) (int, error) {
		return 0, assertScope(qb)
	}).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)
	definition := newCrudTestDefinitionWithList[crudTestListInput]()

	handler, err := newCrudHandler(repository, runner, schema, definition)
	require.NoError(t, err)

	applyFiltersCalls := 0
	input := crudTestListInput{applyFiltersCalls: &applyFiltersCalls}
	input.AddForceFilter(func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("account_id = ?", 7)
	})

	output, err := handler.List(context.Background(), &input)
	require.NoError(t, err)
	require.Empty(t, output.Results)
	require.Zero(t, output.Total)
	require.Equal(t, 2, applyFiltersCalls)
}

func TestCrudHandlerDeleteReturnsNoContentWithoutOutput(t *testing.T) {
	repository := sqlrmocks.NewCountingRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: newTestTx(t)}
	tx.EXPECT().Commit().Return(nil).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)

	definition := newCrudTestDefinition()
	definition.DeleteOperation = func(context.Context, sqlr.TTx, *InputById[int]) (*crudTestEntity, error) {
		return nil, nil
	}

	handler, err := newCrudHandler(repository, runner, schema, definition)
	require.NoError(t, err)

	response, err := handler.DeleteNoContent(context.Background(), &InputById[int]{Id: 9})
	require.NoError(t, err)
	require.Equal(t, http.StatusNoContent, response.StatusCode())
}

func TestCrudHandlerDeleteUsesConfiguredOutput(t *testing.T) {
	repository := sqlrmocks.NewCountingRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: newTestTx(t)}
	tx.EXPECT().Commit().Return(nil).Once()

	repository.EXPECT().Query(mock.Anything, mock.Anything).Return([]crudTestEntity{{
		Entity: sqlr.Entity[int]{Id: 9},
		Name:   "visible",
	}}, nil).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)

	definition := newCrudTestDefinition()
	definition.Delete = func(_ context.Context, _ sqlr.TTx, _ sqlr.RepositoryTx[int, crudTestEntity], entity *crudTestEntity) error {
		entity.Name = "deleted"

		return nil
	}
	definition.DeleteOutput = definition.Output

	handler, err := newCrudHandler(repository, runner, schema, definition)
	require.NoError(t, err)

	output, err := handler.Delete(context.Background(), &InputById[int]{Id: 9})
	require.NoError(t, err)
	require.Equal(t, crudTestOutput{Id: 9, Name: "deleted"}, output)
}

func TestCrudHandlerDeleteOperationUsesConfiguredOutput(t *testing.T) {
	repository := sqlrmocks.NewCountingRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: newTestTx(t)}
	tx.EXPECT().Commit().Return(nil).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)

	definition := newCrudTestDefinition()
	definition.DeleteOperation = func(_ context.Context, _ sqlr.TTx, input *InputById[int]) (*crudTestEntity, error) {
		require.Equal(t, 11, input.Id)

		return &crudTestEntity{Entity: sqlr.Entity[int]{Id: input.Id}, Name: "custom"}, nil
	}
	definition.DeleteOutput = definition.Output

	handler, err := newCrudHandler(repository, runner, schema, definition)
	require.NoError(t, err)

	output, err := handler.Delete(context.Background(), &InputById[int]{Id: 11})
	require.NoError(t, err)
	require.Equal(t, crudTestOutput{Id: 11, Name: "custom"}, output)
}

func TestCrudHandlerDeleteOutputErrorRollsBack(t *testing.T) {
	repository := sqlrmocks.NewCountingRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: newTestTx(t)}
	tx.EXPECT().Rollback().Return(nil).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)

	definition := newCrudTestDefinition()
	definition.DeleteOperation = func(_ context.Context, _ sqlr.TTx, input *InputById[int]) (*crudTestEntity, error) {
		return &crudTestEntity{Entity: sqlr.Entity[int]{Id: input.Id}}, nil
	}
	definition.DeleteOutput = func(context.Context, *crudTestEntity) (crudTestOutput, error) {
		return crudTestOutput{}, errors.New("output failed")
	}

	handler, err := newCrudHandler(repository, runner, schema, definition)
	require.NoError(t, err)

	output, err := handler.Delete(context.Background(), &InputById[int]{Id: 12})
	require.Zero(t, output)
	require.ErrorContains(t, err, "failed to transform deleted entity: output failed")
}

func newCrudTestDefinition() CrudDefinition[int, crudTestEntity, int, crudTestCreateInput, crudTestUpdateInput, ListInput, crudTestOutput] {
	return newCrudTestDefinitionWithList[ListInput]()
}

func newCrudTestDefinitionWithList[LI ListInputSource]() CrudDefinition[int, crudTestEntity, int, crudTestCreateInput, crudTestUpdateInput, LI, crudTestOutput] {
	return CrudDefinition[int, crudTestEntity, int, crudTestCreateInput, crudTestUpdateInput, LI, crudTestOutput]{
		CreateInput: func(_ context.Context, input *crudTestCreateInput) (*crudTestEntity, error) {
			return &crudTestEntity{Name: input.Name}, nil
		},
		UpdateInput: func(_ context.Context, entity *crudTestEntity, input *crudTestUpdateInput) (*crudTestEntity, error) {
			entity.Name = input.Name

			return entity, nil
		},
		PatchInputFromEntity: crudTestPatchInputFromEntity,
		Output: func(_ context.Context, entity *crudTestEntity) (crudTestOutput, error) {
			return crudTestOutput{Id: entity.Id, Name: entity.Name}, nil
		},
	}
}

func crudTestPatchInputFromEntity(_ context.Context, entity *crudTestEntity) (*crudTestUpdateInput, error) {
	return &crudTestUpdateInput{
		InputById: InputById[int]{Id: entity.Id},
		Name:      entity.Name,
	}, nil
}

func newTestTx(t *testing.T) *sqlcmocks.Tx {
	t.Helper()

	return sqlcmocks.NewTx(t)
}
