package sqlh

import (
	"context"

	"github.com/gosoline-project/sqlr"
)

// TxOperation is the transaction-aware form used internally by SQLH.
type TxOperation[I, O any] func(context.Context, sqlr.TTx, *I) (O, error)

// CrudOperation replaces one complete CRUD operation and receives SQLH's configured repository.
type CrudOperation[K sqlr.KeyTypes, E sqlr.Entitier[K], I, O any] func(
	context.Context,
	sqlr.TTx,
	sqlr.CountingRepositoryTx[K, E],
	*I,
) (O, error)
