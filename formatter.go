package sqlh

import (
	"context"

	"github.com/gosoline-project/httpserver"
)

type Formatter[O any] interface {
	Format(ctx context.Context, transformerOutput []*O) (httpserver.Response, error)
}

type SimpleListOutput[O any] struct {
	Total   int  `json:"total"`
	Results []*O `json:"results"`
}

type SimpleListFormatter[O any] struct {
}

func (s *SimpleListFormatter[O]) Format(_ context.Context, transformerOutput []*O) (httpserver.Response, error) {
	out := SimpleListOutput[O]{
		Total:   len(transformerOutput),
		Results: transformerOutput,
	}

	return httpserver.NewJsonResponse(out), nil
}

func NewSimpleListFormatter[O any]() Formatter[O] {
	return &SimpleListFormatter[O]{}
}
