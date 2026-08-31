package sqlh

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/jinzhu/inflection"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

// InputByID is the standard URI input for operations that address one entity.
// It carries force filters so an authorization policy can restrict the lookup
// before SQLR reads or mutates the entity.
type InputByID[K sqlr.KeyTypes] struct {
	ForceFilters
	ID K `uri:"id" json:"-"`
}

// GetForceFilters returns a copy of the server-owned filters carried by the identity input.
func (i InputByID[K]) GetForceFilters() []ForceFilter {
	return i.filtersCopy()
}

// GetID returns the URI identity. It is used by update inputs that embed
// InputByID.
func (i InputByID[K]) GetID() K {
	return i.ID
}

// GetId mirrors SQLR's entity naming for callers that use the input as a small
// identity value outside SQLH.
func (i InputByID[K]) GetId() K {
	return i.ID
}

// Identified is the minimum contract for an update input. Embedding
// [InputByID] is the usual implementation and also supplies the force-filter
// carrier used to scope the pre-update lookup.
type Identified[K sqlr.KeyTypes] interface {
	ForceFilterSource
	GetID() K
}

// ListOutput is the standard typed response for list operations. Results are
// mapped by the CRUD definition's Output function and Total is computed from
// the same scope before pagination is applied.
type ListOutput[O any] struct {
	Results []O `json:"results"`
	Total   int `json:"total"`
}

// IdentityLookup replaces SQLH's default primary-key lookup. The supplied
// builder contains composed relation-tag and definition hooks, while scope must
// be applied to the query used by the custom lookup.
type IdentityLookup[ID sqlr.KeyTypes, K sqlr.KeyTypes, E sqlr.Entitier[K]] func(
	ctx context.Context,
	tx sqlr.TTx,
	repository sqlr.RepositoryTx[K, E],
	id ID,
	scope QueryScope,
	builder func(*sqlr.QueryBuilderSelect),
) (*E, error)

// ListQuery customizes the query part of a list operation. ApplyScope includes
// delete visibility, user, and force filters without pagination. ApplyPagination
// applies the request page after the scope has been installed.
type ListQuery[K sqlr.KeyTypes, E sqlr.Entitier[K], LI ListInputSource] func(
	ctx context.Context,
	tx sqlr.TTx,
	repository sqlr.RepositoryTx[K, E],
	input *LI,
	plan QueryPlan,
) ([]E, error)

// ListCount customizes the total calculation for a list operation. It receives
// the same scope as ListQuery, but pagination must not be applied to the count.
type ListCount[K sqlr.KeyTypes, E sqlr.Entitier[K], LI ListInputSource] func(
	ctx context.Context,
	tx sqlr.TTx,
	repository sqlr.RepositoryTx[K, E],
	input *LI,
	plan QueryPlan,
) (int, error)

// DeleteStrategy customizes deletion after SQLH has performed the scoped
// identity lookup. A strategy can update a soft-delete column instead of
// physically deleting the entity; pair it with CrudDefinition.DeleteScope to
// keep deleted rows out of default operations.
type DeleteStrategy[K sqlr.KeyTypes, E sqlr.Entitier[K]] func(
	ctx context.Context,
	tx sqlr.TTx,
	repository sqlr.RepositoryTx[K, E],
	entity *E,
) error

// DeleteScope restricts the rows considered visible to SQLH's default read, list,
// count, update, and delete operations. It is explicit so a custom delete
// strategy can implement soft-delete or archival semantics without SQLH
// guessing a column name.
type DeleteScope func(qb *sqlr.QueryBuilderSelect)

