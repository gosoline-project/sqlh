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

// InputById is the standard URI input for operations that address one entity.
// It carries force filters so an authorization policy can restrict the lookup
// before SQLR reads or mutates the entity.
type InputById[K sqlr.KeyTypes] struct {
	ForceFilters
	Id K `uri:"id" json:"-"`
}

// GetId returns the URI identity. It is used by update inputs that embed
// InputById.
func (i InputById[K]) GetId() K {
	return i.Id
}

// Identified is the minimum contract for an update input. Embedding
// [InputById] is the usual implementation and also supplies the force-filter
// carrier used to scope the pre-update lookup.
type Identified[K sqlr.KeyTypes] interface {
	ForceFilterSource
	GetId() K
}

// ListOutput is the standard typed response for list operations. Results are
// mapped by the CRUD definition's Output function and Total is computed from
// the same scope before pagination is applied.
type ListOutput[O any] struct {
	Results []O `json:"results"`
	Total   int `json:"total"`
}

// IdentityLookup replaces SQLH's default primary-key lookup. The supplied
// builder contains relation preloads derived from sqlh tags. The supplied scope
// must be applied to the query used by the custom lookup.
type IdentityLookup[Id sqlr.KeyTypes, K sqlr.KeyTypes, E sqlr.Entitier[K]] func(
	ctx context.Context,
	tx sqlr.TTx,
	repository sqlr.RepositoryTx[K, E],
	id Id,
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
	Id sqlr.KeyTypes,
	IC any,
	IU Identified[Id],
	LI ListInputSource,
	O any,
] struct {
	// CreateInput maps a create request into a new entity.
	CreateInput func(context.Context, *IC) (*E, error)
	// UpdateInput applies a complete update input to an entity loaded by the
	// scoped identity lookup. The default PUT operation passes the bound input.
	// The default PATCH operation passes the input returned by
	// PatchInputFromEntity after it merges the request document.
	UpdateInput func(context.Context, *E, *IU) (*E, error)
	// PatchInputFromEntity maps a loaded entity to the complete update input into
	// which SQLH merges the JSON Merge Patch document. Fields omitted from the
	// request retain their current values.
	PatchInputFromEntity func(context.Context, *E) (*IU, error)
	// Output maps one persisted entity to the public response value.
	Output func(context.Context, *E) (O, error)

	// PatchAssociations maps JSON Merge Patch association paths to SQLR relation
	// paths. When it is empty, SQLH derives JSON paths from the complete update
	// input's json tags and the entity relation names. Only paths also configured
	// with sync:update are eligible for PATCH synchronization.
	PatchAssociations map[string]string
	// PatchAssociationTriggers maps non-association JSON paths in the original
	// patch document to entity relation paths whose derived values UpdateInput
	// changes. A trigger only selects the relation for persistence. The relation
	// must be configured with sync:update.
	PatchAssociationTriggers map[string]string

	// Identity replaces the default primary-key lookup used by read, update,
	// patch, and delete. The supplied scope must be applied by custom implementations.
	Identity IdentityLookup[Id, K, E]
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
	ReadOperation   TxOperation[InputById[Id], O]
	UpdateOperation TxOperation[IU, O]
	PatchOperation  TxOperation[PatchInput[Id], O]
	ListOperation   TxOperation[LI, ListOutput[O]]
	// DeleteOperation replaces the complete default delete mutation and returns
	// the entity used by DeleteOutput. When it is nil, SQLH uses the configured
	// identity, scope, and delete strategy.
	DeleteOperation TxOperation[InputById[Id], *E]
	// DeleteOutput maps the deleted entity to the negotiated response value used
	// by Delete. It has the same signature as Output, so callers can assign
	// Output directly. DeleteNoContent does not call this mapper.
	DeleteOutput func(context.Context, *E) (O, error)
}

// CrudDefinitionFactory constructs a CRUD definition during application
// startup. It is separate from the repository factory so application-specific
// dependencies can be initialized without making SQLH depend on them.
type CrudDefinitionFactory[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	Id sqlr.KeyTypes,
	IC any,
	IU Identified[Id],
	LI ListInputSource,
	O any,
] func(ctx context.Context, config cfg.Config, logger log.Logger) (CrudDefinition[K, E, Id, IC, IU, LI, O], error)

// SimpleCrudDefinition wraps a static definition in the standard gosoline
// factory shape.
func SimpleCrudDefinition[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	Id sqlr.KeyTypes,
	IC any,
	IU Identified[Id],
	LI ListInputSource,
	O any,
](definition CrudDefinition[K, E, Id, IC, IU, LI, O]) CrudDefinitionFactory[K, E, Id, IC, IU, LI, O] {
	return func(context.Context, cfg.Config, log.Logger) (CrudDefinition[K, E, Id, IC, IU, LI, O], error) {
		return definition, nil
	}
}

