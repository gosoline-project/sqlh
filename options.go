package sqlh

import (
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
)

// Option configures how [NewCRUD] and [WithCrudHandlers] create a CRUD handler.
type Option[K sqlr.KeyTypes, E sqlr.Entitier[K]] func(opts *handlerOptions[K, E])

// RepositoryTxFactory creates the transaction-aware SQLR repository used by a
// CRUD handler. The client is also used by SQLH to begin the transaction, so a
// custom factory must build repositories against that same client when
// prepared statements are enabled.
type RepositoryTxFactory[K sqlr.KeyTypes, E sqlr.Entitier[K]] func(client sqlc.Client, settings sqlr.Settings) (sqlr.RepositoryTx[K, E], error)

type handlerOptions[K sqlr.KeyTypes, E sqlr.Entitier[K]] struct {
	clientName         string
	repositorySettings sqlr.Settings
	repositoryFactory  RepositoryTxFactory[K, E]
}

func newOpts[K sqlr.KeyTypes, E sqlr.Entitier[K]]() *handlerOptions[K, E] {
	return &handlerOptions[K, E]{
		clientName:        "default",
		repositoryFactory: sqlr.NewRepositoryTxWithSettings[K, E],
	}
}

// WithClientName configures the named SQL client used by the CRUD handler.
func WithClientName[K sqlr.KeyTypes, E sqlr.Entitier[K]](name string) Option[K, E] {
	return func(opts *handlerOptions[K, E]) {
		opts.clientName = name
	}
}

// WithRepositoryTxFactory replaces the default SQLR transaction repository
// constructor. The factory receives the same client that SQLH uses to begin
// request transactions.
func WithRepositoryTxFactory[K sqlr.KeyTypes, E sqlr.Entitier[K]](factory RepositoryTxFactory[K, E]) Option[K, E] {
	return func(opts *handlerOptions[K, E]) {
		opts.repositoryFactory = factory
	}
}

// WithRepositorySettings configures SQLR repository settings, such as prepared
// statement caching.
func WithRepositorySettings[K sqlr.KeyTypes, E sqlr.Entitier[K]](settings sqlr.Settings) Option[K, E] {
	return func(opts *handlerOptions[K, E]) {
		opts.repositorySettings = settings
	}
}

// WithRepositoryTxSettings is an explicit alias for
// [WithRepositorySettings]. It makes the transaction-aware repository choice
// visible at call sites.
func WithRepositoryTxSettings[K sqlr.KeyTypes, E sqlr.Entitier[K]](settings sqlr.Settings) Option[K, E] {
	return WithRepositorySettings[K, E](settings)
}
