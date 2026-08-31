package sqlh

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

// TransactionClient is the smallest client contract needed by TxRunner. The
// production sqlc.Client satisfies it, while applications can use a narrow
// fake in unit tests.
type TransactionClient interface {
	BeginTx(context.Context, ...*sql.TxOptions) (sqlc.Tx, error)
}

// TxRunner owns the SQL client used by SQLH operations. It commits only after
// an operation has returned successfully, before the operation result is
// handed to httpserver for response rendering.
type TxRunner struct {
	client TransactionClient
}

// NewTxRunner creates a transaction runner from a named SQL client in config.
func NewTxRunner(ctx context.Context, config cfg.Config, logger log.Logger, clientName string) (*TxRunner, error) {
	client, err := sqlc.ProvideClient(ctx, config, logger, clientName)
	if err != nil {
		return nil, fmt.Errorf("failed to provide SQL client %q: %w", clientName, err)
	}

	return NewTxRunnerWithClient(client)
}

// NewTxRunnerWithClient creates a transaction runner around an existing SQL
// client. The same client should be used to construct a RepositoryTx when
// prepared statements are enabled.
func NewTxRunnerWithClient(client TransactionClient) (*TxRunner, error) {
	if client == nil {
		return nil, errors.New("SQL client is required")
	}

	return &TxRunner{client: client}, nil
}

// Client returns the SQL client used by the runner.
func (r *TxRunner) Client() TransactionClient {
	if r == nil {
		return nil
	}

	return r.client
}

// Run executes an operation in a transaction. The transaction is rolled back
// when the operation returns an error or panics, and committed otherwise.
func (r *TxRunner) Run(ctx context.Context, operation func(sqlr.TTx) error) (err error) {
	if r == nil || r.client == nil {
		return errors.New("transaction runner is not configured")
	}
	if operation == nil {
		return errors.New("transaction operation is required")
	}

	tx, err := r.client.BeginTx(ctx)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}

	commitAttempted := false
	defer func() {
		recovered := recover()
		if recovered != nil {
			if !commitAttempted {
				if rollbackErr := tx.Rollback(); rollbackErr != nil {
					panic(fmt.Errorf("failed to rollback transaction after panic %v: %w", recovered, rollbackErr))
				}
			}

			panic(recovered)
		}
		if err == nil || commitAttempted {
			return
		}

		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			err = fmt.Errorf("failed to rollback transaction: %w", errors.Join(rollbackErr, err))
		}
	}()

	ttx := sqlr.NewTx(tx)
	operationErr := operation(ttx)
	if operationErr != nil {
		err = operationErr

		return err
	}

	commitAttempted = true
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// RunValue executes a value-producing operation in a transaction and returns
// its value only after the transaction has committed.
func RunValue[I, O any](ctx context.Context, runner *TxRunner, input *I, operation TxOperation[I, O]) (output O, err error) {
	if runner == nil {
		return output, errors.New("transaction runner is required")
	}
	if operation == nil {
		return output, errors.New("transaction operation is required")
	}

	err = runner.Run(ctx, func(tx sqlr.TTx) error {
		output, err = operation(ctx, tx, input)

		return err
	})
	if err != nil {
		var zero O

		return zero, err
	}

	return output, nil
}

// InTransaction converts a transaction-aware operation to the ordinary function
// signature used by httpserver.Bind and authz.Decorate.
func InTransaction[I, O any](runner *TxRunner, operation TxOperation[I, O]) func(context.Context, *I) (O, error) {
	return func(ctx context.Context, input *I) (O, error) {
		return RunValue(ctx, runner, input, operation)
	}
}
