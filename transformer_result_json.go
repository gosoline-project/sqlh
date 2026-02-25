package sqlh

import (
	"context"

	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

// JsonResultsTransformer is a simplified variant of [Transformer] for use with
// [NewJSONResultsTransformer]. Instead of producing full [httpserver.Response]
// values, implementations only need to supply TransformCreateInput,
// TransformUpdateInput, and a single TransformOutput method that converts one
// entity to any JSON-serialisable value. The JSON wrapping of single and
// multi-entity responses is handled automatically by the wrapper created by
// [NewJSONResultsTransformer].
//
// Type parameters:
//   - K: the primary key type (must satisfy [sqlr.KeyTypes]).
//   - E: the entity type (must implement [sqlr.Entitier][K]).
//   - IC: the create-input DTO type.
//   - IU: the update-input DTO type.
type JsonResultsTransformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any] interface {
	// TransformCreateInput converts a create-input DTO into a new entity that
	// can be persisted by the repository.
	TransformCreateInput(ctx context.Context, input *IC) (*E, error)
	// TransformUpdateInput merges an update-input DTO into an existing entity
	// and returns the updated entity ready for persistence.
	TransformUpdateInput(ctx context.Context, entity *E, input *IU) (*E, error)
	// TransformOutput converts a single entity into a JSON-serialisable value
	// that will be written as the HTTP response body.
	TransformOutput(ctx context.Context, entity *E) (any, error)
}

type resultsTransformerWrapper[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any] struct {
	transformer JsonResultsTransformer[K, E, IC, IU]
}

func (r resultsTransformerWrapper[K, E, IC, IU]) TransformCreateInput(ctx context.Context, input *IC) (*E, error) {
	return r.transformer.TransformCreateInput(ctx, input)
}

func (r resultsTransformerWrapper[K, E, IC, IU]) TransformUpdateInput(ctx context.Context, entity *E, input *IU) (*E, error) {
	return r.transformer.TransformUpdateInput(ctx, entity, input)
}

func (r resultsTransformerWrapper[K, E, IC, IU]) RenderEntityResponse(ctx context.Context, entity *E) (httpserver.Response, error) {
	var err error
	var output any

	if output, err = r.transformer.TransformOutput(ctx, entity); err != nil {
		return nil, err
	}

	return httpserver.NewJsonResponse(output), nil
}

func (r resultsTransformerWrapper[K, E, IC, IU]) RenderQueryResponse(ctx context.Context, entities []E) (httpserver.Response, error) {
	var err error
	output := make([]any, len(entities))

	for i, entity := range entities {
		if output[i], err = r.transformer.TransformOutput(ctx, &entity); err != nil {
			return nil, err
		}
	}

	return httpserver.NewJsonResponse(output), nil
}

// NewJSONResultsTransformer wraps a [JsonResultsTransformer] into a
// [TransformerFactory] that satisfies the full [Transformer] interface. The
// wrapper implements [Transformer.RenderEntityResponse] and
// [Transformer.RenderQueryResponse] by calling TransformOutput on each entity
// and encoding the result as a JSON HTTP response.
func NewJSONResultsTransformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any](transformer JsonResultsTransformer[K, E, IC, IU]) TransformerFactory[K, E, IC, IU] {
	return func(ctx context.Context, config cfg.Config, logger log.Logger) (Transformer[K, E, IC, IU], error) {
		return &resultsTransformerWrapper[K, E, IC, IU]{
			transformer: transformer,
		}, nil
	}
}
