package sqlh

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
)

type resourceBuilderHooks struct {
	create      func(*sqlr.QueryBuilderCreate)
	read        func(*sqlr.QueryBuilderSelect)
	query       func(*sqlr.QueryBuilderSelect)
	delete      func(*sqlr.QueryBuilderDelete)
	updateRead  func(*sqlr.QueryBuilderSelect)
	updateWrite func(*sqlr.QueryBuilderUpdate)
}

type resource[K sqlr.KeyTypes, E sqlr.Entitier[K]] struct {
	repository sqlr.CountingRepositoryTx[K, E]
	runner     *TxRunner
	schema     *sqlr.EntitySchema
	tags       *entityBuilderTags

	builderCreate      func(*sqlr.QueryBuilderCreate)
	builderRead        func(*sqlr.QueryBuilderSelect)
	builderQuery       func(*sqlr.QueryBuilderSelect)
	builderDelete      func(*sqlr.QueryBuilderDelete)
	builderUpdateRead  func(*sqlr.QueryBuilderSelect)
	builderUpdateWrite func(*sqlr.QueryBuilderUpdate)

	patchAutoSyncPaths []string
}

func newResource[K sqlr.KeyTypes, E sqlr.Entitier[K]](
	repository sqlr.CountingRepositoryTx[K, E],
	runner *TxRunner,
	schema *sqlr.EntitySchema,
	hooks resourceBuilderHooks,
) (*resource[K, E], error) {
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

	patchAutoSyncPaths := append([]string(nil), schema.AutoSyncUpdatePaths()...)
	patchAutoSyncPaths = append(patchAutoSyncPaths, schema.AutoSyncMany2manyPaths()...)
	patchAutoSyncPaths = uniqueSortedStrings(patchAutoSyncPaths)

	return &resource[K, E]{
		repository: repository,
		runner:     runner,
		schema:     schema,
		tags:       tags,
		builderCreate: composeBuilders(
			builderCreateFromTags(tags),
			hooks.create,
		),
		builderRead: composeBuilders(
			builderLookupFromTags(tags),
			hooks.read,
		),
		builderQuery: composeBuilders(
			builderQueryFromTags(tags),
			hooks.query,
		),
		builderDelete: composeBuilders(
			builderDeleteFromTags(tags),
			hooks.delete,
		),
		builderUpdateRead: composeBuilders(
			builderUpdateLookupFromTags(tags),
			hooks.updateRead,
			builderForUpdate,
		),
		builderUpdateWrite: composeBuilders(
			builderUpdateWriteFromTags(tags),
			hooks.updateWrite,
		),
		patchAutoSyncPaths: patchAutoSyncPaths,
	}, nil
}

func (r *resource[K, E]) configurePatch[IU any](associations map[string]string, triggers map[string]string) (associationFields map[string]string, associationTriggers map[string]string, err error) {
	associationFields, err = buildPatchAssociationFields[IU](r.tags.updateSyncPaths, associations)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to configure patch associations: %w", err)
	}

	associationTriggers, err = buildPatchAssociationTriggers(r.tags.updateSyncPaths, triggers)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to configure patch association triggers: %w", err)
	}

	return associationFields, associationTriggers, nil
}

func (r *resource[K, E]) buildCreateOperation[IC, O any](
	custom TxOperation[IC, O],
	createInput func(context.Context, *IC) (*E, error),
	output func(context.Context, *E) (O, error),
) (TxOperation[IC, O], error) {
	if custom != nil {
		return custom, nil
	}
	if createInput == nil {
		return nil, fmt.Errorf("CRUD create input mapper is required")
	}
	if output == nil {
		return nil, fmt.Errorf("CRUD output mapper is required")
	}

	return func(ctx context.Context, tx sqlr.TTx, input *IC) (O, error) {
		var zero O
		if input == nil {
			return zero, fmt.Errorf("create input is required")
		}

		entity, err := createInput(ctx, input)
		if err != nil {
			return zero, fmt.Errorf("failed to transform create input: %w", err)
		}
		if entity == nil {
			return zero, fmt.Errorf("create input mapper returned a nil entity")
		}

		if err = r.repository.Create(tx, entity, r.builderCreate); err != nil {
			return zero, fmt.Errorf("failed to create entity: %w", err)
		}

		result, err := output(ctx, entity)
		if err != nil {
			return zero, fmt.Errorf("failed to transform created entity: %w", err)
		}

		return result, nil
	}, nil
}

