package sqlh

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/jinzhu/inflection"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

// InputByID is a URI-binding struct used to capture the `:id` path parameter
// in read, update, and delete routes.
type InputByID[K sqlr.KeyTypes] struct {
	// ID is the entity's primary key, decoded from the `:id` URL path segment.
	ID K `uri:"id"`
}

// InputQuery is the request body struct for the query (list) endpoint. It
// carries a JSON filter expression that is translated into a SQL WHERE clause.
type InputQuery struct {
	// Filter holds the JSON filter expression that is translated into a SQL
	// WHERE clause for the query.
	Filter sqlc.JsonFilter `json:"filter"`
}

// WithCrudHandlers registers a complete set of CRUD HTTP routes for an entity
// type onto an httpserver router. The following routes are created:
//
//	POST   /v{version}/{entityName}         – create a new entity
//	GET    /v{version}/{entityName}/:id     – read a single entity by ID
//	PUT    /v{version}/{entityName}/:id     – update an existing entity by ID
//	DELETE /v{version}/{entityName}/:id     – delete an entity by ID
//	POST   /v{version}/{entityNamePlural}   – query entities with a JSON filter
//
// entityName is used verbatim for the singular routes; the plural form is
// derived automatically via github.com/jinzhu/inflection.
//
// transformerFactory is called once at startup to produce the [Transformer]
// used by all handlers to convert between HTTP DTOs and database entities.
//
// The returned [httpserver.RegisterFactoryFunc] is suitable for passing
// directly to an httpserver setup function, optionally combined with [WithTx]
// to run each request inside a database transaction.
func WithCrudHandlers[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any](version int, entityName string, transformerFactory TransformerFactory[K, E, IC, IU]) httpserver.RegisterFactoryFunc {
	return httpserver.With(NewHandlerCrud(transformerFactory), func(router *httpserver.Router, handler *HandlerCrud[K, E, IC, IU]) {
		path := fmt.Sprintf("/v%d/%s", version, entityName)
		router.POST(path, httpserver.Bind(handler.HandleCreate))

		idPath := fmt.Sprintf("%s/:id", path)
		router.GET(idPath, httpserver.Bind(handler.HandleRead))
		router.DELETE(idPath, httpserver.Bind(handler.HandleDelete))
		router.PUT(idPath, func(ginCtx *gin.Context) {
			httpserver.Bind(func(ctx context.Context, input *IU) (httpserver.Response, error) {
				var err error
				var id K

				if id, err = parseKeyFromString[K](ginCtx.Param("id")); err != nil {
					return nil, fmt.Errorf("failed to cast id param to correct type: %w", err)
				}

				return handler.HandleUpdate(ctx, id, input)
			})(ginCtx)
		})

		plural := inflection.Plural(entityName)
		queryPath := fmt.Sprintf("/v%d/%s", version, plural)
		router.POST(queryPath, httpserver.Bind(handler.HandleQuery))
	})
}

// NewHandlerCrud returns an [httpserver.HandlerFactory] that creates a
// [HandlerCrud] at server startup. It initialises a [sqlr.Repository] against
// the "default" SQL connection and builds the [Transformer] via
// transformerFactory.
func NewHandlerCrud[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any](transformerFactory TransformerFactory[K, E, IC, IU]) httpserver.HandlerFactory[HandlerCrud[K, E, IC, IU]] {
	return func(ctx context.Context, config cfg.Config, logger log.Logger) (*HandlerCrud[K, E, IC, IU], error) {
		var err error
		var repo sqlr.Repository[K, E]
		var transformer Transformer[K, E, IC, IU]
		var entityTags *entityBuilderTags

		if repo, err = sqlr.NewRepository[K, E](ctx, config, logger, "default"); err != nil {
			return nil, fmt.Errorf("failed to create repository for handler: %w", err)
		}

		if entityTags, err = parseEntityBuilderTags[E](); err != nil {
			return nil, fmt.Errorf("failed to parse entity %T %s tags: %w", *new(E), sqlhTagName, err)
		}

		if transformer, err = transformerFactory(ctx, config, logger); err != nil {
			return nil, fmt.Errorf("failed to create transformer for handler: %w", err)
		}

		handler := &HandlerCrud[K, E, IC, IU]{
			repo:               repo,
			transformer:        transformer,
			builderCreate:      composeBuilders(builderCreateFromTags(entityTags)),
			builderRead:        composeBuilders(builderReadFromTags(entityTags)),
			builderQuery:       composeBuilders(builderQueryFromTags(entityTags)),
			builderUpdateRead:  composeBuilders(builderUpdateReadFromTags(entityTags)),
			builderUpdateWrite: composeBuilders(builderUpdateWriteFromTags(entityTags)),
		}

		if builder, ok := transformer.(BuilderCreateAware); ok {
			handler.builderCreate = composeBuilders(builderCreateFromTags(entityTags), builder.BuilderCreate)
		}

		if builder, ok := transformer.(BuilderReadAware); ok {
			handler.builderRead = composeBuilders(builderReadFromTags(entityTags), builder.BuilderRead)
		}

		if builder, ok := transformer.(BuilderQueryAware); ok {
			handler.builderQuery = composeBuilders(builderQueryFromTags(entityTags), builder.BuilderQuery)
		}

		if builder, ok := transformer.(BuilderUpdateReadAware); ok {
			handler.builderUpdateRead = composeBuilders(builderUpdateReadFromTags(entityTags), builder.BuilderUpdateRead)
		}

		if builder, ok := transformer.(BuilderUpdateWriteAware); ok {
			handler.builderUpdateWrite = composeBuilders(builderUpdateWriteFromTags(entityTags), builder.BuilderUpdateWrite)
		}

		return handler, nil
	}
}

