# AGENTS.md

## Scope
This repository provides typed, transaction-aware CRUD helpers for Go services built on `httpserver`, `sqlc`, and `sqlr`.

Primary responsibilities:
- run CRUD operations through `TxRunner`;
- expose typed handlers through `CrudHandler` and `WithCrudHandlers`;
- apply request filters, server-owned force filters, pagination, and delete scopes;
- map request DTOs to SQLR entities and entities to response DTOs;
- preload and synchronize relations through `sqlh` struct tags;
- implement JSON Merge Patch with selective association synchronization.

## Repository layout
- root package: library code and unit tests;
- `test/`: integration tests guarded by `integration` and `fixtures` build tags;
- `test/migrations/`: MySQL schema for integration tests;
- `examples/basic/`: minimal CRUD example.

## Toolchain
Tool versions are pinned in `mise.toml`:
- Go 1.27.0;
- golangci-lint 2.13.2.

## Canonical commands
Run commands from the repository root.

### Build and unit tests
- `go build ./...`
- `go test -count=1 ./...`
- `go test -count=1 . -run '^TestName$'`

### Integration tests
The suite starts its own MySQL container through dockertest, so it needs a
running Docker daemon but no manual database setup:
- `go test -count=1 -timeout=8m -tags='integration fixtures' ./test`
- `go test -count=1 -timeout=8m -tags='integration fixtures' ./test -run '^TestCrudIntegrationTestSuite$'`

`go test ./...` skips this suite because of the build tags, and reports `no test
files` for `./test`. Use the tagged command above to run it.

### Format and lint
- `gofmt -w .`
- `gofmt -l .`
- `golangci-lint run ./...`

## API conventions
- Keep public handlers typed as `func(context.Context, *Input) (Output, error)`.
- `*Operation` fields replace a complete default operation. Each override receives the active transaction and the configured repository.
- `DeleteOperation` returns the deleted entity. `Delete` maps it through `DeleteOutput`; `DeleteNoContent` skips mapping and returns 204.
- `DeleteOutput` has the same signature as `Output`, so callers can assign `definition.DeleteOutput = definition.Output`.
- Force filters are server-owned restrictions. SQLH applies them; custom list inputs must not duplicate them in optional trusted `ApplyFilters` methods.
- Public filters and ordering use SQLH `JsonFilter`/`JsonOrder` with public query `field` names, never request-provided database columns or SQL expressions. Filter operators remain `sqlc.JsonFilterType` constants.
- `CrudDefinition.FieldMappings` maps public names to entity Go field paths. A non-nil map is the complete allowlist; an empty map exposes nothing. Nil discovers visible exported Go scalar names backed by SQLR database metadata, never binding tags; `sqlh:"field:alias"` overrides the automatic name and `sqlh:"field:-"` excludes automatic exposure. Explicit maps bypass these defaults. Public names use Unicode simple-fold case-insensitive lookup, not naming-style normalization, and active collisions fail construction. Go structural promotion/shadowing is resolved before aliases. Nonpersisted fields and relation collections stay excluded. Explicit paths support only a direct to-one relation, not to-many or multi-hop paths. SQLH qualifies base columns with the entity table.
- Custom list inputs implement `ListInputSource`, including `GetFilter`, `GetOrderBy`, pagination, force filters, and validators. SQLH validates public fields and operators, pagination, and query modifiers before default queries or custom query/count callbacks.
- `ListInput` does not expose raw `ApplyFilters` or `ApplyQueryModifiers` methods. Custom inputs may implement these optional trusted server-side extensions for shared restrictions and row-only modifiers respectively; never forward unvalidated public identifiers through them.
- Custom list query and count callbacks must apply `QueryPlan.ApplyBuilder` and `QueryPlan.ApplyScope`. Query callbacks must then apply `ApplyQueryModifiers` and `ApplyPagination`, in that order. Count callbacks must apply neither: default count omits row modifiers, ordering, and pagination. Cardinality-changing row modifiers require a matching custom count.
- Relation behavior comes from `sqlh` tags. Do not add public query-builder hooks without a demonstrated requirement.

## Struct tags
- API fields use `json` tags.
- Persisted fields use `db` tags.
- SQLR relationships use `sqlr` tags.
- SQLH relation phases use `sqlh` tags.
- Supported directives are `preload` and `sync`.
- Valid preload phases are `create`, `read`, `query`, and `update`.
- Valid sync phases are `create`, `update`, and `delete`.
- Do not put `sqlh` tags on embedded fields.

## Code style
- Use gofmt formatting and standard import groups.
- Use `Id` in SQLH-owned identifiers; preserve upstream names such as `sqlr.Entity.Id`.
- Keep generic constraints specific, including `sqlr.KeyTypes` and `sqlr.Entitier[K]`.
- Prefer early returns and focused helpers.
- Return errors instead of panicking in library code.
- Wrap errors with useful context and `%w`.
- Add concise doc comments to exported identifiers.
- Keep JSON and database names stable when changing Go identifiers.

## Testing
- Keep unit tests beside the code under test.
- Use `require` for required assertions.
- Test default and custom callback paths when changing extension-point behavior.
- Test query and count together when changing list scope behavior.
- Test commit, rollback, and panic paths when changing transactions.
- Run integration tests when relation synchronization or HTTP CRUD behavior changes.

## What CI checks
The `ci` workflow runs four jobs on every pull request and on pushes to `main`:
- `build`: `go build ./...`;
- `test`: `go test -v ./...` (unit tests only);
- `integration`: `go test -count=1 -timeout=8m -tags='integration fixtures' ./test`;
- `lint`: golangci-lint, configured with the `integration` and `fixtures` build tags.

## Finish checklist
- `gofmt -w .`
- `go build ./...`
- `go test -count=1 ./...`
- `go test -count=1 -timeout=8m -tags='integration fixtures' ./test`
- `golangci-lint run ./...`