func (r *resource[K, E]) buildReadOperation[Id sqlr.KeyTypes, O any](
	custom TxOperation[InputById[Id], O],
	identity IdentityLookup[Id, K, E],
	visibility DeleteScope,
	output func(context.Context, *E) (O, error),
) (TxOperation[InputById[Id], O], error) {
	if custom != nil {
		return custom, nil
	}
	if output == nil {
		return nil, fmt.Errorf("CRUD output mapper is required")
	}

	return func(ctx context.Context, tx sqlr.TTx, input *InputById[Id]) (O, error) {
		var zero O
		if input == nil {
			return zero, fmt.Errorf("read input is required")
		}

		entity, err := r.lookup(ctx, tx, identity, input.Id, resourceLookupScope(input, visibility), r.builderRead)
		if err != nil {
			return zero, fmt.Errorf("failed to read entity with id %v: %w", input.Id, err)
		}

		result, err := output(ctx, entity)
		if err != nil {
			return zero, fmt.Errorf("failed to transform read entity: %w", err)
		}

		return result, nil
	}, nil
}

func (r *resource[K, E]) buildUpdateOperation[Id sqlr.KeyTypes, IU Identified[Id], O any](
	custom TxOperation[IU, O],
	identity IdentityLookup[Id, K, E],
	visibility DeleteScope,
	updateInput func(context.Context, *E, *IU) (*E, error),
	output func(context.Context, *E) (O, error),
) (TxOperation[IU, O], error) {
	if custom != nil {
		return custom, nil
	}
	if updateInput == nil {
		return nil, fmt.Errorf("CRUD update input mapper is required")
	}
	if output == nil {
		return nil, fmt.Errorf("CRUD output mapper is required")
	}

	return func(ctx context.Context, tx sqlr.TTx, input *IU) (O, error) {
		var zero O
		if input == nil {
			return zero, fmt.Errorf("update input is required")
		}

		value := *input
		id := value.GetId()
		entity, err := r.lookup(ctx, tx, identity, id, resourceLookupScope(value, visibility), r.builderUpdateRead)
		if err != nil {
			return zero, fmt.Errorf("failed to read entity before update with id %v: %w", id, err)
		}

		entity, err = updateInput(ctx, entity, input)
		if err != nil {
			return zero, fmt.Errorf("failed to transform update input: %w", err)
		}
		if entity == nil {
			return zero, fmt.Errorf("update input mapper returned a nil entity")
		}

		entity, err = r.repository.Update(tx, entity, r.builderUpdateWrite)
		if err != nil {
			return zero, fmt.Errorf("failed to update entity with id %v: %w", id, err)
		}

		result, err := output(ctx, entity)
		if err != nil {
			return zero, fmt.Errorf("failed to transform updated entity: %w", err)
		}

		return result, nil
	}, nil
}

func (r *resource[K, E]) buildPatchOperation[Id sqlr.KeyTypes, IU Identified[Id], O any](
	custom TxOperation[PatchInput[Id], O],
	identity IdentityLookup[Id, K, E],
	visibility DeleteScope,
	patchInputFromEntity func(context.Context, *E) (*IU, error),
	updateInput func(context.Context, *E, *IU) (*E, error),
	output func(context.Context, *E) (O, error),
	associationFields map[string]string,
	associationTriggers map[string]string,
) (TxOperation[PatchInput[Id], O], error) {
	if custom != nil {
		return custom, nil
	}
	if patchInputFromEntity == nil {
		return nil, fmt.Errorf("CRUD patch input from entity mapper is required")
	}
	if updateInput == nil {
		return nil, fmt.Errorf("CRUD update input mapper is required for default patch operation")
	}
	if output == nil {
		return nil, fmt.Errorf("CRUD output mapper is required")
	}

	return func(ctx context.Context, tx sqlr.TTx, input *PatchInput[Id]) (O, error) {
		return r.patch(ctx, tx, input, identity, visibility, patchInputFromEntity, updateInput, output, associationFields, associationTriggers)
	}, nil
}

