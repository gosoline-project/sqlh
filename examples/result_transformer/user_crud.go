package main

import (
	"context"
	"time"

	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlh"
	"github.com/gosoline-project/sqlr"
)

type (
	UserCreateInput struct {
		Name string `json:"name"`
	}
	UserUpdateInput struct {
		Name string `json:"name"`
	}
	User struct {
		sqlr.Entity[int]
		Name string
	}
	UserOutput struct {
		Id        int       `json:"id"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
)

func NewUserCrud() httpserver.RegisterFactoryFunc {
	return sqlh.WithCrudHandlers(0, "user", sqlh.NewJSONResultsTransformer[int, User, UserCreateInput, UserUpdateInput](&UserTransformer{}))
}

var _ sqlh.JsonResultsTransformer[int, User, UserCreateInput, UserUpdateInput] = (*UserTransformer)(nil)

type UserTransformer struct{}

func (t *UserTransformer) TransformCreateInput(ctx context.Context, input *UserCreateInput) (*User, error) {
	return &User{
		Name: input.Name,
	}, nil
}

func (t *UserTransformer) TransformUpdateInput(ctx context.Context, user *User, input *UserUpdateInput) (*User, error) {
	user.Name = input.Name

	return user, nil
}

func (t *UserTransformer) TransformOutput(ctx context.Context, user *User) (any, error) {
	return UserOutput{
		Id:        user.Id,
		Name:      user.Name,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}, nil
}
