package sqlh

import (
	"testing"
	"time"

	"github.com/gosoline-project/sqlr"
	"github.com/stretchr/testify/require"
)

type tagNestedEntity struct {
	sqlr.Entity[int64]
	Title string `db:"title"`
}

type tagChildEntity struct {
	sqlr.Entity[int64]
	ParentID int64             `db:"parent_id"`
	Nested   []tagNestedEntity `db:"-" sqlr:"foreignKey:child_id" sqlh:"preload:create,read;sync:update"`
}

type tagRootEntity struct {
	sqlr.Entity[int64]
	ChildID int64            `db:"child_id"`
	Name    string           `db:"name"`
	Child   tagChildEntity   `db:"-" sqlr:"belongsTo:child_id" sqlh:"preload:create,read,update"`
	Tags    []tagChildEntity `db:"-" sqlr:"foreignKey:root_id" sqlh:"preload:create;sync:create,update,delete"`
	Created time.Time        `db:"created"`
}

type invalidTaggedScalarEntity struct {
	sqlr.Entity[int64]
	Name string `db:"name" sqlh:"preload:read"`
}

type invalidTaggedValueEntity struct {
	sqlr.Entity[int64]
	OccurredAt time.Time `db:"occurred_at" sqlh:"preload:read"`
}

type invalidDirectiveEntity struct {
	sqlr.Entity[int64]
	ChildID int64          `db:"child_id"`
	Child   tagChildEntity `db:"-" sqlr:"belongsTo:child_id" sqlh:"unknown:read"`
}

type invalidDirectivePhaseEntity struct {
	sqlr.Entity[int64]
	ChildID int64          `db:"child_id"`
	Child   tagChildEntity `db:"-" sqlr:"belongsTo:child_id" sqlh:"sync:read"`
}

type autoRelationChild struct {
	sqlr.Entity[int64]
	Name string `db:"name"`
}

type autoRelationParent struct {
	sqlr.Entity[int64]
	ChildID int64             `db:"child_id"`
	Child   autoRelationChild `sqlh:"preload:read"`
}

func TestParseEntityBuilderTags(t *testing.T) {
	tags, err := parseEntityBuilderTags[tagRootEntity]()

	require.NoError(t, err)
	require.ElementsMatch(t, []string{"Child", "Child.Nested", "Tags", "Tags.Nested"}, tags.createPreloadPaths)
	require.Equal(t, []string{"Tags"}, tags.createSyncPaths)
	require.Equal(t, []string{"Tags"}, tags.deleteSyncPaths)
	require.ElementsMatch(t, []string{"Child", "Child.Nested", "Tags.Nested"}, tags.readPreloadPaths)
	require.Empty(t, tags.queryPreloadPaths)
	require.Equal(t, []string{"Child"}, tags.updatePreloadPaths)
	require.ElementsMatch(t, []string{"Child.Nested", "Tags", "Tags.Nested"}, tags.updateSyncPaths)
}

func TestParseEntityBuilderTags_InvalidScalarField(t *testing.T) {
	_, err := parseEntityBuilderTags[invalidTaggedScalarEntity]()

	require.EqualError(t, err, "field Name: sqlh tag requires an association field")
}

func TestParseEntityBuilderTags_InvalidValueTypeField(t *testing.T) {
	_, err := parseEntityBuilderTags[invalidTaggedValueEntity]()

	require.EqualError(t, err, "field OccurredAt: sqlh tag requires an association field")
}

func TestParseEntityBuilderTags_UnknownDirective(t *testing.T) {
	_, err := parseEntityBuilderTags[invalidDirectiveEntity]()

	require.EqualError(t, err, "field Child: unknown sqlh tag directive \"unknown\"")
}

func TestParseEntityBuilderTags_UnsupportedDirectivePhase(t *testing.T) {
	_, err := parseEntityBuilderTags[invalidDirectivePhaseEntity]()

	require.EqualError(t, err, "field Child: unsupported sqlh phase \"read\" for directive \"sync\"")
}

func TestParseEntityBuilderTags_AutoDetectedRelation(t *testing.T) {
	tags, err := parseEntityBuilderTags[autoRelationParent]()

	require.NoError(t, err)
	require.Equal(t, []string{"Child"}, tags.readPreloadPaths)
}
