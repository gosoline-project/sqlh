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
	return sqlh.WithCrudHandlers[int, User, UserCreateInput, UserUpdateInput, UserOutput, sqlh.SimpleListOutput[UserOutput]](0, "user", sqlh.SimpleTransformer[int, User, UserCreateInput, UserUpdateInput, UserOutput](&UserTransformer{}), sqlh.NewSimpleListFormatter[UserOutput]())
}

type UserTransformer struct{}

func (u UserTransformer) TransformCreate(_ context.Context, input *UserCreateInput) (*User, error) {
	return &User{
		Name: input.Name,
	}, nil
}

func (u UserTransformer) TransformUpdate(_ context.Context, user *User, input *UserUpdateInput) (*User, error) {
	user.Name = input.Name

	return user, nil
}

func (u UserTransformer) TransformOutput(_ context.Context, user *User) (*UserOutput, error) {
	out := &UserOutput{
		Id:        user.Id,
		Name:      user.Name,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}

	return out, nil
}
