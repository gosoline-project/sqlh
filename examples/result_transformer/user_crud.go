package main

import (
	"context"
	"time"

	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlh"
	"github.com/gosoline-project/sqlr"
)

// snippet-start: types
type (
	UserCreateInput struct {
		Name string `json:"name"`
	}
	UserUpdateInput struct {
		sqlh.InputByID[int]
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

// snippet-end: types

// snippet-start: crud
func NewUserCrud() httpserver.RegisterFactoryFunc {
	transformer := &UserTransformer{}
	definition := sqlh.NewCrudDefinition(
		transformer.TransformCreateInput,
		transformer.TransformUpdateInput,
		transformer.TransformPatchInputFromEntity,
		transformer.TransformOutput,
	)

	return sqlh.WithCrudHandlers(0, "user", sqlh.SimpleCrudDefinition(definition))
}

// snippet-end: crud

// snippet-start: transformer
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

func (t *UserTransformer) TransformPatchInputFromEntity(_ context.Context, user *User) (*UserUpdateInput, error) {
	return &UserUpdateInput{
		InputByID: sqlh.InputByID[int]{ID: user.Id},
		Name:      user.Name,
	}, nil
}

func (t *UserTransformer) TransformOutput(ctx context.Context, user *User) (UserOutput, error) {
	return UserOutput{
		Id:        user.Id,
		Name:      user.Name,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}, nil
}

// snippet-end: transformer