// CrudDefinition describes the mapping and extension points for a CRUD
// handler. The default operations use SQLR's transaction-aware repository. The
// *Operation fields can replace an individual operation completely when a
// resource needs domain-specific behavior.
type CrudDefinition[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	ID sqlr.KeyTypes,
	IC any,
	IU Identified[ID],
	LI ListInputSource,
	O any,
] struct {
	// CreateInput maps a create request into a new entity.
	CreateInput func(context.Context, *IC) (*E, error)
	// UpdateInput applies an update request to an entity loaded by the scoped
	// identity lookup.
	UpdateInput func(context.Context, *E, *IU) (*E, error)
	// PatchTarget creates the complete update target to which SQLH applies the
	// JSON Merge Patch document.
	PatchTarget func(context.Context, *E) (*IU, error)
	// PatchApply applies the merged patch target to the loaded entity.
	PatchApply func(context.Context, *E, *IU) (*E, error)
	// Output maps one persisted entity to the public response value.
	Output func(context.Context, *E) (O, error)

	// PatchAssociations maps JSON Merge Patch object paths to SQLR relation
	// paths. When it is empty, SQLH derives JSON paths from the patch target's
	// json tags and the entity relation names. Only paths also configured with
	// sync:update are eligible for PATCH synchronization.
	PatchAssociations map[string]string

	// Identity replaces the default primary-key lookup used by read, update,
	// patch, and delete. The supplied scope must be applied by custom implementations.
	Identity IdentityLookup[ID, K, E]
	// Query replaces the default SQLR list query.
	Query ListQuery[K, E, LI]
	// Count replaces the default total calculation.
	Count ListCount[K, E, LI]
	// Delete replaces physical SQLR deletion with a custom strategy.
	Delete DeleteStrategy[K, E]
	// DeleteScope restricts all default entity operations to rows that are
	// eligible for the configured delete strategy.
	DeleteScope DeleteScope

	// CreateOperation, ReadOperation, UpdateOperation, ListOperation, and
	// PatchOperation replace their corresponding default operation after
	// transaction setup.
	CreateOperation TxOperation[IC, O]
	ReadOperation   TxOperation[InputByID[ID], O]
	UpdateOperation TxOperation[IU, O]
	PatchOperation  TxOperation[PatchInput[ID], O]
	ListOperation   TxOperation[LI, ListOutput[O]]
	// DeleteOperation is an escape hatch for custom delete output/status. The
	// default operation returns an explicit 204 response.
	DeleteOperation TxOperation[InputByID[ID], httpserver.Response]
	// DeleteTypedOperation customizes the typed delete operation used by
	// [DeleteTyped]. It is useful for soft-delete flows that should return an
	// ordinary negotiated output instead of the default 204 response.
	DeleteTypedOperation TxOperation[InputByID[ID], O]

	// SQLR builder hooks. Relation tags are always composed before these hooks.
	BuilderCreate      func(*sqlr.QueryBuilderCreate)
	BuilderRead        func(*sqlr.QueryBuilderSelect)
	BuilderQuery       func(*sqlr.QueryBuilderSelect)
	BuilderDelete      func(*sqlr.QueryBuilderDelete)
	BuilderUpdateRead  func(*sqlr.QueryBuilderSelect)
	BuilderUpdateWrite func(*sqlr.QueryBuilderUpdate)
}

// CrudDefinitionFactory constructs a CRUD definition during application
// startup. It is separate from the repository factory so application-specific
// dependencies can be initialized without making SQLH depend on them.
type CrudDefinitionFactory[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	ID sqlr.KeyTypes,
	IC any,
	IU Identified[ID],
	LI ListInputSource,
	O any,
] func(ctx context.Context, config cfg.Config, logger log.Logger) (CrudDefinition[K, E, ID, IC, IU, LI, O], error)

// SimpleCrudDefinition wraps a static definition in the standard gosoline
// factory shape.
func SimpleCrudDefinition[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	ID sqlr.KeyTypes,
	IC any,
	IU Identified[ID],
	LI ListInputSource,
	O any,
](definition CrudDefinition[K, E, ID, IC, IU, LI, O]) CrudDefinitionFactory[K, E, ID, IC, IU, LI, O] {
	return func(context.Context, cfg.Config, log.Logger) (CrudDefinition[K, E, ID, IC, IU, LI, O], error) {
		return definition, nil
	}
}

