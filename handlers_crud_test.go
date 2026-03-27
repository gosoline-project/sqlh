package sqlh

import (
	"context"
	"reflect"
	"testing"

	"github.com/gosoline-project/httpserver"
	sqlhmocks "github.com/gosoline-project/sqlh/mocks"
	"github.com/gosoline-project/sqlr"
	sqlrmocks "github.com/gosoline-project/sqlr/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type crudTaggedChild struct {
	sqlr.Entity[int64]
	Name string `db:"name"`
}

type crudTaggedItem struct {
	sqlr.Entity[int64]
	ChildID int64             `db:"child_id"`
	Name    string            `db:"name"`
	Child   crudTaggedChild   `db:"-" sqlr:"belongsTo:child_id" sqlh:"preload:create,read,update"`
	Tags    []crudTaggedChild `db:"-" sqlr:"many2many:crud_tagged_item_tags" sqlh:"preload:create,read,update;sync:create,update,delete"`
}

type crudTaggedQueryOnlyItem struct {
	sqlr.Entity[int64]
	ChildID int64           `db:"child_id"`
	Child   crudTaggedChild `db:"-" sqlr:"belongsTo:child_id" sqlh:"preload:query"`
}

type crudTaggedCreateInput struct {
	Name string `json:"name"`
}

type crudTaggedUpdateInput struct {
	Name string `json:"name"`
}

type crudTaggedTransformer struct{}

func (t *crudTaggedTransformer) TransformCreateInput(ctx context.Context, input *crudTaggedCreateInput) (*crudTaggedItem, error) {
	return &crudTaggedItem{Name: input.Name}, nil
}

func (t *crudTaggedTransformer) TransformUpdateInput(ctx context.Context, entity *crudTaggedItem, input *crudTaggedUpdateInput) (*crudTaggedItem, error) {
	entity.Name = input.Name

	return entity, nil
}

func (t *crudTaggedTransformer) RenderEntityResponse(ctx context.Context, entity *crudTaggedItem) (httpserver.Response, error) {
	return httpserver.NewJsonResponse(entity), nil
}

func (t *crudTaggedTransformer) RenderQueryResponse(ctx context.Context, entities []crudTaggedItem) (httpserver.Response, error) {
	return httpserver.NewJsonResponse(entities), nil
}

type crudInterfaceTransformer struct {
	crudTaggedTransformer
}

func (t *crudInterfaceTransformer) BuilderCreate(qb *sqlr.QueryBuilderCreate) {
	qb.Preload("ExtraCreate")
	qb.SyncAssociation("Child")
}

func (t *crudInterfaceTransformer) BuilderRead(qb *sqlr.QueryBuilderRead) {
	qb.Preload("ExtraRead")
}

func (t *crudInterfaceTransformer) BuilderQuery(qb *sqlr.QueryBuilderSelect) {
	qb.Preload("ExtraQuery")
}

func (t *crudInterfaceTransformer) BuilderUpdateRead(qb *sqlr.QueryBuilderRead) {
	qb.Preload("ExtraUpdateRead")
}

func (t *crudInterfaceTransformer) BuilderUpdateWrite(qb *sqlr.QueryBuilderUpdate) {
	qb.Preload("ExtraUpdateWrite")
	qb.SyncAssociation("Child")
}

func (t *crudInterfaceTransformer) BuilderDelete(qb *sqlr.QueryBuilderDelete) {
	qb.SyncAssociation("Child")
}

func TestHandlerCrud_TagBuildersApplyToReadAndQuery(t *testing.T) {
	tags, err := parseEntityBuilderTags[crudTaggedItem]()
	require.NoError(t, err)

	readQB := sqlr.NewQueryBuilderRead()
	queryQB := sqlr.NewQueryBuilderSelect()

	composeBuilders(builderReadFromTags(tags))(readQB)
	composeBuilders(builderQueryFromTags(tags))(queryQB)

	require.ElementsMatch(t, []string{"Child", "Tags"}, preloadRelationsFromReadBuilder(readQB))
	require.Empty(t, preloadRelationsFromSelectBuilder(queryQB))
}