func (r *resource[K, E]) patch[Id sqlr.KeyTypes, IU Identified[Id], O any](
	ctx context.Context,
	tx sqlr.TTx,
	input *PatchInput[Id],
	identity IdentityLookup[Id, K, E],
	visibility DeleteScope,
	patchInputFromEntity func(context.Context, *E) (*IU, error),
	updateInput func(context.Context, *E, *IU) (*E, error),
	output func(context.Context, *E) (O, error),
	associationFields map[string]string,
	associationTriggers map[string]string,
) (O, error) {
	var zero O
	if input == nil {
		return zero, fmt.Errorf("patch input is required")
	}

	document := input.Document()
	if !document.valid() {
		return zero, fmt.Errorf("patch document is required")
	}

	entity, err := r.lookup(ctx, tx, identity, input.Id, resourceLookupScope(input, visibility), r.builderUpdateRead)
	if err != nil {
		return zero, fmt.Errorf("failed to read entity before patch with id %v: %w", input.Id, err)
	}

	completeInput, err := patchInputFromEntity(ctx, entity)
	if err != nil {
		return zero, fmt.Errorf("failed to create patch input from entity: %w", err)
	}
	if completeInput == nil {
		return zero, fmt.Errorf("patch input from entity mapper returned nil")
	}

	if err = document.MergeInto(completeInput); err != nil {
		return zero, fmt.Errorf("failed to apply patch: %w", err)
	}

	entity, err = updateInput(ctx, entity, completeInput)
	if err != nil {
		return zero, fmt.Errorf("failed to transform merged patch input: %w", err)
	}
	if entity == nil {
		return zero, fmt.Errorf("update input mapper returned a nil entity")
	}

	selectedPaths := selectPatchAssociationPaths(document, associationFields)
	selectedPaths = append(selectedPaths, selectPatchAssociationPaths(document, associationTriggers)...)
	selectedPaths = uniqueSortedStrings(selectedPaths)
	if err = normalizePatchAssociationNulls(entity, document, associationFields, selectedPaths); err != nil {
		return zero, fmt.Errorf("failed to normalize patched associations: %w", err)
	}

	entity, err = r.repository.Update(tx, entity, builderPatchWriteFromTags(r.tags.updatePreloadPaths, selectedPaths, r.patchAutoSyncPaths))
	if err != nil {
		return zero, fmt.Errorf("failed to update entity with id %v after patch: %w", input.Id, err)
	}

	result, err := output(ctx, entity)
	if err != nil {
		return zero, fmt.Errorf("failed to transform patched entity: %w", err)
	}

	return result, nil
}

func (r *resource[K, E]) buildListOperation[LI ListInputSource, O any](
	custom TxOperation[LI, ListOutput[O]],
	visibility DeleteScope,
	query ListQuery[K, E, LI],
	count ListCount[K, E, LI],
	output func(context.Context, *E) (O, error),
) (TxOperation[LI, ListOutput[O]], error) {
	if custom != nil {
		return custom, nil
	}
	if output == nil {
		return nil, fmt.Errorf("CRUD output mapper is required")
	}

	return func(ctx context.Context, tx sqlr.TTx, input *LI) (ListOutput[O], error) {
		return r.list(ctx, tx, input, visibility, query, count, output)
	}, nil
}

