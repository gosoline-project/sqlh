package sqlh

import (
	"context"

	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

type TransformerFactory[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any, O any] func(ctx context.Context, config cfg.Config, logger log.Logger) (Transformer[K, E, IC, IU, O], error)

func SimpleTransformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any, O any](transformer Transformer[K, E, IC, IU, O]) TransformerFactory[K, E, IC, IU, O] {
	return func(ctx context.Context, config cfg.Config, logger log.Logger) (Transformer[K, E, IC, IU, O], error) {
		return transformer, nil
	}
}

type Transformer[K sqlr.KeyTypes, E sqlr.Entitier[K], IC any, IU any, O any] interface {
	TransformCreate(ctx context.Context, input *IC) (*E, error)
	TransformUpdate(ctx context.Context, entity *E, input *IU) (*E, error)
}

type TransformerOutput[K sqlr.KeyTypes, E sqlr.Entitier[K], O any] interface {
	TransformOutput(ctx context.Context, entity *E) (O, error)
}