func TestHandlerCrud_QueryBuilderUsesQueryPaths(t *testing.T) {
	tags, err := parseEntityBuilderTags[crudTaggedQueryOnlyItem]()
	require.NoError(t, err)

	readQB := sqlr.NewQueryBuilderRead()
	queryQB := sqlr.NewQueryBuilderSelect()

	composeBuilders(builderReadFromTags(tags))(readQB)
	composeBuilders(builderQueryFromTags(tags))(queryQB)

	require.Empty(t, preloadRelationsFromReadBuilder(readQB))
	require.Equal(t, []string{"Child"}, preloadRelationsFromSelectBuilder(queryQB))
}

func TestHandlerCrud_TagBuildersApplyToCreateAndUpdate(t *testing.T) {
	tags, err := parseEntityBuilderTags[crudTaggedItem]()
	require.NoError(t, err)

	createQB := sqlr.NewQueryBuilderCreate()
	deleteQB := sqlr.NewQueryBuilderDelete()
	updateReadQB := sqlr.NewQueryBuilderRead()
	updateWriteQB := sqlr.NewQueryBuilderUpdate()

	composeBuilders(builderCreateFromTags(tags))(createQB)
	composeBuilders(builderDeleteFromTags(tags))(deleteQB)
	composeBuilders(builderUpdateReadFromTags(tags))(updateReadQB)
	composeBuilders(builderUpdateWriteFromTags(tags))(updateWriteQB)

	require.ElementsMatch(t, []string{"Child", "Tags"}, preloadRelationsFromCreateBuilder(createQB))
	require.ElementsMatch(t, []string{"Tags"}, syncPathsFromCreateBuilder(createQB))
	require.ElementsMatch(t, []string{"Tags"}, syncPathsFromDeleteBuilder(deleteQB))
	require.ElementsMatch(t, []string{"Child", "Tags"}, preloadRelationsFromReadBuilder(updateReadQB))
	require.ElementsMatch(t, []string{"Child", "Tags"}, preloadRelationsFromUpdateBuilder(updateWriteQB))
	require.ElementsMatch(t, []string{"Tags"}, syncPathsFromUpdateBuilder(updateWriteQB))
}

func TestHandlerCrud_ComposesTagAndInterfaceBuilders(t *testing.T) {
	tags, err := parseEntityBuilderTags[crudTaggedItem]()
	require.NoError(t, err)

	transformer := &crudInterfaceTransformer{}

	createQB := sqlr.NewQueryBuilderCreate()
	readQB := sqlr.NewQueryBuilderRead()
	queryQB := sqlr.NewQueryBuilderSelect()
	deleteQB := sqlr.NewQueryBuilderDelete()
	updateReadQB := sqlr.NewQueryBuilderRead()
	updateWriteQB := sqlr.NewQueryBuilderUpdate()

	composeBuilders(builderCreateFromTags(tags), transformer.BuilderCreate)(createQB)
	composeBuilders(builderReadFromTags(tags), transformer.BuilderRead)(readQB)
	composeBuilders(builderQueryFromTags(tags), transformer.BuilderQuery)(queryQB)
	composeBuilders(builderDeleteFromTags(tags), transformer.BuilderDelete)(deleteQB)
	composeBuilders(builderUpdateReadFromTags(tags), transformer.BuilderUpdateRead)(updateReadQB)
	composeBuilders(builderUpdateWriteFromTags(tags), transformer.BuilderUpdateWrite)(updateWriteQB)

	require.ElementsMatch(t, []string{"Child", "ExtraCreate", "Tags"}, preloadRelationsFromCreateBuilder(createQB))
	require.ElementsMatch(t, []string{"Child", "Tags"}, syncPathsFromCreateBuilder(createQB))
	require.ElementsMatch(t, []string{"Child", "ExtraRead", "Tags"}, preloadRelationsFromReadBuilder(readQB))
	require.Equal(t, []string{"ExtraQuery"}, preloadRelationsFromSelectBuilder(queryQB))
	require.ElementsMatch(t, []string{"Child", "Tags"}, syncPathsFromDeleteBuilder(deleteQB))
	require.ElementsMatch(t, []string{"Child", "Tags", "ExtraUpdateRead"}, preloadRelationsFromReadBuilder(updateReadQB))
	require.ElementsMatch(t, []string{"Child", "Tags", "ExtraUpdateWrite"}, preloadRelationsFromUpdateBuilder(updateWriteQB))
	require.ElementsMatch(t, []string{"Child", "Tags"}, syncPathsFromUpdateBuilder(updateWriteQB))
}

