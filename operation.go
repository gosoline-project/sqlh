package sqlh

import (
	"context"

	"github.com/gosoline-project/sqlr"
)

// TxOperation is the transaction-aware form used to implement a public SQLH operation.
type TxOperation[I, O any] func(context.Context, sqlr.TTx, *I) (O, error)
