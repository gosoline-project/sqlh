//go:build integration && fixtures

package test

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	gosolinehttpserver "github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
	"github.com/justtrackio/gosoline/pkg/test/suite"
)

func TestCrudIntegrationTestSuite(t *testing.T) {
	suite.Run(t, new(CrudIntegrationTestSuite))
}

type CrudIntegrationTestSuite struct {
	suite.Suite

	ctx context.Context
}

func (s *CrudIntegrationTestSuite) SetupSuite() []suite.Option {
	return []suite.Option{
		suite.WithConfigFile("config.yml"),
		suite.WithLogLevel("info"),
		suite.WithFixtureSetFactory(Fixtures()),
	}
}

func (s *CrudIntegrationTestSuite) SetupTest() error {
	s.ctx = s.Env().Context()

	return nil
}

func (s *CrudIntegrationTestSuite) SetupHttpServerRouter() gosolinehttpserver.RouterFactory {
	return func(ctx context.Context, config cfg.Config, logger log.Logger, router *gosolinehttpserver.Router) error {
		router.HandleWith(NewPostCrud())
		router.HandleWith(NewMutationPreloadPostCrud())

		return nil
	}
}

func (s *CrudIntegrationTestSuite) TestReadPostPreloadsAssociations(app suite.AppUnderTest, client *resty.Client) error {
	defer app.WaitDone()
	defer app.Stop()

	var output PostOutput
	response, err := client.R().
		SetResult(&output).
		Execute(http.MethodGet, "/v1/post/1")
	if err != nil {
		return err
	}

	s.Equal(http.StatusOK, response.StatusCode())
	s.Equal(PostOutput{
		ID:       1,
		AuthorID: 1,
		Title:    "Getting Started with Go",
		Status:   "published",
		Author: &AuthorOutput{
			ID:   1,
			Name: "Alice Johnson",
		},
		Tags: []TagOutput{
			{ID: 1, Name: "golang"},
			{ID: 4, Name: "tutorial"},
		},
		CreatedAt: time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC),
	}, output)

	return nil
}

func (s *CrudIntegrationTestSuite) TestQueryPostPreloadsAssociations(app suite.AppUnderTest, client *resty.Client) error {
	defer app.WaitDone()
	defer app.Stop()

	var output []PostOutput
	response, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(map[string]any{
			"filter": map[string]any{
				"type":   "eq",
				"column": "title",
				"value":  "Advanced Go Patterns",
			},
		}).
		SetResult(&output).
		Execute(http.MethodPost, "/v1/posts")
	if err != nil {
		return err
	}

	s.Equal(http.StatusOK, response.StatusCode())
	s.Equal([]PostOutput{{
		ID:       2,
		AuthorID: 1,
		Title:    "Advanced Go Patterns",
		Status:   "published",
		Author: &AuthorOutput{
			ID:   1,
			Name: "Alice Johnson",
		},
		Tags: []TagOutput{
			{ID: 1, Name: "golang"},
			{ID: 3, Name: "testing"},
		},
		CreatedAt: time.Date(2024, 1, 10, 14, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2024, 1, 10, 14, 0, 0, 0, time.UTC),
	}}, output)

	return nil
}