func TestHandlerCrud_HandleCreateUsesCreateRehydration(t *testing.T) {
	tags, err := parseEntityBuilderTags[crudTaggedItem]()
	require.NoError(t, err)

	createdEntity := &crudTaggedItem{
		Entity: sqlr.Entity[int64]{Id: 1},
		Name:   "created",
		Child:  crudTaggedChild{Entity: sqlr.Entity[int64]{Id: 10}, Name: "child"},
		Tags:   []crudTaggedChild{{Entity: sqlr.Entity[int64]{Id: 11}, Name: "tag"}},
	}
	repo := sqlrmocks.NewRepository[int64, crudTaggedItem](t)
	transformer := sqlhmocks.NewTransformer[int64, crudTaggedItem, crudTaggedCreateInput, crudTaggedUpdateInput](t)
	var createInputName string
	var createPaths []string
	var createSyncs []string
	var transformerEntity *crudTaggedItem
	var renderedEntity *crudTaggedItem

	transformer.EXPECT().
		TransformCreateInput(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, input *crudTaggedCreateInput) (*crudTaggedItem, error) {
			return &crudTaggedItem{Name: input.Name}, nil
		})

	repo.EXPECT().
		Create(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, entity *crudTaggedItem, opts ...func(qb *sqlr.QueryBuilderCreate)) error {
			qb := applyBuilderOptions(sqlr.NewQueryBuilderCreate(), opts)

			transformerEntity = entity
			createInputName = entity.Name
			createPaths = preloadRelationsFromCreateBuilder(qb)
			createSyncs = syncPathsFromCreateBuilder(qb)
			*entity = *createdEntity

			return nil
		})

	transformer.EXPECT().
		RenderEntityResponse(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, entity *crudTaggedItem) (httpserver.Response, error) {
			renderedEntity = entity

			return httpserver.NewStatusResponse(200), nil
		})

	handler := &HandlerCrud[int64, crudTaggedItem, crudTaggedCreateInput, crudTaggedUpdateInput]{
		repo:          repo,
		transformer:   transformer,
		builderCreate: composeBuilders(builderCreateFromTags(tags)),
	}

	_, err = handler.HandleCreate(context.Background(), &crudTaggedCreateInput{Name: "new"})
	require.NoError(t, err)
	require.Equal(t, []string{"Child", "Tags"}, createPaths)
	require.Equal(t, []string{"Tags"}, createSyncs)
	require.Equal(t, "new", createInputName)
	require.Equal(t, "created", transformerEntity.Name)
	require.Same(t, transformerEntity, renderedEntity)
	require.Equal(t, int64(1), renderedEntity.GetId())
	require.Len(t, renderedEntity.Tags, 1)
}

