package sqlh

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/gosoline-project/sqlc"
	sqlcmocks "github.com/gosoline-project/sqlc/mocks"
	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/require"
)

type transactionTestTx struct {
	*sqlcmocks.Tx
}

func (transactionTestTx) Q() *sqlc.QueryBuilder {
	return nil
}

func (transactionTestTx) SQLTx() *sql.Tx {
	return nil
}

func (tx *transactionTestTx) WithContext(context.Context) sqlc.Tx {
	return tx
}

type transactionTestClient struct {
	tx       *transactionTestTx
	beginErr error
}

func (c transactionTestClient) BeginTx(context.Context, ...*sql.TxOptions) (sqlc.Tx, error) {
	if c.beginErr != nil {
		return nil, c.beginErr
	}

	return c.tx, nil
}

func TestTxRunnerCommitsAfterSuccessfulOperation(t *testing.T) {
	tx := &transactionTestTx{Tx: sqlcmocks.NewTx(t)}
	tx.EXPECT().Commit().Return(nil).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)

	called := false
	err = runner.Run(context.Background(), func(_ sqlr.TTx) error {
		called = true

		return nil
	})
	require.NoError(t, err)
	require.True(t, called)
}

func TestTxRunnerRollsBackOperationErrors(t *testing.T) {
	tx := &transactionTestTx{Tx: sqlcmocks.NewTx(t)}
	tx.EXPECT().Rollback().Return(nil).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)

	operationErr := errors.New("operation failed")
	err = runner.Run(context.Background(), func(_ sqlr.TTx) error {
		return operationErr
	})
	require.ErrorIs(t, err, operationErr)
}

func TestTxRunnerRollsBackPanics(t *testing.T) {
	tx := &transactionTestTx{Tx: sqlcmocks.NewTx(t)}
	tx.EXPECT().Rollback().Return(nil).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)

	require.Panics(t, func() {
		require.NoError(t, runner.Run(context.Background(), func(_ sqlr.TTx) error {
			panic("operation panicked")
		}))
	})
}

func TestTxRunnerReturnsCommitErrors(t *testing.T) {
	tx := &transactionTestTx{Tx: sqlcmocks.NewTx(t)}
	commitErr := errors.New("commit failed")
	tx.EXPECT().Commit().Return(commitErr).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)

	err = runner.Run(context.Background(), func(_ sqlr.TTx) error {
		return nil
	})
	require.ErrorIs(t, err, commitErr)
}
