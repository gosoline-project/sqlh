package main

import (
	"context"

	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlh"
	"github.com/gosoline-project/sqlr"
)

func NewUserCrud() httpserver.RegisterFactoryFunc {
	return sqlh.WithCrudHandlers[int, User, UserCreateInput, UserUpdateInput](0, "user", sqlh.SimpleTransformer[int, User, UserCreateInput, UserUpdateInput](&UserTransformer{}))
}

type UserCreateInput struct {
	Name string `json:"name"`
}

type UserUpdateInput struct {
	Name string `json:"name"`
}

type User struct {
	sqlr.Entity[int]
	Name string
}

type UserTransformer struct{}

func (u UserTransformer) TransformCreate(ctx context.Context, input *UserCreateInput) (*User, error) {
	return &User{
		Name: input.Name,
	}, nil
}

func (u UserTransformer) TransformUpdate(ctx context.Context, user *User, input *UserUpdateInput) (*User, error) {
	user.Name = input.Name

	return user, nil
}