// NewCrudDefinition creates a definition using the standard mapper callbacks.
func NewCrudDefinition[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	ID sqlr.KeyTypes,
	IC any,
	IU Identified[ID],
	O any,
](createInput func(context.Context, *IC) (*E, error), updateInput func(context.Context, *E, *IU) (*E, error), output func(context.Context, *E) (O, error)) CrudDefinition[K, E, ID, IC, IU, ListInput, O] {
	return CrudDefinition[K, E, ID, IC, IU, ListInput, O]{
		CreateInput: createInput,
		UpdateInput: updateInput,
		Output:      output,
	}
}

// CRUD is a transaction-aware typed CRUD handler. Its methods have the public
// operation shape expected by httpserver.Bind and authz.Decorate.
type CRUD[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	ID sqlr.KeyTypes,
	IC any,
	IU Identified[ID],
	LI ListInputSource,
	O any,
] struct {
	repository sqlr.RepositoryTx[K, E]
	runner     *TxRunner
	schema     *sqlr.EntitySchema
	definition CrudDefinition[K, E, ID, IC, IU, LI, O]

	builderCreate      func(*sqlr.QueryBuilderCreate)
	builderRead        func(*sqlr.QueryBuilderSelect)
	builderQuery       func(*sqlr.QueryBuilderSelect)
	builderDelete      func(*sqlr.QueryBuilderDelete)
	builderUpdateRead  func(*sqlr.QueryBuilderSelect)
	builderUpdateWrite func(*sqlr.QueryBuilderUpdate)

	patchAssociationFields map[string]string
	patchPreloadPaths      []string
	patchAutoSyncPaths     []string
	patchOperation         TxOperation[PatchInput[ID], O]
	createOperation        TxOperation[IC, O]
	readOperation          TxOperation[InputByID[ID], O]
	updateOperation        TxOperation[IU, O]
	listOperation          TxOperation[LI, ListOutput[O]]
	deleteOperation        TxOperation[InputByID[ID], httpserver.Response]
	deleteTypedOperation   TxOperation[InputByID[ID], O]
}

// NewCRUD creates a handler factory for a typed CRUD definition.
func NewCRUD[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	ID sqlr.KeyTypes,
	IC any,
	IU Identified[ID],
	LI ListInputSource,
	O any,
](definitionFactory CrudDefinitionFactory[K, E, ID, IC, IU, LI, O], options ...Option[K, E]) httpserver.HandlerFactory[CRUD[K, E, ID, IC, IU, LI, O]] {
	opts := newOpts[K, E]()
	for _, option := range options {
		if option != nil {
			option(opts)
		}
	}

	return func(ctx context.Context, config cfg.Config, logger log.Logger) (*CRUD[K, E, ID, IC, IU, LI, O], error) {
		if definitionFactory == nil {
			return nil, fmt.Errorf("CRUD definition factory is required")
		}

		definition, err := definitionFactory(ctx, config, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to create CRUD definition: %w", err)
		}

		if opts.repositoryFactory == nil {
			return nil, fmt.Errorf("transaction repository factory is required")
		}

		client, err := sqlc.ProvideClient(ctx, config, logger, opts.clientName)
		if err != nil {
			return nil, fmt.Errorf("failed to provide SQL client %q: %w", opts.clientName, err)
		}

		repository, err := opts.repositoryFactory(client, opts.repositorySettings)
		if err != nil {
			return nil, fmt.Errorf("failed to create transaction repository: %w", err)
		}

		runner, err := NewTxRunnerWithClient(client)
		if err != nil {
			return nil, fmt.Errorf("failed to create transaction runner: %w", err)
		}

		schema, err := sqlr.ParseSchema[E]()
		if err != nil {
			return nil, fmt.Errorf("failed to parse entity schema for CRUD handler: %w", err)
		}

		handler, err := newCRUD(repository, runner, schema, definition)
		if err != nil {
			return nil, err
		}

		return handler, nil
	}
}

