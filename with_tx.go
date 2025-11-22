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

type txKey struct {}

func WithTx[H any](handlerFactory httpserver.HandlerFactory[H], register httpserver.RegisterFunc[H]) httpserver.RegisterFactoryFunc {
	return func(ctx context.Context, config cfg.Config, logger log.Logger, router *httpserver.Router) (func(router *httpserver.Router), error) {
		var err error
		var sqlClient sqlc.Client
		var handler *H

		if sqlClient, err = sqlc.ProvideClient(ctx, config, logger, "default"); err != nil {
			return nil, fmt.Errorf("could not create sqlg client: %w", err)
		}

		if handler, err = handlerFactory(ctx, config, logger); err != nil {
			return nil, fmt.Errorf("failed to create handler of type %T: %w", *new(H), err)
		}

		return func(router *httpserver.Router) {
			router = router.Group("")
			router.Use(func(ginCtx *gin.Context) {
				var err error
				var tx sqlc.Tx

				if tx, err = sqlClient.BeginTx(ginCtx.Request.Context()); err != nil {
					ginCtx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to begin transaction: %w", err))

					return
				}

				ginCtx.Set(txKey{}, tx)
				ginCtx.Next()

				if ginCtx.Errors != nil {
					return
				}

				if err = tx.Commit(); err != nil {
					ginCtx.AbortWithError(http.StatusInternalServerError, fmt.Errorf("failed to commit transaction: %w", err))

					return
				}
			})

			register(router, handler)
		}, nil
	}
}
