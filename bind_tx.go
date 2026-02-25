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

// BindTx returns a Gin handler that binds an input struct of type I from the
// current HTTP request and retrieves the active [sqlc.Tx] from the Gin context
// (set by [WithTx]). It then calls handler with the transaction and the bound
// input, writing the response back to the client.
//
// Optional binders can be supplied to override how the input is decoded.
// BindTx is a convenience wrapper around [BindTxR] that discards the raw
// *http.Request.
func BindTx[I any](handler func(tx sqlc.Tx, input *I) (httpserver.Response, error), binders ...binding.Binding) gin.HandlerFunc {
	return BindTxR[I](func(tx sqlc.Tx, _ *http.Request, input *I) (httpserver.Response, error) {
		return handler(tx, input)
	}, binders...)
}

// BindTxR returns a Gin handler that binds an input struct of type I from the
// current HTTP request and retrieves the active [sqlc.Tx] from the Gin context
// (set by [WithTx]). It then calls handler with the transaction, the raw
// *http.Request, and the bound input, writing the response back to the client.
//
// Optional binders can be supplied to override how the input is decoded.
// Use [BindTx] instead when the raw request is not needed.
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
			ginCtx.Error(fmt.Errorf("bind error: %w", err)) //nolint:errcheck // gin.Error stores the error in the context; the *gin.Error return value carries no additional information

			return
		}

		if txAny, ok = ginCtx.Get(txKey{}); !ok {
			ginCtx.Error(fmt.Errorf("could not find transaction in gin context")) //nolint:errcheck // gin.Error stores the error in the context; the *gin.Error return value carries no additional information

			return
		}

		if tx, ok = txAny.(sqlc.Tx); !ok {
			ginCtx.Error(fmt.Errorf("transaction in context is not of type sqlc.Tx")) //nolint:errcheck // gin.Error stores the error in the context; the *gin.Error return value carries no additional information

			return
		}

		if response, err = handler(tx, ginCtx.Request, input); err != nil {
			ginCtx.Error(fmt.Errorf("handler error: %w", err)) //nolint:errcheck // gin.Error stores the error in the context; the *gin.Error return value carries no additional information

			return
		}

		if err = httpserver.BindHandleResponse(response, ginCtx); err != nil {
			ginCtx.Error(fmt.Errorf("response error: %w", err)) //nolint:errcheck // gin.Error stores the error in the context; the *gin.Error return value carries no additional information
		}
	}
}

// BindTxN returns a Gin handler that retrieves the active [sqlc.Tx] from the
// Gin context (set by [WithTx]) and calls handler with it. Unlike [BindTx],
// no input struct is bound from the request.
//
// BindTxN is a convenience wrapper around [BindTxNR] that discards the raw
// *http.Request.
func BindTxN(handler func(tx sqlc.Tx) (httpserver.Response, error)) gin.HandlerFunc {
	return BindTxNR(func(tx sqlc.Tx, _ *http.Request) (httpserver.Response, error) {
		return handler(tx)
	})
}

// BindTxNR returns a Gin handler that retrieves the active [sqlc.Tx] from the
// Gin context (set by [WithTx]) and calls handler with the transaction and the
// raw *http.Request. Unlike [BindTxR], no input struct is bound from the
// request.
//
// Use [BindTxN] instead when the raw request is not needed.
func BindTxNR(handler func(tx sqlc.Tx, req *http.Request) (httpserver.Response, error)) gin.HandlerFunc {
	return func(ginCtx *gin.Context) {
		var ok bool
		var err error
		var txAny any
		var tx sqlc.Tx
		var response httpserver.Response

		if txAny, ok = ginCtx.Get(txKey{}); !ok {
			ginCtx.Error(fmt.Errorf("could not find transaction in gin context")) //nolint:errcheck // gin.Error stores the error in the context; the *gin.Error return value carries no additional information

			return
		}

		if tx, ok = txAny.(sqlc.Tx); !ok {
			ginCtx.Error(fmt.Errorf("transaction in context is not of type sqlc.Tx")) //nolint:errcheck // gin.Error stores the error in the context; the *gin.Error return value carries no additional information

			return
		}

		if response, err = handler(tx, ginCtx.Request); err != nil {
			ginCtx.Error(fmt.Errorf("handler error: %w", err)) //nolint:errcheck // gin.Error stores the error in the context; the *gin.Error return value carries no additional information

			return
		}

		if err = httpserver.BindHandleResponse(response, ginCtx); err != nil {
			ginCtx.Error(fmt.Errorf("response error: %w", err)) //nolint:errcheck // gin.Error stores the error in the context; the *gin.Error return value carries no additional information
		}
	}
}