func newCRUD[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	ID sqlr.KeyTypes,
	IC any,
	IU Identified[ID],
	LI ListInputSource,
	O any,
](repository sqlr.RepositoryTx[K, E], runner *TxRunner, schema *sqlr.EntitySchema, definition CrudDefinition[K, E, ID, IC, IU, LI, O]) (*CRUD[K, E, ID, IC, IU, LI, O], error) {
	if repository == nil {
		return nil, fmt.Errorf("transaction repository is required")
	}
	if runner == nil {
		return nil, fmt.Errorf("transaction runner is required")
	}
	if schema == nil || schema.PrimaryKey == nil {
		return nil, fmt.Errorf("entity schema with primary key is required")
	}

	tags, err := parseEntityBuilderTags[E]()
	if err != nil {
		return nil, fmt.Errorf("failed to parse entity %T %s tags: %w", *new(E), sqlhTagName, err)
	}

	patchAssociationFields, err := buildPatchAssociationFields[IU](tags.updateSyncPaths, definition.PatchAssociations)
	if err != nil {
		return nil, fmt.Errorf("failed to configure patch associations: %w", err)
	}

	patchAutoSyncPaths := append([]string(nil), schema.AutoSyncUpdatePaths()...)
	patchAutoSyncPaths = append(patchAutoSyncPaths, schema.AutoSyncMany2manyPaths()...)
	patchAutoSyncPaths = uniqueSortedStrings(patchAutoSyncPaths)

	handler := &CRUD[K, E, ID, IC, IU, LI, O]{
		repository:             repository,
		runner:                 runner,
		schema:                 schema,
		definition:             definition,
		patchAssociationFields: patchAssociationFields,
		patchPreloadPaths:      append([]string(nil), tags.updatePreloadPaths...),
		patchAutoSyncPaths:     patchAutoSyncPaths,
		builderCreate: composeBuilders(
			builderCreateFromTags(tags),
			definition.BuilderCreate,
		),
		builderRead: composeBuilders(
			builderLookupFromTags(tags),
			definition.BuilderRead,
		),
		builderQuery: composeBuilders(
			builderQueryFromTags(tags),
			definition.BuilderQuery,
		),
		builderDelete: composeBuilders(
			builderDeleteFromTags(tags),
			definition.BuilderDelete,
		),
		builderUpdateRead: composeBuilders(
			builderUpdateLookupFromTags(tags),
			definition.BuilderUpdateRead,
		),
		builderUpdateWrite: composeBuilders(
			builderUpdateWriteFromTags(tags),
			definition.BuilderUpdateWrite,
		),
	}

	if err := handler.configureCreateOperation(); err != nil {
		return nil, err
	}
	if err := handler.configureReadOperation(); err != nil {
		return nil, err
	}
	if err := handler.configureUpdateOperation(); err != nil {
		return nil, err
	}
	if err := handler.configurePatchOperation(); err != nil {
		return nil, err
	}
	if err := handler.configureListOperation(); err != nil {
		return nil, err
	}
	handler.configureDeleteOperation()
	handler.configureDeleteTypedOperation()

	return handler, nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) configureCreateOperation() error {
	if h.definition.CreateOperation != nil {
		h.createOperation = h.definition.CreateOperation

		return nil
	}
	if h.definition.CreateInput == nil {
		return fmt.Errorf("CRUD create input mapper is required")
	}
	if h.definition.Output == nil {
		return fmt.Errorf("CRUD output mapper is required")
	}

	h.createOperation = h.create

	return nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) configureReadOperation() error {
	if h.definition.ReadOperation != nil {
		h.readOperation = h.definition.ReadOperation

		return nil
	}
	if h.definition.Output == nil {
		return fmt.Errorf("CRUD output mapper is required")
	}

	h.readOperation = h.read

	return nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) configureUpdateOperation() error {
	if h.definition.UpdateOperation != nil {
		h.updateOperation = h.definition.UpdateOperation

		return nil
	}
	if h.definition.UpdateInput == nil {
		return fmt.Errorf("CRUD update input mapper is required")
	}
	if h.definition.Output == nil {
		return fmt.Errorf("CRUD output mapper is required")
	}

	h.updateOperation = h.update

	return nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) configurePatchOperation() error {
	if h.definition.PatchOperation != nil {
		h.patchOperation = h.definition.PatchOperation

		return nil
	}
	if h.definition.PatchTarget == nil && h.definition.PatchApply == nil {
		return nil
	}
	if h.definition.PatchTarget == nil {
		return fmt.Errorf("CRUD patch target mapper is required")
	}
	if h.definition.PatchApply == nil {
		return fmt.Errorf("CRUD patch apply mapper is required")
	}
	if h.definition.Output == nil {
		return fmt.Errorf("CRUD output mapper is required")
	}

	h.patchOperation = h.patch

	return nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) configureListOperation() error {
	if h.definition.ListOperation != nil {
		h.listOperation = h.definition.ListOperation

		return nil
	}
	if h.definition.Output == nil {
		return fmt.Errorf("CRUD output mapper is required")
	}

	h.listOperation = h.list

	return nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) configureDeleteOperation() {
	if h.definition.DeleteOperation != nil {
		h.deleteOperation = h.definition.DeleteOperation

		return
	}

	h.deleteOperation = h.delete
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) configureDeleteTypedOperation() {
	if h.definition.DeleteTypedOperation != nil {
		h.deleteTypedOperation = h.definition.DeleteTypedOperation

		return
	}

	h.deleteTypedOperation = h.deleteTyped
}