func (s *CrudIntegrationTestSuite) TestCreatePostSyncsTags(app suite.AppUnderTest, client *resty.Client) error {
	defer app.WaitDone()
	defer app.Stop()

	var output PostOutput
	response, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(PostCreateInput{
			AuthorID: 2,
			Title:    "Integration Testing with SQLH",
			Status:   "draft",
			Tags: []PostInputTag{
				{ID: 2, Name: "database"},
				{ID: 3, Name: "testing"},
			},
		}).
		SetResult(&output).
		Execute(http.MethodPost, "/v1/post")
	if err != nil {
		return err
	}

	s.Equal(http.StatusOK, response.StatusCode())
	s.NotZero(output.ID)

	expectedOutput := PostOutput{
		ID:       output.ID,
		AuthorID: 2,
		Title:    "Integration Testing with SQLH",
		Status:   "draft",
		Tags: []TagOutput{
			{ID: 2, Name: "database"},
			{ID: 3, Name: "testing"},
		},
		CreatedAt: output.CreatedAt,
		UpdatedAt: output.UpdatedAt,
	}

	s.Equal(expectedOutput, output)

	stored, err := s.readPost(output.ID)
	if err != nil {
		return err
	}

	s.Equal(PostOutput{
		ID:       output.ID,
		AuthorID: 2,
		Title:    "Integration Testing with SQLH",
		Status:   "draft",
		Author: &AuthorOutput{
			ID:   2,
			Name: "Bob Smith",
		},
		Tags: []TagOutput{
			{ID: 2, Name: "database"},
			{ID: 3, Name: "testing"},
		},
		CreatedAt: stored.CreatedAt,
		UpdatedAt: stored.UpdatedAt,
	}, postOutputFromPost(stored))

	return nil
}

func (s *CrudIntegrationTestSuite) TestUpdatePostSyncsTags(app suite.AppUnderTest, client *resty.Client) error {
	defer app.WaitDone()
	defer app.Stop()

	var output PostOutput
	response, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(PostUpdateInput{
			AuthorID: 1,
			Title:    "Getting Started with Go and SQLH",
			Status:   "published",
			Tags: []PostInputTag{
				{ID: 1, Name: "golang"},
				{ID: 3, Name: "testing"},
			},
		}).
		SetResult(&output).
		Execute(http.MethodPut, "/v1/post/1")
	if err != nil {
		return err
	}

	s.Equal(http.StatusOK, response.StatusCode())

	expectedOutput := PostOutput{
		ID:       1,
		AuthorID: 1,
		Title:    "Getting Started with Go and SQLH",
		Status:   "published",
		Tags: []TagOutput{
			{ID: 1, Name: "golang"},
			{ID: 3, Name: "testing"},
		},
		CreatedAt: time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC),
		UpdatedAt: output.UpdatedAt,
	}

	s.Equal(expectedOutput, output)

	stored, err := s.readPost(1)
	if err != nil {
		return err
	}

	s.Equal(PostOutput{
		ID:       1,
		AuthorID: 1,
		Title:    "Getting Started with Go and SQLH",
		Status:   "published",
		Author: &AuthorOutput{
			ID:   1,
			Name: "Alice Johnson",
		},
		Tags: []TagOutput{
			{ID: 1, Name: "golang"},
			{ID: 3, Name: "testing"},
		},
		CreatedAt: time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC),
		UpdatedAt: stored.UpdatedAt,
	}, postOutputFromPost(stored))

	return nil
}

func (s *CrudIntegrationTestSuite) TestDeletePost(app suite.AppUnderTest, client *resty.Client) error {
	defer app.WaitDone()
	defer app.Stop()

	response, err := client.R().
		Execute(http.MethodDelete, "/v1/post/3")
	if err != nil {
		return err
	}

	s.Equal(http.StatusNoContent, response.StatusCode())

	_, err = s.readPost(3)
	s.Require().Error(err)
	s.True(errors.Is(err, sqlr.ErrNotFound))

	return nil
}

