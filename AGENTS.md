# AGENTS.md

## Purpose
This repository provides generic HTTP helpers for Go services built on Gin,
`httpserver`, `sqlc`, and `sqlr`.

Primary responsibilities:
- wrap handlers in SQL transactions with `WithTx`
- bind transactions into Gin handlers with `BindTx`, `BindTxR`, `BindTxN`, `BindTxNR`
- generate CRUD endpoints with `WithCrudHandlers` and `HandlerCrud`
- map request/response DTOs through transformer interfaces
- drive relation preloading and sync behavior through `sqlh` struct tags

## Repository Layout
- root package: library code and unit tests
- `test/`: integration tests guarded by `integration` and `fixtures` build tags
- `test/migrations/`: SQL schema used by the integration suite
- `examples/basic/`: minimal CRUD example
- `examples/result_transformer/`: JSON result transformer example

## Repo-Local Agent Rules
I did not find any of these repo-local instruction files:
- `.cursorrules`
- `.cursor/rules/`
- `.github/copilot-instructions.md`

There are no extra Cursor or Copilot rules to merge beyond this file.

## Toolchain
- Go version: `go 1.25.7` from `go.mod`
- lint config: `.golangci.yml`
- CI workflow: `.github/workflows/ci.yml`
- mock generation config: `.mockery.yml`

## Canonical Commands
Run commands from the repository root unless a task explicitly targets `test/`.

### Build
- build everything: `go build ./...`
- build only the root package: `go build .`

### Unit Tests
- run all unit tests: `go test ./...`
- run verbose unit tests: `go test -v ./...`
- rerun without cache: `go test -count=1 ./...`
- run one test across packages: `go test ./... -run '^TestHandlerCrud_TagBuildersApplyToReadAndQuery$'`
- run one test in the root package only: `go test . -run '^TestHandlerCrud_TagBuildersApplyToReadAndQuery$'`

### Integration Tests
Integration coverage lives in `test/` and depends on build tags plus the MySQL
fixture setup from `test/config.yml` and `test/fixtures.go`.

- run all integration tests: `go test -v -tags='integration fixtures' ./test`
- run the suite entrypoint: `go test -v -tags='integration fixtures' ./test -run '^TestCrudIntegrationTestSuite$'`
- run one suite method: `go test -v -tags='integration fixtures' ./test -run '^TestCrudIntegrationTestSuite$' -testify.m '^TestReadPostPreloadsAssociations$'`

### Lint / Format
- lint: `golangci-lint run`
- format: `gofmt -w .`
- stricter formatting when available: `gofumpt -w .`
- normalize generated imports with `goimports` when needed

### Mock Generation
- generate mocks from `.mockery.yml`: `mockery`

## What CI Checks
CI currently runs:
- `go build ./...`
- `go test -v ./...`
- `golangci-lint` via `golangci/golangci-lint-action@v9` using version `v2.10.1`

If you change code, try to leave all three green.

## Lint Rules That Matter
The repo disables default linters and enables a curated set.

Important practical consequences:
- unchecked errors fail (`errcheck`)
- unused variables, code, and ineffective assignments fail (`unused`, `ineffassign`)
- suspicious constructs fail (`govet`, `staticcheck`)
- high nesting and cognitive complexity are watched (`nestif`, `gocognit`)
- long lines are allowed up to 240 chars (`lll`)
- blank-line style around returns and control flow is enforced (`nlreturn`, `whitespace`)
- duplicate TODO-like comments are flagged (`godox`, `dupword`)
- `nolint` directives must be specific and explained (`nolintlint`)

Special cases:
- `_test.go` files are exempt from `dogsled`, `goconst`, and `lll`
- `examples/` is excluded from lint and formatter enforcement

## Code Style

### Imports
- use standard Go import grouping and let `gofmt` or `gofumpt` order imports
- keep stdlib imports separate from third-party imports
- avoid aliases unless needed for a real collision or readability benefit
- alias `httpserver` as `gosolinehttpserver` only where ambiguity would hurt clarity

### Formatting
- follow `gofmt` exactly
- prefer `gofumpt`-compatible structure
- favor short blocks and early returns over nested branches
- add a blank line before `return` in longer logical blocks when it improves readability
- do not chase very short lines; the repo accepts lines up to 240 characters

### Types and Generics
- preserve the existing generic style built around `K`, `E`, create DTOs, and update DTOs
- keep constraints specific, such as `sqlr.KeyTypes` and `sqlr.Entitier[K]`
- prefer concrete types over `any`; reserve `any` for abstraction or JSON passthrough boundaries
- keep input DTOs, entities, and output DTOs as distinct types when behavior differs
- use pointers consistently when functions mutate or populate values

### Naming
- exported identifiers use PascalCase
- unexported identifiers use lowerCamelCase
- receivers stay short and conventional, commonly `h`, `t`, `r`, or `s`
- prefer descriptive helper and fixture type names in tests
- use `ID` in your own fields such as `AuthorID` and `ChildID`
- preserve upstream embedded field names like `sqlr.Entity.Id`, `CreatedAt`, and `UpdatedAt`
- JSON and DB tags use snake_case

### Control Flow and APIs
- prefer early returns over `else` nesting
- initialize `var err error` for multi-step flows that reuse the same error variable
- keep constructors and factories explicit and dependency injected
- prefer small focused helpers over large condition-heavy methods
- compose builder functions instead of duplicating query-builder logic
- preserve public API stability unless the task explicitly asks for a breaking change

### Error Handling
- return errors; do not panic in library code
- wrap errors with context via `fmt.Errorf("...: %w", err)`
- use specific, action-oriented messages such as `failed to create entity`
- use `errors.Is` for sentinel comparisons
- if an error is intentionally ignored, keep a specific explained `nolint` comment like the existing Gin context calls

### Comments and Documentation
- exported functions and types should have doc comments
- keep comments factual and concise
- explain non-obvious generics, reflection, builder, and tag-parsing behavior
- avoid comments that merely restate the code
- avoid leaving stray `TODO`, `FIXME`, or `BUG` comments unless they are intentional and necessary

### Struct Tags and Data Mapping
- API DTOs use `json` tags
- persisted fields use `db` tags
- relationship metadata uses `sqlr` tags
- optional CRUD relation behavior uses `sqlh` tags
- supported `sqlh` directives: `preload`, `sync`
- valid `preload` phases: `read`, `query`, `update`
- valid `sync` phases: `create`, `update`, `delete`
- do not place `sqlh` tags on embedded fields; the parser rejects that pattern

### Testing Style
- keep unit tests next to the code under test
- keep integration tests under `test/` with the required build tags
- prefer `require` from `testify` in unit tests
- use explicit expected structs when output shape matters
- use table tests when they improve coverage, not by default
- add `t.Parallel()` only when the test is actually isolated

## Guidance For Edits
- match the existing package architecture before introducing new patterns
- when touching exported APIs, update doc comments and tests in the same change
- when adding relation behavior, update both parser tests and CRUD builder tests
- if you add build-tagged code, make sure it still works with the tags from `.golangci.yml`
- avoid editing `examples/` unless the task is specifically about examples or public API usage

## Finish Checklist
- `go build ./...`
- `go test ./...`
- `golangci-lint run`
- if integration behavior changed: `go test -v -tags='integration fixtures' ./test`