// Create executes the create operation in a transaction and returns the typed
// output only after the transaction commits.
func (h *CRUD[K, E, ID, IC, IU, LI, O]) Create(ctx context.Context, input *IC) (O, error) {
	return RunValue(ctx, h.runner, input, h.createOperation)
}

// Read executes a scoped identity lookup in a transaction and returns the
// typed output only after the transaction commits.
func (h *CRUD[K, E, ID, IC, IU, LI, O]) Read(ctx context.Context, input *InputByID[ID]) (O, error) {
	return RunValue(ctx, h.runner, input, h.readOperation)
}

// Update performs a scoped identity lookup, applies the update mapper, and
// persists the entity in one transaction.
func (h *CRUD[K, E, ID, IC, IU, LI, O]) Update(ctx context.Context, input *IU) (O, error) {
	return RunValue(ctx, h.runner, input, h.updateOperation)
}

// Patch applies a JSON Merge Patch in a transaction and returns the typed
// output only after the transaction commits. It is available when the CRUD
// definition configures PatchTarget and PatchApply or PatchOperation.
func (h *CRUD[K, E, ID, IC, IU, LI, O]) Patch(ctx context.Context, input *PatchInput[ID]) (O, error) {
	return RunValue(ctx, h.runner, input, h.patchOperation)
}

// List queries and counts entities using one shared filter scope, then maps the
// results to the typed list output.
func (h *CRUD[K, E, ID, IC, IU, LI, O]) List(ctx context.Context, input *LI) (ListOutput[O], error) {
	return RunValue(ctx, h.runner, input, h.listOperation)
}

// Delete performs a scoped identity lookup and then uses the configured delete
// strategy. The default response is 204 No Content.
func (h *CRUD[K, E, ID, IC, IU, LI, O]) Delete(ctx context.Context, input *InputByID[ID]) (httpserver.Response, error) {
	return RunValue(ctx, h.runner, input, h.deleteOperation)
}

// DeleteTyped performs the configured delete operation and returns its typed
// output. Bind this operation directly when a soft-delete endpoint should use
// response negotiation; the standard [Delete] operation remains a 204 escape
// hatch for conventional physical deletes.
func (h *CRUD[K, E, ID, IC, IU, LI, O]) DeleteTyped(ctx context.Context, input *InputByID[ID]) (O, error) {
	return RunValue(ctx, h.runner, input, h.deleteTypedOperation)
}

