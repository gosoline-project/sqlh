# SQLH CRUD redesign plan

## Scope

This work is an intentional pre-v1 breaking redesign of `github.com/gosoline-project/sqlh`.
It targets the split `github.com/gosoline-project/httpserver` implementation from the
`feature/response-negotiation` branch and SQLR `v0.8.2`.

The redesign implements:

- typed operations compatible with `httpserver.Bind[I, O]`;
- transaction-aware operations backed by `sqlr.RepositoryTx`;
- a configurable SQL client;
- a generic list input with built-in query fields and force filters;
- public/secondary identity lookup;
- scoped list and count operations;
- customizable delete behavior, including a typed delete operation for
  soft-delete responses and an explicit 204 response escape hatch;
- typed output that participates in response negotiation;
- a convenience CRUD route registration function with optional PATCH;
- operation-level extension points for domain behavior;
- an independent JSON Merge Patch operation with request-aware association synchronization.

The package must remain independent of `github.com/gosoline-project/authz`.
Authz decorates the SQLH operations externally. The application-level authz policy
can add SQLH force filters to a request input before the decorated operation runs.

## Architecture

The public operation shape is:

```go
func(context.Context, *Input) (Output, error)
```

This is the same shape expected by `authz.Decorate` and `httpserver.Bind`.

SQLH internally supports transaction-aware operations:

```go
type TxOperation[I, O any] func(context.Context, sqlr.TTx, *I) (O, error)
```

A transaction wrapper begins a transaction, creates `sqlr.NewTx(tx)`, executes the
operation, commits before returning the output, and rolls back on every error.
The HTTP response is therefore rendered only after a successful commit.

## Force filters

SQLH provides a request-local `ForceFilters` carrier that can be embedded in list
or identity inputs. Force filters are server-owned query restrictions and are not
bound from HTTP input.

An application authz policy can use its `Before` hook to add a force filter while
also returning the corresponding authorization check. Default SQLH operations
apply the filters as mandatory predicates to list, count, identity lookup, update
lookup, and delete lookup. Custom query and count callbacks must apply the
provided `QueryPlan.ApplyScope` function.

Force filters:

- are applied in addition to user filters;
- cannot be removed by user input;
- are applied before pagination;
- are reused by query and count;
- must only add restrictive `WHERE` conditions.

For list endpoints with a meaningful total, use a parent/account authorization
check plus a force filter and Enforce mode. Do not rely on authz post-result Filter
mode for these endpoints because the current authz decorator filters result slices
without updating a `Total` field.

## CRUD API

SQLH exposes independent typed operation builders for create, read, list, update,
and delete. A convenience registration function composes them and registers the
standard routes. Individual operations remain usable when an endpoint needs
custom domain behavior.

The standard convenience routes are:

- `POST /v{version}/{entity}`
- `GET /v{version}/{entity}/:id`
- `PUT /v{version}/{entity}/:id`
- `PATCH /v{version}/{entity}/:id` when PATCH is configured
- `DELETE /v{version}/{entity}/:id`
- `POST /v{version}/{plural-entity}`

The convenience function must accept operation-specific configuration for:

- create and update mapping;
- identity resolution;
- list query and count;
- output mapping;
- delete strategy;
- SQLR relation builders;
- SQL client name.

The generic defaults use SQLR primary-key operations. Services can replace the
identity, list, count, and delete behavior without replacing the whole handler.

PATCH uses `PatchBaseline` to create a complete update input, merges the JSON
Merge Patch document into it, and passes the result to `UpdateInput`. It derives
direct association paths from update-input JSON tags and update-sync relation
names. Applications can configure `PatchAssociationTriggers` when scalar fields
cause `UpdateInput` to derive relation values. Trigger keys are JSON paths from
the original patch document. Trigger values are entity relations configured with
`sync:update`. Triggers select relation synchronization but do not mutate values
or change merge-patch null handling.

## Generic list input

SQLH provides a reusable list input containing predefined filter, nested page,
and force-filter fields. It intentionally does not add order/order-by or
grouping fields until a concrete consumer requires their semantics; applications
can embed or replace the input with domain-specific query behavior while
retaining the force-filter contract.

The list output contains typed results and an explicit total. Query and count use
the same user filters, soft-delete scope, and force filters.

## Identity and soft delete

Identity lookup is a configurable operation. It must support public or secondary
keys and must apply force filters and soft-delete scope before returning an entity.
This avoids loading an out-of-scope entity before authorization or returning an
account-specific existence oracle.

Delete is a configurable strategy. Physical SQLR deletion remains the default for
simple resources. A service may provide a soft-delete strategy that updates a
`deleted_at` field and then bind `DeleteTyped` (or provide
`DeleteTypedOperation`) to return a typed output through response negotiation.
The standard `Delete` operation remains an explicit 204 response escape hatch.
SQLH must not infer soft delete from a field name. Soft delete is explicit because
it affects read, list, count, update, delete, association, and event behavior.

## HTTP and errors

The new typed path returns ordinary output values rather than constructing
`httpserver.Response` values. The response-negotiation HTTP server selects the
representation. JSON-only helpers must not call `NewJsonResponse` internally in
the typed path.

Errors are returned to the HTTP server and mapped centrally. SQLH should provide
only generic SQL errors; applications register mappings for not-found,
validation, forbidden, locked, and domain-specific errors.

## Domain boundary

SQLH does not contain authz, experiment status rules, validation, event
publication, dispatcher behavior, or metrics. These are implemented by the
application operation or repository adapter. SQLH provides the transaction,
repository, query, identity, and output seams required to run them.

## Implementation order

1. Update the HTTP server dependency to a released response-negotiation version.
2. Replace Gin-context transaction middleware with `TxRunner` and
   `sqlr.RepositoryTx` operations.
3. Add typed output transformers and operation builders.
4. Add generic list input, force filters, query/count planning, and list output.
5. Add public/secondary identity lookup and delete strategies.
6. Add convenience CRUD route registration and the independent PATCH operation.
7. Add SQLH unit and integration tests.
8. Verify JSON Merge Patch presence, null, and association synchronization semantics.

## Verification

Run:

```text
gofmt -w .
go build ./...
go test ./...
golangci-lint run
```

No files in `backend.experiment-service` are part of this work.
The plan remains in the branch for review and will be removed separately.