// HandlerCrud is a generic HTTP handler that implements Create, Read, Update,
// Delete, and Query operations for an entity type E with primary key type K.
// It delegates persistence to a [sqlr.Repository] and DTO conversion to a
// [Transformer].
//
// Use [WithCrudHandlers] to register all routes in one call, or call the
// individual Handle* methods to attach only the routes you need.
type HandlerCrud[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any] struct {
	repo               sqlr.Repository[K, E]
	transformer        Transformer[K, E, IC, IU]
	builderCreate      func(qb *sqlr.QueryBuilderCreate)
	builderRead        func(qb *sqlr.QueryBuilderRead)
	builderQuery       func(qb *sqlr.QueryBuilderSelect)
	builderUpdateRead  func(qb *sqlr.QueryBuilderRead)
	builderUpdateWrite func(qb *sqlr.QueryBuilderUpdate)
}

// HandleCreate handles a create request. It transforms the input DTO into an
// entity via [Transformer.TransformCreateInput], persists it using the
// repository, and returns the created entity as an HTTP response via
// [Transformer.RenderEntityResponse].
func (h *HandlerCrud[K, E, IC, IU]) HandleCreate(ctx context.Context, input *IC) (httpserver.Response, error) {
	var err error
	var entity *E

	if entity, err = h.transformer.TransformCreateInput(ctx, input); err != nil {
		return nil, fmt.Errorf("failed to transform create input: %w", err)
	}

	if err = h.repo.Create(ctx, entity, h.builderCreate); err != nil {
		return nil, fmt.Errorf("failed to create entity: %w", err)
	}

	return h.transformer.RenderEntityResponse(ctx, entity)
}

// HandleRead handles a read request. It fetches the entity identified by
// input.ID from the repository and returns it as an HTTP response via
// [Transformer.RenderEntityResponse].
func (h *HandlerCrud[K, E, IC, IU]) HandleRead(ctx context.Context, input *InputByID[K]) (httpserver.Response, error) {
	var err error
	var entity *E

	if entity, err = h.repo.Read(ctx, input.ID, h.builderRead); err != nil {
		return nil, fmt.Errorf("failed to read entity with id %v: %w", input.ID, err)
	}

	return h.transformer.RenderEntityResponse(ctx, entity)
}

// HandleQuery handles a query request. It converts input.Filter into a SQL
// expression, queries the repository for matching entities, and returns the
// result as an HTTP response via [Transformer.RenderQueryResponse].
func (h *HandlerCrud[K, E, IC, IU]) HandleQuery(ctx context.Context, input *InputQuery) (httpserver.Response, error) {
	var err error
	var entities []E
	var expression *sqlc.Expression

	if expression, err = input.Filter.ToExpression(); err != nil {
		return nil, fmt.Errorf("failed to transform filter to expression: %w", err)
	}

	if entities, err = h.repo.Query(ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Where(expression)
		h.builderQuery(qb)
	}); err != nil {
		return nil, fmt.Errorf("failed to query entities: %w", err)
	}

	return h.transformer.RenderQueryResponse(ctx, entities)
}

// HandleUpdate handles an update request. It reads the existing entity for id,
// merges the input DTO via [Transformer.TransformUpdateInput], persists the
// result, and returns the updated entity as an HTTP response via
// [Transformer.RenderEntityResponse].
func (h *HandlerCrud[K, E, IC, IU]) HandleUpdate(ctx context.Context, id K, input *IU) (httpserver.Response, error) {
	var err error
	var entity *E

	if entity, err = h.repo.Read(ctx, id, h.builderUpdateRead); err != nil {
		return nil, fmt.Errorf("failed to read entity with id %v: %w", id, err)
	}

	if entity, err = h.transformer.TransformUpdateInput(ctx, entity, input); err != nil {
		return nil, fmt.Errorf("failed to transform update input: %w", err)
	}

	if entity, err = h.repo.Update(ctx, entity, h.builderUpdateWrite); err != nil {
		return nil, fmt.Errorf("failed to update entity with id %v: %w", id, err)
	}

	return h.transformer.RenderEntityResponse(ctx, entity)
}

// HandleDelete handles a delete request. It removes the entity identified by
// input.ID from the repository and returns a 200 OK response on success.
func (h *HandlerCrud[K, E, IC, IU]) HandleDelete(ctx context.Context, input *InputByID[K]) (httpserver.Response, error) {
	if err := h.repo.Delete(ctx, input.ID); err != nil {
		return nil, fmt.Errorf("failed to delete entity with id %v: %w", input.ID, err)
	}

	return httpserver.NewStatusResponse(http.StatusNoContent), nil
}
