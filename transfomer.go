package sqlh

import (
	"context"

	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

type TransformerFactory[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any] func(ctx context.Context, config cfg.Config, logger log.Logger) (Transformer[K, E, IC, IU], error)

func SimpleTransformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any](transformer Transformer[K, E, IC, IU]) TransformerFactory[K, E, IC, IU] {
	return func(ctx context.Context, config cfg.Config, logger log.Logger) (Transformer[K, E, IC, IU], error) {
		return transformer, nil
	}
}

type Transformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any] interface {
	TransformCreate(ctx context.Context, input *IC) (*E, error)
	TransformUpdate(ctx context.Context, entity *E, input *IU) (*E, error)
}

type TransformerOutput[K sqlr.KeyTypes, E sqlr.Entitier[K]] interface {
	TransformOutput(ctx context.Context, entity *E) (any, error)
}
