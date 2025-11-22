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
	"github.com/spf13/cast"
)

type Bla interface {
	GetId() int
}

type E struct {
	id int
}

func (e *E) GetId() int {
	return e.id
}

func Hust[T Bla](t T) {
	fmt.Println(t.GetId())
}

func init() {
	e := &E{id: 42}
	Hust[*E](e)
}

type InputRead[K sqlr.KeyTypes] struct {
	Id K `uri:"id"`
}

type InputQuery[K sqlr.KeyTypes] struct {
	Filter sqlc.JsonFilter `json:"filter"`
}

func WithCrudHandlers[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any](version int, entityName string, transformerFactory TransformerFactory[K, E, IC, IU]) httpserver.RegisterFactoryFunc {
	return httpserver.With(NewHandlerCrud[K, E, IC, IU](transformerFactory), func(router *httpserver.Router, handler *HandlerCrud[K, E, IC, IU]) {
		path := fmt.Sprintf("/v%d/%s", version, entityName)
		router.POST(path, httpserver.Bind(handler.HandleCreate))

		idPath := fmt.Sprintf("%s/:id", path)
		router.GET(idPath, httpserver.Bind(handler.HandleRead))
		router.DELETE(idPath, httpserver.Bind(handler.HandleDelete))
		router.PUT(idPath, func(c *gin.Context) {
			httpserver.Bind(func(ctx context.Context, input *IU) (httpserver.Response, error) {
				var err error
				var id K

				if id, err = cast.ToE[K](c.Param("id")); err != nil {
					return nil, fmt.Errorf("failed to cast id param to correct type: %w", err)
				}

				return handler.HandleUpdate(ctx, id, input)
			})(c)
		})

		plural := inflection.Plural(entityName)
		queryPath := fmt.Sprintf("/v%d/%s", version, plural)
		router.POST(queryPath, httpserver.Bind(handler.HandleQuery))
	})
}

func NewHandlerCrud[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any](transformerFactory TransformerFactory[K, E, IC, IU]) httpserver.HandlerFactory[HandlerCrud[K, E, IC, IU]] {
	return func(ctx context.Context, config cfg.Config, logger log.Logger) (*HandlerCrud[K, E, IC, IU], error) {
		var err error
		var repo sqlr.Repository[K, E]
		var transformer Transformer[K, E, IC, IU]

		if repo, err = sqlr.NewRepository[K, E](ctx, config, logger, "default"); err != nil {
			return nil, fmt.Errorf("failed to create repository for handler: %w", err)
		}

		if transformer, err = transformerFactory(ctx, config, logger); err != nil {
			return nil, fmt.Errorf("failed to create transformer for handler: %w", err)
		}

		return &HandlerCrud[K, E, IC, IU]{
			repo:        repo,
			transformer: transformer,
		}, nil
	}
}

type HandlerCrud[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any] struct {
	repo        sqlr.Repository[K, E]
	transformer Transformer[K, E, IC, IU]
}

func (h *HandlerCrud[K, E, IC, IU]) HandleCreate(ctx context.Context, input *IC) (httpserver.Response, error) {
	var err error
	var entity *E

	if entity, err = h.transformer.TransformCreate(ctx, input); err != nil {
		return nil, fmt.Errorf("failed to transform create input: %w", err)
	}

	if err = h.repo.Create(ctx, entity); err != nil {
		return nil, fmt.Errorf("failed to create entity: %w", err)
	}

	return h.out(ctx, entity)
}

func (h *HandlerCrud[K, E, IC, IU]) HandleRead(ctx context.Context, input *InputRead[K]) (httpserver.Response, error) {
	var err error
	var entity *E

	if entity, err = h.repo.Read(ctx, input.Id); err != nil {
		return nil, fmt.Errorf("failed to read entity with id %q: %w", input.Id, err)
	}

	return h.out(ctx, entity)
}

func (h *HandlerCrud[K, E, IC, IU]) HandleQuery(ctx context.Context, input *InputQuery[K]) (httpserver.Response, error) {
	var err error
	var entities []E
	var expression *sqlc.Expression

	if expression, err = input.Filter.ToExpression(); err != nil {
		return nil, fmt.Errorf("failed to transform filter to expression: %w", err)
	}

	qb := sqlr.NewQueryBuilderSelect().
		Where(expression)

	if entities, err = h.repo.Query(ctx, qb); err != nil {
		return nil, fmt.Errorf("failed to query entities: %w", err)
	}

	return httpserver.NewJsonResponse(entities), nil
}

func (h *HandlerCrud[K, E, IC, IU]) HandleUpdate(ctx context.Context, id K, input *IU) (httpserver.Response, error) {
	var err error
	var entity *E

	if entity, err = h.repo.Read(ctx, id); err != nil {
		return nil, fmt.Errorf("failed to read entity with id %q: %w", id, err)
	}

	if entity, err = h.transformer.TransformUpdate(ctx, entity, input); err != nil {
		return nil, fmt.Errorf("failed to transform update input: %w", err)
	}

	if entity, err = h.repo.Update(ctx, entity); err != nil {
		return nil, fmt.Errorf("failed to update entity with id %q: %w", id, err)
	}

	return h.out(ctx, entity)
}

func (h *HandlerCrud[K, E, IC, IU]) HandleDelete(ctx context.Context, input *InputRead[K]) (httpserver.Response, error) {
	if err := h.repo.Delete(ctx, input.Id); err != nil {
		return nil, fmt.Errorf("failed to delete entity with id %q: %w", input.Id, err)
	}

	return httpserver.NewStatusResponse(http.StatusOK), nil
}

func (h *HandlerCrud[K, E, IC, IU]) out(ctx context.Context, entity *E) (httpserver.Response, error) {
	var ok bool
	var err error
	var outTransformer TransformerOutput[K, E]
	var out any

	if outTransformer, ok = h.transformer.(TransformerOutput[K, E]); !ok {
		return httpserver.NewJsonResponse(entity), nil
	}

	if out, err = outTransformer.TransformOutput(ctx, entity); err != nil {
		return nil, fmt.Errorf("failed to transform output: %w", err)
	}

	return httpserver.NewJsonResponse(out), nil
}
