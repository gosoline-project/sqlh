// Package sqlh provides HTTP handler helpers that integrate SQL transactions
// with Gin-based HTTP servers.
//
// The package offers three main capabilities:
//
//  1. Transaction middleware: [WithTx] wraps a Gin router group so that every
//     request runs inside a SQL transaction. The transaction is committed on
//     success and rolled back if any handler error is recorded.
//
//  2. Handler constructors: [BindTx], [BindTxR], [BindTxN], and [BindTxNR]
//     create Gin handler functions that bind the current request's transaction
//     (placed in the context by [WithTx]) and an optional input struct, then
//     delegate to a user-supplied function.
//
//  3. Generic CRUD handlers: [WithCrudHandlers] and [HandlerCrud] wire up a
//     full set of Create/Read/Update/Delete/Query HTTP endpoints for any
//     SQL-backed entity type, using a [Transformer] to convert between HTTP
//     DTOs and database entities. Handler setup can be customized with options
//     such as [WithClientName] and [WithRepositoryFactory].
//     [JsonResultsTransformer] together with [NewJsonResultsTransformer]
//     provide a convenience implementation that renders entities as JSON
//     without needing to construct [httpserver.Response] values manually.
//     Association handling can be configured on relationship fields with
//     `sqlh` tags such as `sqlh:"preload:read,query;sync:create,update,delete"`.
package sqlh