func (s *CrudIntegrationTestSuite) TestCreatePostPreloadsTagsOnCreate(app suite.AppUnderTest, client *resty.Client) error {
	defer app.WaitDone()
	defer app.Stop()

	var output MutationPreloadPostOutput
	response, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(MutationPreloadPostCreateInput{
			AuthorID: 2,
			Title:    "Create Preload Tags",
			Status:   "draft",
			Tags: []MutationPreloadPostInputTag{
				{ID: 2},
				{ID: 3},
			},
		}).
		SetResult(&output).
		Execute(http.MethodPost, "/v1/preload-post")
	if err != nil {
		return err
	}

	s.Equal(http.StatusOK, response.StatusCode())
	s.NotZero(output.ID)
	s.Equal(MutationPreloadPostOutput{
		ID:       output.ID,
		AuthorID: 2,
		Title:    "Create Preload Tags",
		Status:   "draft",
		Tags: []TagOutput{
			{ID: 2, Name: "database"},
			{ID: 3, Name: "testing"},
		},
		CreatedAt: output.CreatedAt,
		UpdatedAt: output.UpdatedAt,
	}, output)

	stored, err := s.readPost(output.ID)
	if err != nil {
		return err
	}

	s.Equal(PostOutput{
		ID:       output.ID,
		AuthorID: 2,
		Title:    "Create Preload Tags",
		Status:   "draft",
		Author: &AuthorOutput{
			ID:   2,
			Name: "Bob Smith",
		},
		Tags: []TagOutput{
			{ID: 2, Name: "database"},
			{ID: 3, Name: "testing"},
		},
		CreatedAt: stored.CreatedAt,
		UpdatedAt: stored.UpdatedAt,
	}, postOutputFromPost(stored))

	return nil
}

func (s *CrudIntegrationTestSuite) TestUpdatePostPreloadsTagsOnUpdate(app suite.AppUnderTest, client *resty.Client) error {
	defer app.WaitDone()
	defer app.Stop()

	var output MutationPreloadPostOutput
	response, err := client.R().
		SetHeader("Content-Type", "application/json").
		SetBody(MutationPreloadPostUpdateInput{
			AuthorID: 2,
			Title:    "Updated Preload Tags",
			Status:   "published",
			Tags: []MutationPreloadPostInputTag{
				{ID: 2},
				{ID: 3},
			},
		}).
		SetResult(&output).
		Execute(http.MethodPut, "/v1/preload-post/1")
	if err != nil {
		return err
	}

	s.Equal(http.StatusOK, response.StatusCode())
	s.Equal(MutationPreloadPostOutput{
		ID:       1,
		AuthorID: 2,
		Title:    "Updated Preload Tags",
		Status:   "published",
		Tags: []TagOutput{
			{ID: 2, Name: "database"},
			{ID: 3, Name: "testing"},
		},
		CreatedAt: time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC),
		UpdatedAt: output.UpdatedAt,
	}, output)

	stored, err := s.readPost(1)
	if err != nil {
		return err
	}

	s.Equal(PostOutput{
		ID:       1,
		AuthorID: 2,
		Title:    "Updated Preload Tags",
		Status:   "published",
		Author: &AuthorOutput{
			ID:   2,
			Name: "Bob Smith",
		},
		Tags: []TagOutput{
			{ID: 2, Name: "database"},
			{ID: 3, Name: "testing"},
		},
		CreatedAt: time.Date(2024, 1, 5, 10, 0, 0, 0, time.UTC),
		UpdatedAt: stored.UpdatedAt,
	}, postOutputFromPost(stored))

	return nil
}

func (s *CrudIntegrationTestSuite) readPost(id int64) (*Post, error) {
	repo, err := sqlr.NewRepository[int64, Post](s.ctx, s.Env().Config(), s.Env().Logger(), "default")
	if err != nil {
		return nil, err
	}

	defer repo.Close() //nolint:errcheck // test helper best effort cleanup

	return repo.Read(s.ctx, id, func(qb *sqlr.QueryBuilderRead) {
		qb.Preload("Author")
		qb.Preload("Tags")
	})
}

func postOutputFromPost(post *Post) PostOutput {
	var author *AuthorOutput
	if post.Author.Id != 0 || post.Author.Name != "" {
		author = &AuthorOutput{
			ID:   post.Author.Id,
			Name: post.Author.Name,
		}
	}

	tags := make([]TagOutput, len(post.Tags))
	for i, tag := range post.Tags {
		tags[i] = TagOutput{
			ID:   tag.Id,
			Name: tag.Name,
		}
	}

	return PostOutput{
		ID:        post.Id,
		AuthorID:  post.AuthorID,
		Title:     post.Title,
		Status:    post.Status,
		Author:    author,
		Tags:      tags,
		CreatedAt: post.CreatedAt,
		UpdatedAt: post.UpdatedAt,
	}
}
