package sqlh

import (
	"context"
	"testing"

	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/cfg"
	"github.com/justtrackio/gosoline/pkg/log"
)

type testJsonEntity struct {
	sqlr.Entity[int]
}

type testJsonInput struct{}

type testJsonResultsTransformer struct {
	builderCreateCalled      bool
	builderReadCalled        bool
	builderDeleteCalled      bool
	builderUpdateReadCalled  bool
	builderUpdateWriteCalled bool
}

func (t *testJsonResultsTransformer) TransformCreateInput(ctx context.Context, input *testJsonInput) (*testJsonEntity, error) {
	return &testJsonEntity{}, nil
}

func (t *testJsonResultsTransformer) TransformUpdateInput(ctx context.Context, entity *testJsonEntity, input *testJsonInput) (*testJsonEntity, error) {
	return entity, nil
}

func (t *testJsonResultsTransformer) TransformOutput(ctx context.Context, entity *testJsonEntity) (any, error) {
	return map[string]any{"id": entity.Id}, nil
}

func (t *testJsonResultsTransformer) BuilderCreate(qb *sqlr.QueryBuilderCreate) {
	t.builderCreateCalled = true
}

func (t *testJsonResultsTransformer) BuilderRead(qb *sqlr.QueryBuilderRead) {
	t.builderReadCalled = true
}

func (t *testJsonResultsTransformer) BuilderDelete(qb *sqlr.QueryBuilderDelete) {
	t.builderDeleteCalled = true
}

func (t *testJsonResultsTransformer) BuilderUpdateRead(qb *sqlr.QueryBuilderRead) {
	t.builderUpdateReadCalled = true
}

func (t *testJsonResultsTransformer) BuilderUpdateWrite(qb *sqlr.QueryBuilderUpdate) {
	t.builderUpdateWriteCalled = true
}

type testJsonResultsTransformerWithoutBuilders struct{}

func (t *testJsonResultsTransformerWithoutBuilders) TransformCreateInput(ctx context.Context, input *testJsonInput) (*testJsonEntity, error) {
	return &testJsonEntity{}, nil
}

func (t *testJsonResultsTransformerWithoutBuilders) TransformUpdateInput(ctx context.Context, entity *testJsonEntity, input *testJsonInput) (*testJsonEntity, error) {
	return entity, nil
}

func (t *testJsonResultsTransformerWithoutBuilders) TransformOutput(ctx context.Context, entity *testJsonEntity) (any, error) {
	return map[string]any{"id": entity.Id}, nil
}

func TestNewJsonResultsTransformer_ForwardsBuilderAwareInterfaces(t *testing.T) {
	t.Parallel()

	wrapped := &testJsonResultsTransformer{}
	transformer, err := NewJsonResultsTransformer[int, testJsonEntity, testJsonInput, testJsonInput](wrapped)(context.Background(), cfg.New(), log.NewLogger())
	if err != nil {
		t.Fatalf("expected wrapper creation to succeed: %v", err)
	}

	builderCreate, ok := transformer.(BuilderCreateAware)
	if !ok {
		t.Fatal("expected wrapped transformer to implement BuilderCreateAware")
	}

	builderRead, ok := transformer.(BuilderReadAware)
	if !ok {
		t.Fatal("expected wrapped transformer to implement BuilderReadAware")
	}

	builderDelete, ok := transformer.(BuilderDeleteAware)
	if !ok {
		t.Fatal("expected wrapped transformer to implement BuilderDeleteAware")
	}

	builderUpdateRead, ok := transformer.(BuilderUpdateReadAware)
	if !ok {
		t.Fatal("expected wrapped transformer to implement BuilderUpdateReadAware")
	}

	builderUpdateWrite, ok := transformer.(BuilderUpdateWriteAware)
	if !ok {
		t.Fatal("expected wrapped transformer to implement BuilderUpdateWriteAware")
	}

	builderCreate.BuilderCreate(sqlr.NewQueryBuilderCreate())
	builderRead.BuilderRead(sqlr.NewQueryBuilderRead())
	builderDelete.BuilderDelete(sqlr.NewQueryBuilderDelete())
	builderUpdateRead.BuilderUpdateRead(sqlr.NewQueryBuilderRead())
	builderUpdateWrite.BuilderUpdateWrite(sqlr.NewQueryBuilderUpdate())

	if !wrapped.builderCreateCalled {
		t.Fatal("expected BuilderCreate to be forwarded")
	}

	if !wrapped.builderReadCalled {
		t.Fatal("expected BuilderRead to be forwarded")
	}

	if !wrapped.builderDeleteCalled {
		t.Fatal("expected BuilderDelete to be forwarded")
	}

	if !wrapped.builderUpdateReadCalled {
		t.Fatal("expected BuilderUpdateRead to be forwarded")
	}

	if !wrapped.builderUpdateWriteCalled {
		t.Fatal("expected BuilderUpdateWrite to be forwarded")
	}
}

func TestNewJsonResultsTransformer_BuilderAwareInterfacesNoopWhenUnsupported(t *testing.T) {
	t.Parallel()

	transformer, err := NewJsonResultsTransformer[int, testJsonEntity, testJsonInput, testJsonInput](&testJsonResultsTransformerWithoutBuilders{})(context.Background(), cfg.New(), log.NewLogger())
	if err != nil {
		t.Fatalf("expected wrapper creation to succeed: %v", err)
	}

	builderCreate, ok := transformer.(BuilderCreateAware)
	if !ok {
		t.Fatal("expected wrapped transformer to implement BuilderCreateAware")
	}

	builderRead, ok := transformer.(BuilderReadAware)
	if !ok {
		t.Fatal("expected wrapped transformer to implement BuilderReadAware")
	}

	builderDelete, ok := transformer.(BuilderDeleteAware)
	if !ok {
		t.Fatal("expected wrapped transformer to implement BuilderDeleteAware")
	}

	builderUpdateRead, ok := transformer.(BuilderUpdateReadAware)
	if !ok {
		t.Fatal("expected wrapped transformer to implement BuilderUpdateReadAware")
	}

	builderUpdateWrite, ok := transformer.(BuilderUpdateWriteAware)
	if !ok {
		t.Fatal("expected wrapped transformer to implement BuilderUpdateWriteAware")
	}

	builderCreate.BuilderCreate(sqlr.NewQueryBuilderCreate())
	builderRead.BuilderRead(sqlr.NewQueryBuilderRead())
	builderDelete.BuilderDelete(sqlr.NewQueryBuilderDelete())
	builderUpdateRead.BuilderUpdateRead(sqlr.NewQueryBuilderRead())
	builderUpdateWrite.BuilderUpdateWrite(sqlr.NewQueryBuilderUpdate())
}