func (r *resource[K, E]) list[LI ListInputSource, O any](
	ctx context.Context,
	tx sqlr.TTx,
	input *LI,
	visibility DeleteScope,
	query ListQuery[K, E, LI],
	count ListCount[K, E, LI],
	output func(context.Context, *E) (O, error),
) (ListOutput[O], error) {
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
		ApplyBuilder: r.builderQuery,
		ApplyScope: func(qb *sqlr.QueryBuilderSelect) error {
			if visibility != nil {
				visibility(qb)
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
	if query != nil {
		entities, err = query(ctx, tx, r.repository, input, plan)
	} else {
		entities, err = r.repository.Query(tx, func(qb *sqlr.QueryBuilderSelect) {
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
	if count != nil {
		total, err = count(ctx, tx, r.repository, input, plan)
	} else {
		total, err = r.count(tx, plan)
	}
	if err != nil {
		return ListOutput[O]{}, fmt.Errorf("failed to count entities: %w", err)
	}

	results := make([]O, len(entities))
	for i := range entities {
		if results[i], err = output(ctx, &entities[i]); err != nil {
			return ListOutput[O]{}, fmt.Errorf("failed to transform list entity at index %d: %w", i, err)
		}
	}

	return ListOutput[O]{Results: results, Total: total}, nil
}

func (r *resource[K, E]) buildDeleteOperation[Id sqlr.KeyTypes](
	custom TxOperation[InputById[Id], httpserver.Response],
	identity IdentityLookup[Id, K, E],
	visibility DeleteScope,
	deleteStrategy DeleteStrategy[K, E],
) TxOperation[InputById[Id], httpserver.Response] {
	if custom != nil {
		return custom
	}

	return func(ctx context.Context, tx sqlr.TTx, input *InputById[Id]) (httpserver.Response, error) {
		if _, err := r.deleteEntity(ctx, tx, input, identity, visibility, deleteStrategy); err != nil {
			return nil, err
		}

		return httpserver.NewStatusResponse(http.StatusNoContent), nil
	}
}

func (r *resource[K, E]) buildDeleteTypedOperation[Id sqlr.KeyTypes, O any](
	custom TxOperation[InputById[Id], O],
	identity IdentityLookup[Id, K, E],
	visibility DeleteScope,
	deleteStrategy DeleteStrategy[K, E],
	output func(context.Context, *E) (O, error),
) TxOperation[InputById[Id], O] {
	if custom != nil {
		return custom
	}

	return func(ctx context.Context, tx sqlr.TTx, input *InputById[Id]) (O, error) {
		var zero O

		entity, err := r.deleteEntity(ctx, tx, input, identity, visibility, deleteStrategy)
		if err != nil {
			return zero, err
		}
		if output == nil {
			return zero, fmt.Errorf("CRUD output mapper is required for typed delete")
		}

		result, err := output(ctx, entity)
		if err != nil {
			return zero, fmt.Errorf("failed to transform deleted entity: %w", err)
		}

		return result, nil
	}
}

func (r *resource[K, E]) count(tx sqlr.TTx, plan QueryPlan) (int, error) {
	qb := sqlr.NewQueryBuilderSelect()
	plan.ApplyBuilder(qb)
	if err := plan.ApplyScope(qb); err != nil {
		return 0, err
	}

	return r.repository.Count(tx, qb)
}

func (r *resource[K, E]) deleteEntity[Id sqlr.KeyTypes](
	ctx context.Context,
	tx sqlr.TTx,
	input *InputById[Id],
	identity IdentityLookup[Id, K, E],
	visibility DeleteScope,
	deleteStrategy DeleteStrategy[K, E],
) (*E, error) {
	if input == nil {
		return nil, fmt.Errorf("delete input is required")
	}

	deleteBuilder := composeBuilders(r.builderRead, builderForUpdate)
	entity, err := r.lookup(ctx, tx, identity, input.Id, resourceLookupScope(input, visibility), deleteBuilder)
	if err != nil {
		return nil, fmt.Errorf("failed to find entity before delete with id %v: %w", input.Id, err)
	}

	if deleteStrategy != nil {
		if err = deleteStrategy(ctx, tx, r.repository, entity); err != nil {
			return nil, fmt.Errorf("failed to delete entity with id %v: %w", input.Id, err)
		}
	} else if err = r.repository.Delete(tx, (*entity).GetId(), r.builderDelete); err != nil {
		return nil, fmt.Errorf("failed to delete entity with id %v: %w", input.Id, err)
	}

	return entity, nil
}

func (r *resource[K, E]) lookup[Id sqlr.KeyTypes](
	ctx context.Context,
	tx sqlr.TTx,
	identity IdentityLookup[Id, K, E],
	id Id,
	scope QueryScope,
	builder func(*sqlr.QueryBuilderSelect),
) (*E, error) {
	if identity != nil {
		return identity(ctx, tx, r.repository, id, scope, builder)
	}

	var queryErr error
	entities, err := r.repository.Query(tx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Where(sqlc.Col(r.schema.TableName, r.schema.PrimaryKey.Name).Eq(id))
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

func (r *resource[K, E]) close() error {
	if r == nil || r.repository == nil {
		return nil
	}

	return r.repository.Close()
}

func resourceLookupScope(source ForceFilterSource, visibility DeleteScope) QueryScope {
	return composeScopes(
		deleteScope(visibility),
		forceScope(source),
	)
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