func TestHandlerCrud_HandleUpdateUsesUpdateRehydration(t *testing.T) {
	tags, err := parseEntityBuilderTags[crudTaggedItem]()
	require.NoError(t, err)

	readEntity := &crudTaggedItem{
		Entity: sqlr.Entity[int64]{Id: 1},
		Name:   "before",
	}
	updatedEntity := &crudTaggedItem{
		Entity: sqlr.Entity[int64]{Id: 1},
		Name:   "after",
		Child:  crudTaggedChild{Entity: sqlr.Entity[int64]{Id: 10}, Name: "child"},
		Tags:   []crudTaggedChild{{Entity: sqlr.Entity[int64]{Id: 11}, Name: "tag"}},
	}
	repo := sqlrmocks.NewRepository[int64, crudTaggedItem](t)
	transformer := sqlhmocks.NewTransformer[int64, crudTaggedItem, crudTaggedCreateInput, crudTaggedUpdateInput](t)
	var readCalls int
	var updateReadPaths []string
	var updateWritePaths []string
	var updateWriteSyncs []string
	var transformerEntity *crudTaggedItem
	var renderedEntity *crudTaggedItem

	transformer.EXPECT().
		TransformUpdateInput(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, entity *crudTaggedItem, input *crudTaggedUpdateInput) (*crudTaggedItem, error) {
			entity.Name = input.Name

			return entity, nil
		})

	repo.EXPECT().
		Read(mock.Anything, int64(1), mock.Anything).
		RunAndReturn(func(_ context.Context, _ int64, opts ...func(qb *sqlr.QueryBuilderRead)) (*crudTaggedItem, error) {
			qb := applyBuilderOptions(sqlr.NewQueryBuilderRead(), opts)

			readCalls++
			updateReadPaths = preloadRelationsFromReadBuilder(qb)

			return readEntity, nil
		}).
		Once()

	repo.EXPECT().
		Update(mock.Anything, mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, entity *crudTaggedItem, opts ...func(qb *sqlr.QueryBuilderUpdate)) (*crudTaggedItem, error) {
			qb := applyBuilderOptions(sqlr.NewQueryBuilderUpdate(), opts)

			transformerEntity = entity
			updateWritePaths = preloadRelationsFromUpdateBuilder(qb)
			updateWriteSyncs = syncPathsFromUpdateBuilder(qb)

			return updatedEntity, nil
		}).
		Once()

	transformer.EXPECT().
		RenderEntityResponse(mock.Anything, mock.Anything).
		RunAndReturn(func(_ context.Context, entity *crudTaggedItem) (httpserver.Response, error) {
			renderedEntity = entity

			return httpserver.NewStatusResponse(200), nil
		})

	handler := &HandlerCrud[int64, crudTaggedItem, crudTaggedCreateInput, crudTaggedUpdateInput]{
		repo:               repo,
		transformer:        transformer,
		builderUpdateRead:  composeBuilders(builderUpdateReadFromTags(tags)),
		builderUpdateWrite: composeBuilders(builderUpdateWriteFromTags(tags)),
	}

	_, err = handler.HandleUpdate(context.Background(), 1, &crudTaggedUpdateInput{Name: "changed"})
	require.NoError(t, err)
	require.Equal(t, 1, readCalls)
	require.Equal(t, []string{"Child", "Tags"}, updateReadPaths)
	require.Equal(t, []string{"Child", "Tags"}, updateWritePaths)
	require.Equal(t, []string{"Tags"}, updateWriteSyncs)
	require.Same(t, readEntity, transformerEntity)
	require.Equal(t, "changed", transformerEntity.Name)
	require.Same(t, updatedEntity, renderedEntity)
}

func applyBuilderOptions[T any](qb T, opts []func(T)) T {
	for _, opt := range opts {
		if opt != nil {
			opt(qb)
		}
	}

	return qb
}

func preloadRelationsFromReadBuilder(qb *sqlr.QueryBuilderRead) []string {
	return preloadRelations(reflectValueField(qb, "preloads"))
}

func preloadRelationsFromCreateBuilder(qb *sqlr.QueryBuilderCreate) []string {
	return preloadRelations(reflectValueField(qb, "preloads"))
}

func preloadRelationsFromSelectBuilder(qb *sqlr.QueryBuilderSelect) []string {
	return preloadRelations(reflectValueField(qb, "preloads"))
}

func preloadRelationsFromUpdateBuilder(qb *sqlr.QueryBuilderUpdate) []string {
	return preloadRelations(reflectValueField(qb, "preloads"))
}

func preloadRelations(preloadsValue reflect.Value) []string {
	relations := make([]string, 0, preloadsValue.Len())
	for i := 0; i < preloadsValue.Len(); i++ {
		relations = append(relations, preloadsValue.Index(i).FieldByName("relation").String())
	}

	return relations
}

func syncPathsFromCreateBuilder(qb *sqlr.QueryBuilderCreate) []string {
	return syncPathsFromBuilder(qb)
}

func syncPathsFromUpdateBuilder(qb *sqlr.QueryBuilderUpdate) []string {
	return syncPathsFromBuilder(qb)
}

func syncPathsFromDeleteBuilder(qb *sqlr.QueryBuilderDelete) []string {
	return syncPathsFromBuilder(qb)
}

func syncPathsFromBuilder(qb any) []string {
	associationValue := reflectValueField(qb, "associationOptions")
	syncPathsValue := associationValue.FieldByName("syncPaths")
	paths := make([]string, syncPathsValue.Len())
	for i := 0; i < syncPathsValue.Len(); i++ {
		paths[i] = syncPathsValue.Index(i).String()
	}

	return paths
}

func reflectValueField(value any, fieldName string) reflect.Value {
	v := reflect.ValueOf(value)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	return v.FieldByName(fieldName)
}