// NewCrudDefinition creates a definition using the mapper callbacks required
// by the standard CRUD operations.
func NewCrudDefinition[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	Id sqlr.KeyTypes,
	IC any,
	IU Identified[Id],
	O any,
](
	createInput func(context.Context, *IC) (*E, error),
	updateInput func(context.Context, *E, *IU) (*E, error),
	patchInputFromEntity func(context.Context, *E) (*IU, error),
	output func(context.Context, *E) (O, error),
) CrudDefinition[K, E, Id, IC, IU, ListInput, O] {
	return CrudDefinition[K, E, Id, IC, IU, ListInput, O]{
		CreateInput:          createInput,
		UpdateInput:          updateInput,
		PatchInputFromEntity: patchInputFromEntity,
		Output:               output,
	}
}

// CrudHandler is a transaction-aware typed CRUD handler. Its methods have the public
// operation shape expected by httpserver.Bind and authz.Decorate.
type CrudHandler[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	Id sqlr.KeyTypes,
	IC any,
	IU Identified[Id],
	LI ListInputSource,
	O any,
] struct {
	resource *resource[K, E]

	patchOperation  TxOperation[PatchInput[Id], O]
	createOperation TxOperation[IC, O]
	readOperation   TxOperation[InputById[Id], O]
	updateOperation TxOperation[IU, O]
	listOperation   TxOperation[LI, ListOutput[O]]
	deleteOperation TxOperation[InputById[Id], *E]
	deleteOutput    func(context.Context, *E) (O, error)
}

// NewCrudHandler creates a handler factory for a typed CRUD definition.
func NewCrudHandler[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	Id sqlr.KeyTypes,
	IC any,
	IU Identified[Id],
	LI ListInputSource,
	O any,
](definitionFactory CrudDefinitionFactory[K, E, Id, IC, IU, LI, O], options ...Option[K, E]) httpserver.HandlerFactory[CrudHandler[K, E, Id, IC, IU, LI, O]] {
	opts := newOpts[K, E]()
	for _, option := range options {
		if option != nil {
			option(opts)
		}
	}

	return func(ctx context.Context, config cfg.Config, logger log.Logger) (*CrudHandler[K, E, Id, IC, IU, LI, O], error) {
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

		handler, err := newCrudHandler(repository, runner, schema, definition)
		if err != nil {
			return nil, err
		}

		return handler, nil
	}
}

func newCrudHandler[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	Id sqlr.KeyTypes,
	IC any,
	IU Identified[Id],
	LI ListInputSource,
	O any,
](repository sqlr.CountingRepositoryTx[K, E], runner *TxRunner, schema *sqlr.EntitySchema, definition CrudDefinition[K, E, Id, IC, IU, LI, O]) (*CrudHandler[K, E, Id, IC, IU, LI, O], error) {
	res, err := newResource(repository, runner, schema)
	if err != nil {
		return nil, err
	}

	patchAssociationFields, patchAssociationTriggers, err := res.configurePatch[IU](definition.PatchAssociations, definition.PatchAssociationTriggers)
	if err != nil {
		return nil, err
	}

	createOperation, err := res.buildCreateOperation(definition.CreateOperation, definition.CreateInput, definition.Output)
	if err != nil {
		return nil, err
	}
	readOperation, err := res.buildReadOperation(definition.ReadOperation, definition.Identity, definition.DeleteScope, definition.Output)
	if err != nil {
		return nil, err
	}
	updateOperation, err := res.buildUpdateOperation(definition.UpdateOperation, definition.Identity, definition.DeleteScope, definition.UpdateInput, definition.Output)
	if err != nil {
		return nil, err
	}
	patchOperation, err := res.buildPatchOperation(
		definition.PatchOperation,
		definition.Identity,
		definition.DeleteScope,
		definition.PatchInputFromEntity,
		definition.UpdateInput,
		definition.Output,
		patchAssociationFields,
		patchAssociationTriggers,
	)
	if err != nil {
		return nil, err
	}
	listOperation, err := res.buildListOperation(definition.ListOperation, definition.DeleteScope, definition.Query, definition.Count, definition.Output)
	if err != nil {
		return nil, err
	}

	return &CrudHandler[K, E, Id, IC, IU, LI, O]{
		resource:        res,
		createOperation: createOperation,
		readOperation:   readOperation,
		updateOperation: updateOperation,
		patchOperation:  patchOperation,
		listOperation:   listOperation,
		deleteOperation: res.buildDeleteOperation(definition.DeleteOperation, definition.Identity, definition.DeleteScope, definition.Delete),
		deleteOutput:    definition.DeleteOutput,
	}, nil
}

