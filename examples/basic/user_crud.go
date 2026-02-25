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
	return sqlh.WithCrudHandlers(0, "user", sqlh.SimpleTransformer[int, User, UserCreateInput, UserUpdateInput](&UserTransformer{}))
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

// RenderEntityResponse renders a single User entity as an HTTP response.
func (t *UserTransformer) RenderEntityResponse(ctx context.Context, user *User) (httpserver.Response, error) {
	return httpserver.NewJsonResponse(t.toOutput(user)), nil
}

// RenderQueryResponse renders a list of User entities as an HTTP response.
func (t *UserTransformer) RenderQueryResponse(ctx context.Context, users []User) (httpserver.Response, error) {
	output := make([]UserOutput, len(users))
	for i, user := range users {
		output[i] = t.toOutput(&user)
	}

	return httpserver.NewJsonResponse(output), nil
}

func (t *UserTransformer) toOutput(user *User) UserOutput {
	return UserOutput{
		ID:        user.Id,
		Name:      user.Name,
		CreatedAt: user.CreatedAt,
		UpdatedAt: user.UpdatedAt,
	}
}
