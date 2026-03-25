package sqlh

import (
	"context"
	"reflect"
	"testing"

	"github.com/gosoline-project/httpserver"
	"github.com/gosoline-project/sqlr"
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
	Child   crudTaggedChild   `db:"-" sqlr:"belongsTo:child_id" sqlh:"preload:read,update"`
	Tags    []crudTaggedChild `db:"-" sqlr:"many2many:crud_tagged_item_tags" sqlh:"preload:read,update;sync:create,update,delete"`
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

	require.ElementsMatch(t, []string{"Child", "Tags"}, syncPathsFromCreateBuilder(createQB))
	require.ElementsMatch(t, []string{"Child", "ExtraRead", "Tags"}, preloadRelationsFromReadBuilder(readQB))
	require.Equal(t, []string{"ExtraQuery"}, preloadRelationsFromSelectBuilder(queryQB))
	require.ElementsMatch(t, []string{"Child", "Tags"}, syncPathsFromDeleteBuilder(deleteQB))
	require.ElementsMatch(t, []string{"Child", "Tags", "ExtraUpdateRead"}, preloadRelationsFromReadBuilder(updateReadQB))
	require.ElementsMatch(t, []string{"Child", "Tags", "ExtraUpdateWrite"}, preloadRelationsFromUpdateBuilder(updateWriteQB))
	require.ElementsMatch(t, []string{"Child", "Tags"}, syncPathsFromUpdateBuilder(updateWriteQB))
}

type recordingCrudRepo struct {
	readEntity        *crudTaggedItem
	updatedEntity     *crudTaggedItem
	readCalls         int
	updateReadPaths   []string
	updateWritePaths  []string
	updateWriteSyncs  []string
	transformerEntity *crudTaggedItem
}

func (r *recordingCrudRepo) Create(ctx context.Context, entity *crudTaggedItem, opts ...func(qb *sqlr.QueryBuilderCreate)) error {
	panic("unexpected Create call")
}

func (r *recordingCrudRepo) Read(ctx context.Context, id int64, opts ...func(qb *sqlr.QueryBuilderRead)) (*crudTaggedItem, error) {
	r.readCalls++
	qb := sqlr.NewQueryBuilderRead()
	for _, opt := range opts {
		if opt != nil {
			opt(qb)
		}
	}

	r.updateReadPaths = preloadRelationsFromReadBuilder(qb)

	return r.readEntity, nil
}

func (r *recordingCrudRepo) Query(ctx context.Context, opts ...func(qb *sqlr.QueryBuilderSelect)) ([]crudTaggedItem, error) {
	panic("unexpected Query call")
}

func (r *recordingCrudRepo) Update(ctx context.Context, entity *crudTaggedItem, opts ...func(qb *sqlr.QueryBuilderUpdate)) (*crudTaggedItem, error) {
	qb := sqlr.NewQueryBuilderUpdate()
	for _, opt := range opts {
		if opt != nil {
			opt(qb)
		}
	}

	r.transformerEntity = entity
	r.updateWritePaths = preloadRelationsFromUpdateBuilder(qb)
	r.updateWriteSyncs = syncPathsFromUpdateBuilder(qb)

	return r.updatedEntity, nil
}

func (r *recordingCrudRepo) Delete(ctx context.Context, id int64, opts ...func(qb *sqlr.QueryBuilderDelete)) error {
	panic("unexpected Delete call")
}

func (r *recordingCrudRepo) Close() error {
	return nil
}

type recordingCrudTransformer struct {
	renderedEntity *crudTaggedItem
}

func (t *recordingCrudTransformer) TransformCreateInput(ctx context.Context, input *crudTaggedCreateInput) (*crudTaggedItem, error) {
	panic("unexpected TransformCreateInput call")
}

func (t *recordingCrudTransformer) TransformUpdateInput(ctx context.Context, entity *crudTaggedItem, input *crudTaggedUpdateInput) (*crudTaggedItem, error) {
	entity.Name = input.Name

	return entity, nil
}

func (t *recordingCrudTransformer) RenderEntityResponse(ctx context.Context, entity *crudTaggedItem) (httpserver.Response, error) {
	t.renderedEntity = entity

	return httpserver.NewStatusResponse(200), nil
}

func (t *recordingCrudTransformer) RenderQueryResponse(ctx context.Context, entities []crudTaggedItem) (httpserver.Response, error) {
	panic("unexpected RenderQueryResponse call")
}

func TestHandlerCrud_HandleUpdateUsesUpdateRehydration(t *testing.T) {
	tags, err := parseEntityBuilderTags[crudTaggedItem]()
	require.NoError(t, err)

	repo := &recordingCrudRepo{
		readEntity: &crudTaggedItem{
			Entity: sqlr.Entity[int64]{Id: 1},
			Name:   "before",
		},
		updatedEntity: &crudTaggedItem{
			Entity: sqlr.Entity[int64]{Id: 1},
			Name:   "after",
			Child:  crudTaggedChild{Entity: sqlr.Entity[int64]{Id: 10}, Name: "child"},
			Tags:   []crudTaggedChild{{Entity: sqlr.Entity[int64]{Id: 11}, Name: "tag"}},
		},
	}
	transformer := &recordingCrudTransformer{}
	handler := &HandlerCrud[int64, crudTaggedItem, crudTaggedCreateInput, crudTaggedUpdateInput]{
		repo:               repo,
		transformer:        transformer,
		builderUpdateRead:  composeBuilders(builderUpdateReadFromTags(tags)),
		builderUpdateWrite: composeBuilders(builderUpdateWriteFromTags(tags)),
	}

	_, err = handler.HandleUpdate(context.Background(), 1, &crudTaggedUpdateInput{Name: "changed"})
	require.NoError(t, err)
	require.Equal(t, 1, repo.readCalls)
	require.Equal(t, []string{"Child", "Tags"}, repo.updateReadPaths)
	require.Equal(t, []string{"Child", "Tags"}, repo.updateWritePaths)
	require.Equal(t, []string{"Tags"}, repo.updateWriteSyncs)
	require.Same(t, repo.readEntity, repo.transformerEntity)
	require.Equal(t, "changed", repo.transformerEntity.Name)
	require.Same(t, repo.updatedEntity, transformer.renderedEntity)
}

func preloadRelationsFromReadBuilder(qb *sqlr.QueryBuilderRead) []string {
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
