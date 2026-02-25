package sqlh

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlc"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

type txKey struct{}

// WithTx wraps a handler factory with a SQL transaction middleware.
//
// It builds the handler H using handlerFactory, then registers it via
// register inside a new Gin router group. A middleware is attached to that
// group which, for every incoming request:
//   - Begins a new transaction using the "default" SQL client.
//   - Stores the transaction in the Gin context (retrievable via [BindTx] and
//     related helpers).
//   - Calls the next handler.
//   - Rolls back the transaction if any errors were recorded on the context,
//     or commits it otherwise.
//
// The returned [httpserver.RegisterFactoryFunc] is suitable for passing
// directly to an httpserver setup function.
func WithTx[H any](handlerFactory httpserver.HandlerFactory[H], register httpserver.RegisterFunc[H]) httpserver.RegisterFactoryFunc {
	return func(ctx context.Context, config cfg.Config, logger log.Logger, router *httpserver.Router) (func(router *httpserver.Router), error) {
		var err error
		var sqlClient sqlc.Client
		var handler *H

		if sqlClient, err = sqlc.ProvideClient(ctx, config, logger, "default"); err != nil {
			return nil, fmt.Errorf("could not create sqlc client: %w", err)
		}

		if handler, err = handlerFactory(ctx, config, logger); err != nil {
			return nil, fmt.Errorf("failed to create handler of type %T: %w", *new(H), err)
		}

		return func(router *httpserver.Router) {
			router = router.Group("")
			router.Use(txMiddleware(sqlClient))

			register(router, handler)
		}, nil
	}
}

func txMiddleware(sqlClient sqlc.Client) gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		tx, err := sqlClient.BeginTx(ginCtx.Request.Context())
		if err != nil {
			ginCtx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to begin transaction: %w", err)) //nolint:errcheck // return value is the same error stored in the context

			return
		}

		ginCtx.Set(txKey{}, tx)
		ginCtx.Next()

		if len(ginCtx.Errors) > 0 {
			if rollbackErr := tx.Rollback(); rollbackErr != nil {
				ginCtx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to rollback transaction: %w", rollbackErr)) //nolint:errcheck // return value is the same error stored in the context
			}

			return
		}

		if err = tx.Commit(); err != nil {
			ginCtx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to commit transaction: %w", err)) //nolint:errcheck // return value is the same error stored in the context
		}
	}
}
