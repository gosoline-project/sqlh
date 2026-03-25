package sqlh

import (
	"context"

	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

// Transformer converts between HTTP input DTOs and database entities, and
// renders HTTP responses from entities. It is the central extension point for
// [HandlerCrud]: implement this interface to control how incoming request
// bodies are mapped to entities and how entities are serialised in responses.
//
// Type parameters:
//   - K: the primary key type (must satisfy [sqlr.KeyTypes]).
//   - E: the entity type (must implement [sqlr.Entitier][K]).
//   - IC: the create-input DTO type.
//   - IU: the update-input DTO type.
type Transformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any] interface {
	// TransformCreateInput converts a create-input DTO into a new entity that
	// can be persisted by the repository.
	TransformCreateInput(ctx context.Context, input *IC) (*E, error)
	// TransformUpdateInput merges an update-input DTO into an existing entity
	// and returns the updated entity ready for persistence.
	TransformUpdateInput(ctx context.Context, entity *E, input *IU) (*E, error)
	// RenderEntityResponse serialises a single entity into an HTTP response.
	RenderEntityResponse(ctx context.Context, entity *E) (httpserver.Response, error)
	// RenderQueryResponse serialises a slice of entities into an HTTP response.
	RenderQueryResponse(ctx context.Context, entity []E) (httpserver.Response, error)
}

// BuilderCreateAware augments the create builder used for
// [sqlr.Repository.Create].
type BuilderCreateAware interface {
	BuilderCreate(qb *sqlr.QueryBuilderCreate)
}

// BuilderReadAware augments the read builder used for single-entity reads.
type BuilderReadAware interface {
	BuilderRead(qb *sqlr.QueryBuilderRead)
}

// BuilderQueryAware augments the select builder used for query/list requests.
type BuilderQueryAware interface {
	BuilderQuery(qb *sqlr.QueryBuilderSelect)
}

// BuilderDeleteAware augments the delete builder used for
// [sqlr.Repository.Delete].
type BuilderDeleteAware interface {
	BuilderDelete(qb *sqlr.QueryBuilderDelete)
}

// BuilderUpdateReadAware augments the read builder used to load the existing
// entity before [Transformer.TransformUpdateInput] runs.
type BuilderUpdateReadAware interface {
	BuilderUpdateRead(qb *sqlr.QueryBuilderRead)
}

// BuilderUpdateWriteAware augments the update builder used for
// [sqlr.Repository.Update], including association sync and any post-update
// preloads used to rehydrate the response entity.
type BuilderUpdateWriteAware interface {
	BuilderUpdateWrite(qb *sqlr.QueryBuilderUpdate)
}

// TransformerFactory is a constructor function for a [Transformer]. It follows
// the standard gosoline factory pattern, receiving the application context,
// configuration, and logger so that the transformer can perform any necessary
// setup (e.g. loading config values or creating dependencies) at startup.
type TransformerFactory[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any] func(ctx context.Context, config cfg.Config, logger log.Logger) (Transformer[K, E, IC, IU], error)

// SimpleTransformer wraps an already-constructed [Transformer] into a
// [TransformerFactory]. This is useful when the transformer requires no
// configuration or lazy initialisation and can be created before the factory
// is called.
func SimpleTransformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any](transformer Transformer[K, E, IC, IU]) TransformerFactory[K, E, IC, IU] {
	return func(ctx context.Context, config cfg.Config, logger log.Logger) (Transformer[K, E, IC, IU], error) {
		return transformer, nil
	}
}
