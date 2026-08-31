package main

import (
	"context"
	"time"

	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlh"
	"github.com/gosoline-project/sqlr"
)

// UserCreateInput is the HTTP request body for creating a user.
// UserUpdateInput is the HTTP request body for updating a user.
// User is the database entity for a user.
// UserOutput is the HTTP response body for a user.
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
		ID        int       `json:"id"`
		Name      string    `json:"name"`
		CreatedAt time.Time `json:"created_at"`
		UpdatedAt time.Time `json:"updated_at"`
	}
)

// NewUserCrud returns the CRUD handler registration function for the user entity.
func NewUserCrud() httpserver.RegisterFactoryFunc {
	return sqlh.WithCrudHandlers(0, "user", sqlh.SimpleTransformer[int, User, UserCreateInput, UserUpdateInput, UserOutput](&UserTransformer{}))
}

// UserTransformer implements sqlh.Transformer for the User entity.
type UserTransformer struct{}

// TransformCreateInput converts a UserCreateInput DTO into a new User entity.
func (t *UserTransformer) TransformCreateInput(_ context.Context, input *UserCreateInput) (*User, error) {
	return &User{
		Name: input.Name,
	}, nil
}

// TransformUpdateInput merges a UserUpdateInput DTO into an existing User entity.
func (t *UserTransformer) TransformUpdateInput(ctx context.Context, user *User, input *UserUpdateInput) (*User, error) {
	user.Name = input.Name

	return user, nil
}

func (t *UserTransformer) TransformOutput(_ context.Context, user *User) (UserOutput, error) {
	return t.toOutput(user), nil
}

func (t *UserTransformer) toOutput(user *User) UserOutput {
	return UserOutput{
		ID:        user.Id,
		Name:      user.Name,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