// Close releases resources held by the SQLR repository, including prepared
// statements when repository prepared statements are enabled.
func (h *CRUD[K, E, ID, IC, IU, LI, O]) Close() error {
	if h == nil || h.repository == nil {
		return nil
	}

	return h.repository.Close()
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) create(ctx context.Context, tx sqlr.TTx, input *IC) (O, error) {
	var zero O
	if input == nil {
		return zero, fmt.Errorf("create input is required")
	}

	entity, err := h.definition.CreateInput(ctx, input)
	if err != nil {
		return zero, fmt.Errorf("failed to transform create input: %w", err)
	}
	if entity == nil {
		return zero, fmt.Errorf("create input mapper returned a nil entity")
	}

	if err = h.repository.Create(tx, entity, h.builderCreate); err != nil {
		return zero, fmt.Errorf("failed to create entity: %w", err)
	}

	output, err := h.definition.Output(ctx, entity)
	if err != nil {
		return zero, fmt.Errorf("failed to transform created entity: %w", err)
	}

	return output, nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) read(ctx context.Context, tx sqlr.TTx, input *InputByID[ID]) (O, error) {
	var zero O
	if input == nil {
		return zero, fmt.Errorf("read input is required")
	}

	entity, err := h.lookup(ctx, tx, input.ID, h.lookupScope(input), h.builderRead)
	if err != nil {
		return zero, fmt.Errorf("failed to read entity with id %v: %w", input.ID, err)
	}

	output, err := h.definition.Output(ctx, entity)
	if err != nil {
		return zero, fmt.Errorf("failed to transform read entity: %w", err)
	}

	return output, nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) update(ctx context.Context, tx sqlr.TTx, input *IU) (O, error) {
	var zero O
	if input == nil {
		return zero, fmt.Errorf("update input is required")
	}

	value := *input
	id := value.GetID()
	entity, err := h.lookup(ctx, tx, id, h.lookupScope(value), h.builderUpdateRead)
	if err != nil {
		return zero, fmt.Errorf("failed to read entity before update with id %v: %w", id, err)
	}

	entity, err = h.definition.UpdateInput(ctx, entity, input)
	if err != nil {
		return zero, fmt.Errorf("failed to transform update input: %w", err)
	}
	if entity == nil {
		return zero, fmt.Errorf("update input mapper returned a nil entity")
	}

	entity, err = h.repository.Update(tx, entity, h.builderUpdateWrite)
	if err != nil {
		return zero, fmt.Errorf("failed to update entity with id %v: %w", id, err)
	}

	output, err := h.definition.Output(ctx, entity)
	if err != nil {
		return zero, fmt.Errorf("failed to transform updated entity: %w", err)
	}

	return output, nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) patch(ctx context.Context, tx sqlr.TTx, input *PatchInput[ID]) (O, error) {
	var zero O
	if input == nil {
		return zero, fmt.Errorf("patch input is required")
	}

	document := input.Document()
	if !document.valid() {
		return zero, fmt.Errorf("patch document is required")
	}

	entity, err := h.lookup(ctx, tx, input.ID, h.lookupScope(input), h.builderUpdateRead)
	if err != nil {
		return zero, fmt.Errorf("failed to read entity before patch with id %v: %w", input.ID, err)
	}

	target, err := h.definition.PatchTarget(ctx, entity)
	if err != nil {
		return zero, fmt.Errorf("failed to create patch target: %w", err)
	}
	if target == nil {
		return zero, fmt.Errorf("patch target mapper returned a nil target")
	}

	if err = document.MergeInto(target); err != nil {
		return zero, fmt.Errorf("failed to apply patch: %w", err)
	}

	entity, err = h.definition.PatchApply(ctx, entity, target)
	if err != nil {
		return zero, fmt.Errorf("failed to apply patched target: %w", err)
	}
	if entity == nil {
		return zero, fmt.Errorf("patch apply mapper returned a nil entity")
	}

	selectedPaths := selectPatchAssociationPaths(document, h.patchAssociationFields)
	if err = normalizePatchAssociationNulls(entity, document, h.patchAssociationFields, selectedPaths); err != nil {
		return zero, fmt.Errorf("failed to normalize patched associations: %w", err)
	}

	entity, err = h.repository.Update(tx, entity, builderPatchWriteFromTags(h.patchPreloadPaths, selectedPaths, h.patchAutoSyncPaths))
	if err != nil {
		return zero, fmt.Errorf("failed to update entity with id %v after patch: %w", input.ID, err)
	}

	output, err := h.definition.Output(ctx, entity)
	if err != nil {
		return zero, fmt.Errorf("failed to transform patched entity: %w", err)
	}

	return output, nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) list(ctx context.Context, tx sqlr.TTx, input *LI) (ListOutput[O], error) {
	if input == nil {
		return ListOutput[O]{}, fmt.Errorf("list input is required")
	}
	value := *input
	if err := value.ValidatePagination(); err != nil {
		return ListOutput[O]{}, err
	}
	if err := value.ApplyFilters(sqlr.NewQueryBuilderSelect()); err != nil {
		return ListOutput[O]{}, fmt.Errorf("failed to validate list filters: %w", err)
	}

	plan := QueryPlan{
		ApplyBuilder: h.builderQuery,
		ApplyScope: func(qb *sqlr.QueryBuilderSelect) error {
			if h.definition.DeleteScope != nil {
				h.definition.DeleteScope(qb)
			}

			return value.ApplyFilters(qb)
		},
		ApplyPagination: func(qb *sqlr.QueryBuilderSelect) {
			value.ApplyPagination(qb)
		},
	}

	var entities []E
	var err error
	var queryErr error
	if h.definition.Query != nil {
		entities, err = h.definition.Query(ctx, tx, h.repository, input, plan)
	} else {
		entities, err = h.repository.Query(tx, func(qb *sqlr.QueryBuilderSelect) {
			plan.ApplyBuilder(qb)
			if scopeErr := plan.ApplyScope(qb); scopeErr != nil {
				queryErr = scopeErr

				return
			}
			plan.ApplyPagination(qb)
		})
		if err == nil {
			err = queryErr
		}
	}
	if err != nil {
		return ListOutput[O]{}, fmt.Errorf("failed to query entities: %w", err)
	}

	var total int
	if h.definition.Count != nil {
		total, err = h.definition.Count(ctx, tx, h.repository, input, plan)
	} else {
		total, err = h.count(ctx, tx, plan)
	}
	if err != nil {
		return ListOutput[O]{}, fmt.Errorf("failed to count entities: %w", err)
	}

	results := make([]O, len(entities))
	for i := range entities {
		if results[i], err = h.definition.Output(ctx, &entities[i]); err != nil {
			return ListOutput[O]{}, fmt.Errorf("failed to transform list entity at index %d: %w", i, err)
		}
	}

	return ListOutput[O]{Results: results, Total: total}, nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) count(ctx context.Context, tx sqlr.TTx, plan QueryPlan) (int, error) {
	qb := sqlr.NewQueryBuilderSelect()
	plan.ApplyBuilder(qb)
	if err := plan.ApplyScope(qb); err != nil {
		return 0, err
	}

	clauses, params, err := qb.ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count scope: %w", err)
	}

	countQuery, _, err := sqlc.From(h.schema.TableName).
		Columns(sqlc.Col("*").Count().As("total")).
		ToSql()
	if err != nil {
		return 0, fmt.Errorf("failed to build count query: %w", err)
	}
	if clauses != "" {
		countQuery += " " + clauses
	}

	var total int
	if err = tx.Get(ctx, &total, countQuery, params...); err != nil {
		return 0, fmt.Errorf("failed to execute count query: %w", err)
	}

	return total, nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) delete(ctx context.Context, tx sqlr.TTx, input *InputByID[ID]) (httpserver.Response, error) {
	if _, err := h.deleteEntity(ctx, tx, input); err != nil {
		return nil, err
	}

	return httpserver.NewStatusResponse(http.StatusNoContent), nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) deleteTyped(ctx context.Context, tx sqlr.TTx, input *InputByID[ID]) (O, error) {
	var zero O

	entity, err := h.deleteEntity(ctx, tx, input)
	if err != nil {
		return zero, err
	}
	if h.definition.Output == nil {
		return zero, fmt.Errorf("CRUD output mapper is required for typed delete")
	}

	output, err := h.definition.Output(ctx, entity)
	if err != nil {
		return zero, fmt.Errorf("failed to transform deleted entity: %w", err)
	}

	return output, nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) deleteEntity(ctx context.Context, tx sqlr.TTx, input *InputByID[ID]) (*E, error) {
	if input == nil {
		return nil, fmt.Errorf("delete input is required")
	}

	builder := h.builderRead
	entity, err := h.lookup(ctx, tx, input.ID, h.lookupScope(input), builder)
	if err != nil {
		return nil, fmt.Errorf("failed to find entity before delete with id %v: %w", input.ID, err)
	}

	if h.definition.Delete != nil {
		if err = h.definition.Delete(ctx, tx, h.repository, entity); err != nil {
			return nil, fmt.Errorf("failed to delete entity with id %v: %w", input.ID, err)
		}
	} else if err = h.repository.Delete(tx, (*entity).GetId(), h.builderDelete); err != nil {
		return nil, fmt.Errorf("failed to delete entity with id %v: %w", input.ID, err)
	}

	return entity, nil
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) lookup(ctx context.Context, tx sqlr.TTx, id ID, scope QueryScope, builder func(*sqlr.QueryBuilderSelect)) (*E, error) {
	if h.definition.Identity != nil {
		return h.definition.Identity(ctx, tx, h.repository, id, scope, builder)
	}

	var queryErr error
	entities, err := h.repository.Query(tx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Where(sqlc.Col(h.schema.TableName, h.schema.PrimaryKey.Name).Eq(id))
		if scope != nil {
			queryErr = scope(qb)
			if queryErr != nil {
				return
			}
		}
		if builder != nil {
			builder(qb)
		}
		// Do not limit joined lookups to one SQL row. SQLR needs all rows from
		// has-many joins to hydrate the complete association.
	})
	if queryErr != nil {
		return nil, queryErr
	}
	if err != nil {
		return nil, err
	}
	if len(entities) == 0 {
		return nil, fmt.Errorf("entity id=%v: %w", id, sqlr.ErrNotFound)
	}

	return &entities[0], nil
}