// Create executes the create operation in a transaction and returns the typed
// output only after the transaction commits.
func (h *CrudHandler[K, E, Id, IC, IU, LI, O]) Create(ctx context.Context, input *IC) (O, error) {
	return h.resource.runner.RunValue(ctx, input, h.createOperation)
}

// Read executes a scoped identity lookup in a transaction and returns the
// typed output only after the transaction commits.
func (h *CrudHandler[K, E, Id, IC, IU, LI, O]) Read(ctx context.Context, input *InputById[Id]) (O, error) {
	return h.resource.runner.RunValue(ctx, input, h.readOperation)
}

// Update performs a scoped identity lookup, applies the update mapper, and
// persists the entity in one transaction.
func (h *CrudHandler[K, E, Id, IC, IU, LI, O]) Update(ctx context.Context, input *IU) (O, error) {
	return h.resource.runner.RunValue(ctx, input, h.updateOperation)
}

// Patch applies a JSON Merge Patch in a transaction and returns the typed
// output only after the transaction commits.
func (h *CrudHandler[K, E, Id, IC, IU, LI, O]) Patch(ctx context.Context, input *PatchInput[Id]) (O, error) {
	return h.resource.runner.RunValue(ctx, input, h.patchOperation)
}

// List queries and counts entities using one shared filter scope, then maps the
// results to the typed list output.
func (h *CrudHandler[K, E, Id, IC, IU, LI, O]) List(ctx context.Context, input *LI) (ListOutput[O], error) {
	return h.resource.runner.RunValue(ctx, input, h.listOperation)
}

// Delete performs the configured delete operation and maps its entity through
// DeleteOutput inside the transaction.
func (h *CrudHandler[K, E, Id, IC, IU, LI, O]) Delete(ctx context.Context, input *InputById[Id]) (O, error) {
	var zero O
	if h.deleteOutput == nil {
		return zero, fmt.Errorf("CRUD delete output mapper is required")
	}

	return h.resource.runner.RunValue(ctx, input, func(ctx context.Context, tx sqlr.TTx, input *InputById[Id]) (O, error) {
		entity, err := h.deleteOperation(ctx, tx, input)
		if err != nil {
			return zero, err
		}
		if entity == nil {
			return zero, fmt.Errorf("delete operation returned a nil entity")
		}

		output, err := h.deleteOutput(ctx, entity)
		if err != nil {
			return zero, fmt.Errorf("failed to transform deleted entity: %w", err)
		}

		return output, nil
	})
}

// DeleteNoContent performs the configured delete operation and returns 204 No
// Content without mapping an output.
func (h *CrudHandler[K, E, Id, IC, IU, LI, O]) DeleteNoContent(ctx context.Context, input *InputById[Id]) (httpserver.Response, error) {
	if _, err := h.resource.runner.RunValue(ctx, input, h.deleteOperation); err != nil {
		return nil, err
	}

	return httpserver.NewStatusResponse(http.StatusNoContent), nil
}

// Close releases resources held by the SQLR repository, including prepared
// statements when repository prepared statements are enabled.
func (h *CrudHandler[K, E, Id, IC, IU, LI, O]) Close() error {
	if h == nil || h.resource == nil {
		return nil
	}

	return h.resource.close()
}

// WithCrudHandlers registers the standard create, read, update, patch, delete,
// and list routes for a typed CRUD handler.
func WithCrudHandlers[
	K sqlr.KeyTypes,
	E sqlr.Entitier[K],
	Id sqlr.KeyTypes,
	IC any,
	IU Identified[Id],
	LI ListInputSource,
	O any,
](version int, entityName string, definitionFactory CrudDefinitionFactory[K, E, Id, IC, IU, LI, O], options ...Option[K, E]) httpserver.RegisterFactoryFunc {
	return httpserver.With(NewCrudHandler(definitionFactory, options...), func(router *httpserver.Router, handler *CrudHandler[K, E, Id, IC, IU, LI, O]) {
		path := fmt.Sprintf("/v%d/%s", version, entityName)
		router.POST(path, httpserver.Bind(handler.Create))
		router.GET(fmt.Sprintf("%s/:id", path), httpserver.Bind(handler.Read, httpserver.NoBodyBinding{}))
		router.PUT(fmt.Sprintf("%s/:id", path), httpserver.Bind(handler.Update))
		router.PATCH(fmt.Sprintf("%s/:id", path), httpserver.Bind(handler.Patch))
		router.DELETE(fmt.Sprintf("%s/:id", path), httpserver.Bind(handler.DeleteNoContent, httpserver.NoBodyBinding{}))
		router.POST(fmt.Sprintf("/v%d/%s", version, inflection.Plural(entityName)), httpserver.Bind(handler.List))
	})
}
