package sqlh

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"
	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlc"
	"github.com/justtrackio/gosoline/pkg/refl"
)

func BindTx[I any](handler func(tx sqlc.Tx, input *I) (httpserver.Response, error), binders ...binding.Binding) gin.HandlerFunc {
	return BindTxR[I](func(tx sqlc.Tx, _ *http.Request, input *I) (httpserver.Response, error) {
		return handler(tx, input)
	}, binders...)
}

func BindTxR[I any](handler func(tx sqlc.Tx, req *http.Request, input *I) (httpserver.Response, error), binders ...binding.Binding) gin.HandlerFunc {
	tags := refl.GetTagNames(new(I))

	return func(ginCtx *gin.Context) {
		var ok bool
		var err error
		var input *I
		var txAny any
		var tx sqlc.Tx
		var response httpserver.Response

		if input, err = httpserver.BindHandleRequest[I](ginCtx, tags, binders); err != nil {
			ginCtx.Error(fmt.Errorf("bind error: %w", err))

			return
		}

		if txAny, ok = ginCtx.Get(txKey{}); !ok {
			ginCtx.Error(fmt.Errorf("could not find transaction in gin context"))

			return
		}

		if tx, ok = txAny.(sqlc.Tx); !ok {
			ginCtx.Error(fmt.Errorf("transaction in context is not of type sqlc.Tx"))

			return
		}

		if response, err = handler(tx, ginCtx.Request, input); err != nil {
			ginCtx.Error(fmt.Errorf("handler error: %w", err))

			return
		}

		if err = httpserver.BindHandleResponse(response, ginCtx); err != nil {
			ginCtx.Error(fmt.Errorf("response error: %w", err))
		}
	}
}

func BindTxN(handler func(tx sqlc.Tx) (httpserver.Response, error)) gin.HandlerFunc {
	return BindTxNR(func(tx sqlc.Tx, _ *http.Request) (httpserver.Response, error) {
		return handler(tx)
	})
}

func BindTxNR(handler func(tx sqlc.Tx, req *http.Request) (httpserver.Response, error)) gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		var ok bool
		var err error
		var txAny any
		var tx sqlc.Tx
		var response httpserver.Response

		if txAny, ok = ginCtx.Get(txKey{}); !ok {
			ginCtx.Error(fmt.Errorf("could not find transaction in gin context"))

			return
		}

		if tx, ok = txAny.(sqlc.Tx); !ok {
			ginCtx.Error(fmt.Errorf("transaction in context is not of type sqlc.Tx"))

			return
		}

		if response, err = handler(tx, ginCtx.Request); err != nil {
			ginCtx.Error(fmt.Errorf("handler error: %w", err))

			return
		}

		if err = httpserver.BindHandleResponse(response, ginCtx); err != nil {
			ginCtx.Error(fmt.Errorf("response error: %w", err))
		}
	}
}
