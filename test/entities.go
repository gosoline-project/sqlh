//go:build integration && fixtures

package test

import (
	"context"
	"time"

	gosolinehttpserver "github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlh"
	"github.com/gosoline-project/sqlr"
)

type Post struct {
	sqlr.Entity[int64]
	AuthorID int64  `db:"author_id"`
	Title    string `db:"title"`
	Status   string `db:"status"`
	Author   Author `db:"-" sqlr:"belongsTo:author_id" sqlh:"preload:read,query"`
	Tags     []Tag  `db:"-" sqlr:"many2many:post_tags;sync:update" sqlh:"preload:read,update,query;sync:create,update,delete"`
}

type MutationPreloadPost struct {
	sqlr.Entity[int64]
	AuthorID int64  `db:"author_id"`
	Title    string `db:"title"`
	Status   string `db:"status"`
	Tags     []Tag  `db:"-" sqlr:"many2many:post_tags;parentKey:post_id;relatedKey:tag_id" sqlh:"preload:create,update;sync:create,update,delete"`
}

func (MutationPreloadPost) TableName() string {
	return "posts"
}

type Tag struct {
	sqlr.Entity[int64]
	Name string `db:"name"`
}

type Author struct {
	sqlr.Entity[int64]
	Name string `db:"name"`
}

type PostInputTag struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type PostCreateInput struct {
	AuthorID int64          `json:"author_id"`
	Title    string         `json:"title"`
	Status   string         `json:"status"`
	Tags     []PostInputTag `json:"tags"`
}

type PostUpdateInput struct {
	sqlh.InputByID[int64]
	AuthorID int64          `json:"author_id"`
	Title    string         `json:"title"`
	Status   string         `json:"status"`
	Tags     []PostInputTag `json:"tags"`
}

type MutationPreloadPostCreateInput struct {
	AuthorID int64                         `json:"author_id"`
	Title    string                        `json:"title"`
	Status   string                        `json:"status"`
	Tags     []MutationPreloadPostInputTag `json:"tags"`
}

type MutationPreloadPostUpdateInput struct {
	sqlh.InputByID[int64]
	AuthorID int64                         `json:"author_id"`
	Title    string                        `json:"title"`
	Status   string                        `json:"status"`
	Tags     []MutationPreloadPostInputTag `json:"tags"`
}

type MutationPreloadPostInputTag struct {
	ID int64 `json:"id"`
}

type PostOutput struct {
	ID        int64         `json:"id"`
	AuthorID  int64         `json:"author_id"`
	Title     string        `json:"title"`
	Status    string        `json:"status"`
	Author    *AuthorOutput `json:"author,omitempty"`
	Tags      []TagOutput   `json:"tags,omitempty"`
	CreatedAt time.Time     `json:"created_at"`
	UpdatedAt time.Time     `json:"updated_at"`
}

type AuthorOutput struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type TagOutput struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type MutationPreloadPostOutput struct {
	ID        int64       `json:"id"`
	AuthorID  int64       `json:"author_id"`
	Title     string      `json:"title"`
	Status    string      `json:"status"`
	Tags      []TagOutput `json:"tags,omitempty"`
	CreatedAt time.Time   `json:"created_at"`
	UpdatedAt time.Time   `json:"updated_at"`
}

type PostTransformer struct{}

type MutationPreloadPostTransformer struct{}

func (t *PostTransformer) TransformCreateInput(_ context.Context, input *PostCreateInput) (*Post, error) {
	return &Post{
		AuthorID: input.AuthorID,
		Title:    input.Title,
		Status:   input.Status,
		Tags:     inputTagsToTags(input.Tags),
	}, nil
}

func (t *PostTransformer) TransformUpdateInput(_ context.Context, post *Post, input *PostUpdateInput) (*Post, error) {
	post.AuthorID = input.AuthorID
	post.Title = input.Title
	post.Status = input.Status
	post.Tags = inputTagsToTags(input.Tags)

	return post, nil
}

func (t *PostTransformer) TransformPatchBaseline(_ context.Context, post *Post) (*PostUpdateInput, error) {
	tags := make([]PostInputTag, len(post.Tags))
	for i, tag := range post.Tags {
		tags[i] = PostInputTag{ID: tag.Id, Name: tag.Name}
	}

	return &PostUpdateInput{
		InputByID: sqlh.InputByID[int64]{ID: post.Id},
		AuthorID:  post.AuthorID,
		Title:     post.Title,
		Status:    post.Status,
		Tags:      tags,
	}, nil
}

func (t *PostTransformer) TransformOutput(_ context.Context, post *Post) (PostOutput, error) {
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
	}, nil
}

func (t *MutationPreloadPostTransformer) TransformCreateInput(_ context.Context, input *MutationPreloadPostCreateInput) (*MutationPreloadPost, error) {
	return &MutationPreloadPost{
		AuthorID: input.AuthorID,
		Title:    input.Title,
		Status:   input.Status,
		Tags:     mutationPreloadInputTagsToTags(input.Tags),
	}, nil
}

func (t *MutationPreloadPostTransformer) TransformUpdateInput(_ context.Context, post *MutationPreloadPost, input *MutationPreloadPostUpdateInput) (*MutationPreloadPost, error) {
	post.AuthorID = input.AuthorID
	post.Title = input.Title
	post.Status = input.Status
	post.Tags = mutationPreloadInputTagsToTags(input.Tags)

	return post, nil
}

func (t *MutationPreloadPostTransformer) TransformOutput(_ context.Context, post *MutationPreloadPost) (MutationPreloadPostOutput, error) {
	tags := make([]TagOutput, len(post.Tags))
	for i, tag := range post.Tags {
		tags[i] = TagOutput{
			ID:   tag.Id,
			Name: tag.Name,
		}
	}

	return MutationPreloadPostOutput{
		ID:        post.Id,
		AuthorID:  post.AuthorID,
		Title:     post.Title,
		Status:    post.Status,
		Tags:      tags,
		CreatedAt: post.CreatedAt,
		UpdatedAt: post.UpdatedAt,
	}, nil
}

func NewPostCrud() gosolinehttpserver.RegisterFactoryFunc {
	return sqlh.WithCrudHandlers(1, "post", sqlh.SimpleTransformer[int64, Post, PostCreateInput, PostUpdateInput, PostOutput](&PostTransformer{}))
}

func NewMutationPreloadPostCrud() gosolinehttpserver.RegisterFactoryFunc {
	return sqlh.WithCrudHandlers(1, "preload-post", sqlh.SimpleTransformer[int64, MutationPreloadPost, MutationPreloadPostCreateInput, MutationPreloadPostUpdateInput, MutationPreloadPostOutput](&MutationPreloadPostTransformer{}))
}

func inputTagsToTags(inputTags []PostInputTag) []Tag {
	if len(inputTags) == 0 {
		return nil
	}

	tags := make([]Tag, len(inputTags))
	for i, tag := range inputTags {
		tags[i] = Tag{
			Entity: sqlr.Entity[int64]{Id: tag.ID},
			Name:   tag.Name,
		}
	}

	return tags
}

func mutationPreloadInputTagsToTags(inputTags []MutationPreloadPostInputTag) []Tag {
	if len(inputTags) == 0 {
		return nil
	}

	tags := make([]Tag, len(inputTags))
	for i, tag := range inputTags {
		tags[i] = Tag{Entity: sqlr.Entity[int64]{Id: tag.ID}}
	}

	return tags
}
