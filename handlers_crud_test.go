package sqlh

import (
	"context"
	"errors"
	"strings"
	"testing"

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
	InputByID[int]
	Name string `json:"name"`
}

type crudTestOutput struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func TestCRUDCreateCommitsBeforeReturningTypedOutput(t *testing.T) {
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

	handler, err := newCRUD(repository, runner, schema, NewCrudDefinition(
		func(_ context.Context, input *crudTestCreateInput) (*crudTestEntity, error) {
			return &crudTestEntity{Name: input.Name}, nil
		},
		func(_ context.Context, entity *crudTestEntity, input *crudTestUpdateInput) (*crudTestEntity, error) {
			entity.Name = input.Name

			return entity, nil
		},
		func(_ context.Context, entity *crudTestEntity) (crudTestOutput, error) {
			return crudTestOutput{ID: entity.Id, Name: entity.Name}, nil
		},
	))
	require.NoError(t, err)

	output, err := handler.Create(context.Background(), &crudTestCreateInput{Name: "created"})
	require.NoError(t, err)
	require.Equal(t, crudTestOutput{ID: 7, Name: "created"}, output)
}

func TestCRUDReadAppliesForceFiltersToIdentityLookup(t *testing.T) {
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

		query, _, err := qb.ToSql()
		if err != nil {
			return nil, err
		}
		if !strings.Contains(query, "`"+schema.TableName+"`.`"+schema.PrimaryKey.Name+"`") {
			return nil, errors.New("primary-key lookup is not table qualified")
		}
		if !strings.Contains(query, "account_id") {
			return nil, errors.New("force filter missing from lookup")
		}

		return []crudTestEntity{{Entity: sqlr.Entity[int]{Id: 3}, Name: "scoped"}}, nil
	}).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	handler, err := newCRUD(repository, runner, schema, NewCrudDefinition(
		func(_ context.Context, input *crudTestCreateInput) (*crudTestEntity, error) {
			return &crudTestEntity{Name: input.Name}, nil
		},
		func(_ context.Context, entity *crudTestEntity, input *crudTestUpdateInput) (*crudTestEntity, error) {
			entity.Name = input.Name

			return entity, nil
		},
		func(_ context.Context, entity *crudTestEntity) (crudTestOutput, error) {
			return crudTestOutput{ID: entity.Id, Name: entity.Name}, nil
		},
	))
	require.NoError(t, err)

	input := &InputByID[int]{ID: 3}
	input.AddForceFilter(func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("account_id = ?", 42)
	})

	output, err := handler.Read(context.Background(), input)
	require.NoError(t, err)
	require.Equal(t, crudTestOutput{ID: 3, Name: "scoped"}, output)
}

func TestCRUDDeleteTypedUsesSoftDeleteStrategy(t *testing.T) {
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

	definition := NewCrudDefinition(
		func(_ context.Context, input *crudTestCreateInput) (*crudTestEntity, error) {
			return &crudTestEntity{Name: input.Name}, nil
		},
		func(_ context.Context, entity *crudTestEntity, input *crudTestUpdateInput) (*crudTestEntity, error) {
			entity.Name = input.Name

			return entity, nil
		},
		func(_ context.Context, entity *crudTestEntity) (crudTestOutput, error) {
			return crudTestOutput{ID: entity.Id, Name: entity.Name}, nil
		},
	)
	definition.Delete = func(_ context.Context, _ sqlr.TTx, _ sqlr.RepositoryTx[int, crudTestEntity], entity *crudTestEntity) error {
		entity.Name = "deleted"

		return nil
	}

	handler, err := newCRUD(repository, runner, schema, definition)
	require.NoError(t, err)

	output, err := handler.DeleteTyped(context.Background(), &InputByID[int]{ID: 9})
	require.NoError(t, err)
	require.Equal(t, crudTestOutput{ID: 9, Name: "deleted"}, output)
}

func newTestTx(t *testing.T) *sqlcmocks.Tx {
	t.Helper()

	return sqlcmocks.NewTx(t)
}
