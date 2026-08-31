// Package sqlh provides typed, transaction-aware CRUD operations for SQLR
// entities and HTTP servers.
//
// SQLH operations use the same shape as the surrounding HTTP and authorization
// packages:
//
//	func(context.Context, *Input) (Output, error)
//
// [WithCrudHandlers] registers the standard create, read, update, delete, and
// list routes. [NewCRUD] and the individual methods on [CRUD] can be used when
// an application needs to compose operations with authz.Decorate or register
// routes manually. Responses are ordinary typed values; the response
// negotiator in httpserver selects the representation after SQLH commits the
// transaction.
//
// [ForceFilters] is a request-local carrier for server-owned query restrictions.
// Applications can embed it in [ListInput], [InputByID], or custom update
// inputs and add restrictions from an authorization policy before the SQLH
// operation runs. SQLH applies those restrictions to list, count, identity,
// update, and delete lookups.
//
// [CRUD.DeleteTyped] and [DeleteTypedOperation] provide a negotiated typed
// delete result for explicit soft-delete endpoints; [CRUD.Delete] retains the
// conventional 204 response. [PatchInput] and [PatchDocument] provide
// independent JSON Merge Patch handling. PATCH synchronizes only association
// paths supplied in the request and configured with `sync:update`.
//
// Association handling can be configured on relationship fields with sqlh tags
// such as `sqlh:"preload:create,read,query;sync:create,update,delete"`.
package sqlh
