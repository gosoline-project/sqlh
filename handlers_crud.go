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

type InputById[K sqlr.KeyTypes] struct {
	Id K `uri:"id"`
}

type InputQuery struct {
	Filter sqlc.JsonFilter `json:"filter"`
}

func WithCrudHandlers[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any, O any, LE any, LO any](version int, entityName string, transformerFactory TransformerFactory[K, E, IC, IU, O], formatter Formatter[E, O, LE, LO]) httpserver.RegisterFactoryFunc {
	return httpserver.With(NewHandlerCrud[K, E, IC, IU, O, LE, LO](transformerFactory, formatter), func(router *httpserver.Router, handler *HandlerCrud[K, E, IC, IU, O, LE, LO]) {
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

func NewHandlerCrud[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any, O any, LE any, LO any](transformerFactory TransformerFactory[K, E, IC, IU, O], formatter Formatter[E, O, LE, LO]) httpserver.HandlerFactory[HandlerCrud[K, E, IC, IU, O, LE, LO]] {
	return func(ctx context.Context, config cfg.Config, logger log.Logger) (*HandlerCrud[K, E, IC, IU, O, LE, LO], error) {
		var err error
		var repo sqlr.Repository[K, E]
		var transformer Transformer[K, E, IC, IU, O]

		if repo, err = sqlr.NewRepository[K, E](ctx, config, logger, "default"); err != nil {
			return nil, fmt.Errorf("failed to create repository for handler: %w", err)
		}

		if transformer, err = transformerFactory(ctx, config, logger); err != nil {
			return nil, fmt.Errorf("failed to create transformer for handler: %w", err)
		}

		return &HandlerCrud[K, E, IC, IU, O, LE, LO]{
			repo:        repo,
			transformer: transformer,
			formatter:   formatter,
		}, nil
	}
}

type HandlerCrud[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any, O any, LE any, LO any] struct {
	repo        sqlr.Repository[K, E]
	transformer Transformer[K, E, IC, IU, O]
	formatter   Formatter[E, O, LE, LO]
}

func (h *HandlerCrud[K, E, IC, IU, O, LE, LO]) HandleCreate(ctx context.Context, input *IC) (httpserver.Response, error) {
	var err error
	var entity *E

	if entity, err = h.transformer.TransformCreate(ctx, input); err != nil {
		return nil, fmt.Errorf("failed to transform create input: %w", err)
	}

	if err = h.repo.Create(ctx, entity); err != nil {
		return nil, fmt.Errorf("failed to create entity: %w", err)
	}

	return h.outSingle(ctx, entity)
}

func (h *HandlerCrud[K, E, IC, IU, O, LE, LO]) HandleRead(ctx context.Context, input *InputById[K]) (httpserver.Response, error) {
	var err error
	var entity *E

	if entity, err = h.repo.Read(ctx, input.Id); err != nil {
		return nil, fmt.Errorf("failed to read entity with id %v: %w", input.Id, err)
	}

	return h.outSingle(ctx, entity)
}

func (h *HandlerCrud[K, E, IC, IU, O, LE, LO]) HandleQuery(ctx context.Context, input *InputQuery) (httpserver.Response, error) {
	var err error
	var entities []E
	var expression *sqlc.Expression

	if expression, err = input.Filter.ToExpression(); err != nil {
		return nil, fmt.Errorf("failed to transform filter to expression: %w", err)
	}

	if entities, err = h.repo.Query(ctx, func(qb *sqlr.QueryBuilderSelect) {
		qb.Where(expression)
	}); err != nil {
		return nil, fmt.Errorf("failed to query entities: %w", err)
	}

	return h.outMultiple(ctx, entities)
}

func (h *HandlerCrud[K, E, IC, IU, O, LE, LO]) HandleUpdate(ctx context.Context, id K, input *IU) (httpserver.Response, error) {
	var err error
	var entity *E

	if entity, err = h.repo.Read(ctx, id); err != nil {
		return nil, fmt.Errorf("failed to read entity with id %v: %w", id, err)
	}

	if entity, err = h.transformer.TransformUpdate(ctx, entity, input); err != nil {
		return nil, fmt.Errorf("failed to transform update input: %w", err)
	}

	if entity, err = h.repo.Update(ctx, entity); err != nil {
		return nil, fmt.Errorf("failed to update entity with id %v: %w", id, err)
	}

	return h.outSingle(ctx, entity)
}

func (h *HandlerCrud[K, E, IC, IU, O, LE, LO]) HandleDelete(ctx context.Context, input *InputById[K]) (httpserver.Response, error) {
	if err := h.repo.Delete(ctx, input.Id); err != nil {
		return nil, fmt.Errorf("failed to delete entity with id %v: %w", input.Id, err)
	}

	return httpserver.NewStatusResponse(http.StatusOK), nil
}

func (h *HandlerCrud[K, E, IC, IU, O, LE, LO]) outSingle(ctx context.Context, entity *E) (httpserver.Response, error) {
	var ok bool
	var err error
	var outTransformer TransformerOutput[K, E, O]
	var out *O

	if outTransformer, ok = h.transformer.(TransformerOutput[K, E, O]); !ok {
		return httpserver.NewJsonResponse(entity), nil
	}

	if out, err = outTransformer.TransformOutput(ctx, entity); err != nil {
		return nil, fmt.Errorf("failed to transform output: %w", err)
	}

	return httpserver.NewJsonResponse(out), nil
}

func (h *HandlerCrud[K, E, IC, IU, O, LE, LO]) outMultiple(ctx context.Context, entities []E) (httpserver.Response, error) {
	var ok bool
	var err error
	var outTransformer TransformerOutput[K, E, O]
	if outTransformer, ok = h.transformer.(TransformerOutput[K, E, O]); !ok {
		es := make([]*E, len(entities))
		for idx, e := range entities {
			es[idx] = &e
		}

		var le LE
		if le, err = h.formatter.FormatEntityList(ctx, es); err != nil {
			return nil, fmt.Errorf("failed to format output: %w", err)
		}

		return httpserver.NewJsonResponse(le), nil
	}

	outs := make([]*O, len(entities))

	for idx, entity := range entities {
		if outs[idx], err = outTransformer.TransformOutput(ctx, &entity); err != nil {
			return nil, fmt.Errorf("failed to transform output: %w", err)
		}
	}

	var lo LO
	if lo, err = h.formatter.FormatOutputList(ctx, outs); err != nil {
		return nil, fmt.Errorf("failed to format output: %w", err)
	}

	return httpserver.NewJsonResponse(lo), nil
}
