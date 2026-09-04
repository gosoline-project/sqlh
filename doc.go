// Package sqlh provides typed, transaction-aware CRUD operations for SQLR
// entities and HTTP servers.
//
// # Operations and transactions
//
// Public SQLH operations use the same shape as httpserver handlers and
// authorization decorators:
//
//	func(context.Context, *Input) (Output, error)
//
// SQLH runs each operation in a database transaction. It commits the
// transaction before it returns the output and rolls back on every error. The
// HTTP server therefore renders a response only after a successful commit.
// Responses are ordinary typed values. The response negotiator in httpserver
// selects their representation.
//
// # CRUD routes and extension points
//
// [WithCrudHandlers] registers these standard routes:
//
//   - POST /v{version}/{entity}
//   - GET /v{version}/{entity}/:id
//   - PUT /v{version}/{entity}/:id
//   - PATCH /v{version}/{entity}/:id when PATCH is configured
//   - DELETE /v{version}/{entity}/:id
//   - POST /v{version}/{plural-entity}
//
// [NewCRUD] and the individual methods on [CRUD] support manual route
// registration and operation decoration. [CrudDefinition] provides mapping,
// identity, query, count, delete, and SQLR builder hooks. Its operation fields
// replace complete default operations when an endpoint needs custom behavior.
// A custom UpdateOperation does not replace the mapping used by the default
// PATCH operation. A custom PatchOperation replaces the complete default PATCH
// pipeline.
//
// # Force filters and lists
//
// [ForceFilters] carries server-owned query restrictions. Applications can
// embed it in [ListInput], [InputByID], or custom update inputs. Force filters
// are not bound from HTTP input and cannot be removed by user input. Default
// SQLH operations add them to user filters before pagination and apply them to
// list, count, identity, update, and delete lookups. A force filter must only
// add restrictive WHERE conditions.
//
// [ListInput] provides filters, nested pagination, and force filters. [ListOutput]
// contains typed results and an explicit total. Default query and count paths
// use the same user filters, force filters, and soft-delete scope. Custom Query
// and Count callbacks must call QueryPlan.ApplyScope. A custom Query callback
// must apply pagination after that scope. Applications can embed or replace the
// list input when they need domain-specific query behavior.
//
// # JSON Merge Patch
//
// [PatchInput] and [PatchDocument] implement JSON Merge Patch independently of
// PUT. The default PATCH operation uses this sequence:
//
//   - load the entity with update preloads and lookup scopes;
//   - call [CrudDefinition.PatchInputFromEntity] to build a complete update input;
//   - merge the request document into that input;
//   - call [CrudDefinition.UpdateInput] with the merged complete input;
//   - persist the entity and only the associations selected by the request;
//   - map the persisted entity to output and commit the transaction.
//
// PatchInputFromEntity must return every writable value that an omitted field
// must preserve. SQLH retains the original document separately from the merged
// input, so it can distinguish an omitted field from an explicitly supplied
// zero value or null. JSON Merge Patch replaces arrays as complete values.
// For direct association fields, null and an empty array both clear the
// association.
//
// SQLH derives direct association mappings from update-input JSON paths and
// entity relation names. Only relations configured with sqlh sync:update are
// eligible for PATCH synchronization. An omitted association is not
// synchronized merely because it exists in the complete update input.
//
// [CrudDefinition.PatchAssociationTriggers] handles indirect association
// changes. Each map key is a JSON path in the original patch document. Each map
// value is an entity relation path that has sqlh sync:update. For example:
//
//	PatchAssociationTriggers: map[string]string{
//		"status": "Children",
//	}
//
// If UpdateInput derives Children values from status, this trigger selects the
// Children relation when the request contains status. A trigger does not mutate
// the relation and does not change merge or null semantics. It only tells SQLH
// to persist relation changes made by UpdateInput. SQLH cannot infer this
// dependency because it exists in application mapping code.
//
// Association behavior is configured on relationship fields with sqlh tags,
// for example:
//
//	Children []*Child `sqlh:"preload:create,read,query,update;sync:create,update,delete"`
//
// # Identity and deletion
//
// A custom identity lookup can use a public or secondary key. It must apply the
// supplied scope so force filters and soft-delete restrictions take effect
// before SQLH returns an entity.
//
// Physical SQLR deletion is the default. Applications can provide an explicit
// soft-delete strategy. [CRUD.DeleteTyped] and [DeleteTypedOperation] return a
// negotiated typed result for such endpoints. [CRUD.Delete] retains the
// conventional 204 response. SQLH does not infer soft deletion from field
// names.
//
// # Errors and domain behavior
//
// SQLH returns errors to the HTTP server for central mapping. Applications own
// authorization, validation, status transitions, event publication, metrics,
// and other domain behavior. SQLH provides transaction, repository, query,
// identity, association, and output seams for that behavior.
package sqlh
