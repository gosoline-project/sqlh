package sqlh

import (
	"context"

	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

// Transformer maps HTTP DTOs to entities and entities to typed HTTP output.
// The output is deliberately an ordinary Go value so httpserver.Bind can
// negotiate its representation from the request's Accept header.
type Transformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU Identified[K], O any] interface {
	TransformCreateInput(ctx context.Context, input *IC) (*E, error)
	TransformUpdateInput(ctx context.Context, entity *E, input *IU) (*E, error)
	TransformOutput(ctx context.Context, entity *E) (O, error)
}

// PatchTransformer optionally adds JSON Merge Patch support to a CRUD
// transformer. The patch target is a complete update representation. SQLH
// applies the request document to that target before calling TransformPatch.
type PatchTransformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IU Identified[K]] interface {
	TransformPatchTarget(ctx context.Context, entity *E) (*IU, error)
	TransformPatch(ctx context.Context, entity *E, target *IU) (*E, error)
}

// TransformerFactory constructs a Transformer during application startup.
type TransformerFactory[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU Identified[K], O any] func(ctx context.Context, config cfg.Config, logger log.Logger) (Transformer[K, E, IC, IU, O], error)

// BuilderCreateAware augments the SQLR create builder used by CRUD create
// operations.
type BuilderCreateAware interface {
	BuilderCreate(qb *sqlr.QueryBuilderCreate)
}

// BuilderReadAware augments the select builder used to look up one entity.
type BuilderReadAware interface {
	BuilderRead(qb *sqlr.QueryBuilderSelect)
}

// BuilderQueryAware augments the select builder used by list operations.
type BuilderQueryAware interface {
	BuilderQuery(qb *sqlr.QueryBuilderSelect)
}

// BuilderDeleteAware augments the SQLR delete builder used by physical deletes.
type BuilderDeleteAware interface {
	BuilderDelete(qb *sqlr.QueryBuilderDelete)
}

// BuilderUpdateReadAware augments the select builder used to load an entity
// before an update.
type BuilderUpdateReadAware interface {
	BuilderUpdateRead(qb *sqlr.QueryBuilderSelect)
}

// BuilderUpdateWriteAware augments the SQLR update builder used to persist an
// updated entity.
type BuilderUpdateWriteAware interface {
	BuilderUpdateWrite(qb *sqlr.QueryBuilderUpdate)
}

// NewCrudDefinitionFromTransformer creates a static CRUD definition from a
// transformer. Relation builder-aware interfaces are copied into the
// definition when the transformer implements them.
func NewCrudDefinitionFromTransformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU Identified[K], O any](transformer Transformer[K, E, IC, IU, O]) CrudDefinition[K, E, K, IC, IU, ListInput, O] {
	definition := CrudDefinition[K, E, K, IC, IU, ListInput, O]{
		CreateInput: transformer.TransformCreateInput,
		UpdateInput: transformer.TransformUpdateInput,
		Output:      transformer.TransformOutput,
	}

	if builder, ok := transformer.(BuilderCreateAware); ok {
		definition.BuilderCreate = builder.BuilderCreate
	}
	if builder, ok := transformer.(BuilderReadAware); ok {
		definition.BuilderRead = builder.BuilderRead
	}
	if builder, ok := transformer.(BuilderQueryAware); ok {
		definition.BuilderQuery = builder.BuilderQuery
	}
	if builder, ok := transformer.(BuilderDeleteAware); ok {
		definition.BuilderDelete = builder.BuilderDelete
	}
	if builder, ok := transformer.(BuilderUpdateReadAware); ok {
		definition.BuilderUpdateRead = builder.BuilderUpdateRead
	}
	if builder, ok := transformer.(BuilderUpdateWriteAware); ok {
		definition.BuilderUpdateWrite = builder.BuilderUpdateWrite
	}
	if patchTransformer, ok := transformer.(PatchTransformer[K, E, IU]); ok {
		definition.PatchTarget = patchTransformer.TransformPatchTarget
		definition.PatchApply = patchTransformer.TransformPatch
	}

	return definition
}

// SimpleTransformer wraps an already-constructed transformer in a static CRUD
// definition factory. Use a custom CrudDefinition when the default repository
// operations need to be replaced.
func SimpleTransformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU Identified[K], O any](transformer Transformer[K, E, IC, IU, O]) CrudDefinitionFactory[K, E, K, IC, IU, ListInput, O] {
	return SimpleCrudDefinition(NewCrudDefinitionFromTransformer(transformer))
}
