//go:build integration && fixtures

package test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	gosolinehttpserver "github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlh"
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
	"github.com/justtrackio/gosoline/pkg/test/suite"
)

type closeTrackedPostRepository struct {
	sqlr.RepositoryTx[int64, Post]
	closed chan struct{}
	once   sync.Once
}

func (r *closeTrackedPostRepository) Close() error {
	err := r.RepositoryTx.Close()
	r.once.Do(func() {
		close(r.closed)
	})

	return err
}

type CrudShutdownIntegrationTestSuite struct {
	suite.Suite

	repositoryClosed chan struct{}
}

func TestCrudShutdownIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CrudShutdownIntegrationTestSuite))
}

func (s *CrudShutdownIntegrationTestSuite) SetupSuite() []suite.Option {
	s.repositoryClosed = make(chan struct{})

	return []suite.Option{
		suite.WithConfigFile("config.yml"),
		suite.WithLogLevel("info"),
	}
}

func (s *CrudShutdownIntegrationTestSuite) SetupHttpServerRouter() gosolinehttpserver.RouterFactory {
	return func(_ context.Context, _ cfg.Config, _ log.Logger, router *gosolinehttpserver.Router) error {
		transformer := &PostTransformer{}
		definition := sqlh.NewCrudDefinition(
			transformer.TransformCreateInput,
			transformer.TransformUpdateInput,
			transformer.TransformPatchInputFromEntity,
			transformer.TransformOutput,
		)
		repositoryFactory := func(client sqlc.Client, settings sqlr.Settings) (sqlr.RepositoryTx[int64, Post], error) {
			repository, err := sqlr.NewRepositoryTxWithSettings[int64, Post](client, settings)
			if err != nil {
				return nil, err
			}

			return &closeTrackedPostRepository{
				RepositoryTx: repository,
				closed:       s.repositoryClosed,
			}, nil
		}

		router.HandleWith(sqlh.WithCrudHandlers(
			1,
			"post",
			sqlh.SimpleCrudDefinition(definition),
			sqlh.WithRepositoryTxFactory(repositoryFactory),
		))

		return nil
	}
}

func (s *CrudShutdownIntegrationTestSuite) TestServerShutdownClosesCrudRepository(app suite.AppUnderTest, _ *resty.Client) error {
	app.Stop()
	app.WaitDone()

	select {
	case <-s.repositoryClosed:
		return nil
	case <-time.After(time.Second):
		return fmt.Errorf("CRUD repository was not closed during HTTP server shutdown")
	}
}