func composeScopes(scopes ...QueryScope) QueryScope {
	return func(qb *sqlr.QueryBuilderSelect) error {
		for _, scope := range scopes {
			if scope == nil {
				continue
			}
			if err := scope(qb); err != nil {
				return err
			}
		}

		return nil
	}
}

func deleteScope(source DeleteScope) QueryScope {
	return func(qb *sqlr.QueryBuilderSelect) error {
		if source != nil {
			source(qb)
		}

		return nil
	}
}

func forceScope(source ForceFilterSource) QueryScope {
	return func(qb *sqlr.QueryBuilderSelect) error {
		applyForceFilters(source, qb)

		return nil
	}
}

func (h *CRUD[K, E, ID, IC, IU, LI, O]) lookupScope(source ForceFilterSource) QueryScope {
	return composeScopes(
		deleteScope(h.definition.DeleteScope),
		forceScope(source),
	)
}

// WithCrudHandlers registers the standard create, read, update, delete, and
// list routes for a typed CRUD handler. It also registers PATCH when the
// definition configures PatchTarget and PatchApply or PatchOperation.
func WithCrudHandlers[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	ID sqlr.KeyTypes,
	IC any,
	IU Identified[ID],
	LI ListInputSource,
	O any,
](version int, entityName string, definitionFactory CrudDefinitionFactory[K, E, ID, IC, IU, LI, O], options ...Option[K, E]) httpserver.RegisterFactoryFunc {
	return httpserver.With(NewCRUD(definitionFactory, options...), func(router *httpserver.Router, handler *CRUD[K, E, ID, IC, IU, LI, O]) {
		path := fmt.Sprintf("/v%d/%s", version, entityName)
		router.POST(path, httpserver.Bind(handler.Create))
		router.GET(fmt.Sprintf("%s/:id", path), httpserver.Bind(handler.Read, httpserver.NoBodyBinding{}))
		router.PUT(fmt.Sprintf("%s/:id", path), httpserver.Bind(handler.Update))
		if handler.patchOperation != nil {
			router.PATCH(fmt.Sprintf("%s/:id", path), httpserver.Bind(handler.Patch))
		}
		router.DELETE(fmt.Sprintf("%s/:id", path), httpserver.Bind(handler.Delete, httpserver.NoBodyBinding{}))
		router.POST(fmt.Sprintf("/v%d/%s", version, inflection.Plural(entityName)), httpserver.Bind(handler.List))
	})
}
