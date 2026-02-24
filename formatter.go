package sqlh

import "context"

type Formatter[O any, LO any] interface {
	Format(ctx context.Context, transformerOutput []*O) (LO, error)
}

type SimpleListOutput[O any] struct {
	Total   int  `json:"total"`
	Results []*O `json:"results"`
}
type SimpleListFormatter[O any] struct {
}

func (s *SimpleListFormatter[O]) Format(_ context.Context, transformerOutput []*O) (SimpleListOutput[O], error) {
	out := SimpleListOutput[O]{
		Total:   len(transformerOutput),
		Results: transformerOutput,
	}

	return out, nil
}

func NewSimpleListFormatter[O any]() Formatter[O, SimpleListOutput[O]] {
	return &SimpleListFormatter[O]{}
}
