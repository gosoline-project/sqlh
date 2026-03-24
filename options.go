package sqlh

import (
	"context"

	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

// Option configures how [NewHandlerCrud] and [WithCrudHandlers] construct a
// [HandlerCrud].
type Option[K sqlr.KeyTypes, E sqlr.Entitier[K]] func(opts *handlerOptions[K, E])

// RepositoryFactory creates the [sqlr.Repository] used by a [HandlerCrud].
//
// name is the configured SQL client name. It defaults to "default" and can be
// overridden with [WithClientName].
type RepositoryFactory[K sqlr.KeyTypes, E sqlr.Entitier[K]] func(ctx context.Context, config cfg.Config, logger log.Logger, name string) (sqlr.Repository[K, E], error)

type handlerOptions[K sqlr.KeyTypes, E sqlr.Entitier[K]] struct {
	clientName        string
	repositoryFactory RepositoryFactory[K, E]
}

func newOpts[K sqlr.KeyTypes, E sqlr.Entitier[K]]() *handlerOptions[K, E] {
	return &handlerOptions[K, E]{
		clientName:        "default",
		repositoryFactory: sqlr.NewRepository[K, E],
	}
}

// WithClientName configures the SQL client name used when creating the default
// repository for a [HandlerCrud].
func WithClientName[K sqlr.KeyTypes, E sqlr.Entitier[K]](name string) Option[K, E] {
	return func(opts *handlerOptions[K, E]) {
		opts.clientName = name
	}
}

// WithRepositoryFactory overrides the repository constructor used by
// [NewHandlerCrud] and [WithCrudHandlers].
func WithRepositoryFactory[K sqlr.KeyTypes, E sqlr.Entitier[K]](factory RepositoryFactory[K, E]) Option[K, E] {
	return func(opts *handlerOptions[K, E]) {
		opts.repositoryFactory = factory
	}
}
