package sqlh

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/gosoline-project/httpserver"
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

	output, err := handler.Delete(context.Background(), &InputById[int]{Id: 9})
	require.NoError(t, err)
	response, ok := output.(httpserver.Response)
	require.True(t, ok)
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
	require.Nil(t, output)
	require.ErrorContains(t, err, "failed to transform deleted entity: output failed")
}

func newCrudTestDefinition() CrudDefinition[int, crudTestEntity, int, crudTestCreateInput, crudTestUpdateInput, ListInput, crudTestOutput] {
	return NewCrudDefinition(
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
	)
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
