package sqlh

import "context"

type Formatter[E any, O any, LE any, LO any] interface {
	FormatOutputList(ctx context.Context, transformerOutput []*O) (LO, error)
	FormatEntityList(ctx context.Context, entities []*E) (LE, error)
}

type SimpleListOutput[O any] struct {
	Total   int  `json:"total"`
	Results []*O `json:"results"`
}
type SimpleListFormatter[E any, O any] struct {
}

func (s *SimpleListFormatter[E, O]) FormatOutputList(_ context.Context, transformerOutput []*O) (SimpleListOutput[O], error) {
	out := SimpleListOutput[O]{
		Total:   len(transformerOutput),
		Results: transformerOutput,
	}

	return out, nil
}

func (s *SimpleListFormatter[E, O]) FormatEntityList(_ context.Context, entities []*E) (SimpleListOutput[E], error) {
	out := SimpleListOutput[E]{
		Total:   len(entities),
		Results: entities,
	}

	return out, nil
}

func NewSimpleListFormatter[E any, O any]() Formatter[E, O, SimpleListOutput[E], SimpleListOutput[O]] {
	return &SimpleListFormatter[E, O]{}
}
