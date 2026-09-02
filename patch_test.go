package sqlh

import (
	"context"
	"encoding/json"
	"testing"

	sqlcmocks "github.com/gosoline-project/sqlc/mocks"
	"github.com/gosoline-project/sqlr"
	sqlrmocks "github.com/gosoline-project/sqlr/mocks"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type patchTestProfile struct {
	Name string `json:"name"`
	City string `json:"city"`
}

type patchTestTarget struct {
	Name    string           `json:"name"`
	Count   int              `json:"count"`
	Profile patchTestProfile `json:"profile"`
	Tags    []int            `json:"tags"`
	Comment *string          `json:"comment"`
}

type patchTestAssociationInput struct {
	InputByID[int]
	Tags  []int `json:"tags"`
	Other []int `json:"other"`
}

type patchTestAssociationEntity struct {
	sqlr.Entity[int]
	Tags  []patchTestAssociationEntity `db:"-" sqlr:"many2many:patch_test_tags" sqlh:"sync:update"`
	Other []patchTestAssociationEntity `db:"-" sqlr:"many2many:patch_test_other" sqlh:"sync:update"`
}

func TestNewPatchDocumentRejectsNonObjects(t *testing.T) {
	for _, input := range []string{"", "null", "[]", `"value"`} {
		t.Run(input, func(t *testing.T) {
			_, err := NewPatchDocument([]byte(input))

			require.Error(t, err)
		})
	}
}

func TestPatchDocumentMergeIntoUsesJSONMergePatchSemantics(t *testing.T) {
	comment := "keep"
	target := &patchTestTarget{
		Name:    "before",
		Count:   7,
		Profile: patchTestProfile{Name: "Ada", City: "Berlin"},
		Tags:    []int{1, 2},
		Comment: &comment,
	}

	document, err := NewPatchDocument([]byte(`{"name":"after","profile":{"city":"Paris"},"tags":[],"comment":null}`))
	require.NoError(t, err)

	require.NoError(t, document.MergeInto(target))
	require.Equal(t, "after", target.Name)
	require.Equal(t, 7, target.Count)
	require.Equal(t, patchTestProfile{Name: "Ada", City: "Paris"}, target.Profile)
	require.Empty(t, target.Tags)
	require.Nil(t, target.Comment)
}

func TestPatchDocumentTracksPresenceAndNull(t *testing.T) {
	document, err := NewPatchDocument([]byte(`{"tags":null,"profile":{"city":"Paris"}}`))
	require.NoError(t, err)

	require.True(t, document.Has("tags"))
	require.True(t, document.IsNull("tags"))
	require.True(t, document.Has("profile.city"))
	require.False(t, document.Has("profile.name"))
	require.False(t, document.IsNull("missing"))
}

func TestPatchInputStoresDocumentAndRetainsURIIdentity(t *testing.T) {
	input := PatchInput[int]{InputByID: InputByID[int]{ID: 9}}

	require.NoError(t, json.Unmarshal([]byte(`{"name":"updated"}`), &input))
	require.Equal(t, 9, input.GetID())
	require.True(t, input.Document().Has("name"))
}

func TestSelectPatchAssociationPathsUsesOnlySuppliedSyncPaths(t *testing.T) {
	document, err := NewPatchDocument([]byte(`{"tags":[]}`))
	require.NoError(t, err)

	fields, err := buildPatchAssociationFields[patchTestAssociationInput]([]string{"Tags", "Other"}, nil)
	require.NoError(t, err)

	require.Equal(t, []string{"Tags"}, selectPatchAssociationPaths(document, fields))
}

func TestNormalizePatchAssociationNullsCreatesEmptySlice(t *testing.T) {
	document, err := NewPatchDocument([]byte(`{"tags":null}`))
	require.NoError(t, err)

	fields, err := buildPatchAssociationFields[patchTestAssociationInput]([]string{"Tags"}, nil)
	require.NoError(t, err)

	entity := &patchTestAssociationEntity{
		Tags: []patchTestAssociationEntity{{Entity: sqlr.Entity[int]{Id: 1}}},
	}
	require.NoError(t, normalizePatchAssociationNulls(entity, document, fields, []string{"Tags"}))
	require.NotNil(t, entity.Tags)
	require.Empty(t, entity.Tags)
}

func TestNormalizePatchAssociationEmptyArrayCreatesEmptySlice(t *testing.T) {
	document, err := NewPatchDocument([]byte(`{"tags":[]}`))
	require.NoError(t, err)

	fields, err := buildPatchAssociationFields[patchTestAssociationInput]([]string{"Tags"}, nil)
	require.NoError(t, err)

	entity := &patchTestAssociationEntity{
		Tags: []patchTestAssociationEntity{{Entity: sqlr.Entity[int]{Id: 1}}},
	}
	require.NoError(t, normalizePatchAssociationNulls(entity, document, fields, []string{"Tags"}))
	require.NotNil(t, entity.Tags)
	require.Empty(t, entity.Tags)
}

func TestCRUDPatchAppliesMergePatchInTransaction(t *testing.T) {
	repository := sqlrmocks.NewRepositoryTx[int, crudTestEntity](t)
	tx := &transactionTestTx{Tx: sqlcmocks.NewTx(t)}
	tx.EXPECT().Commit().Return(nil).Once()

	repository.EXPECT().Query(mock.Anything, mock.Anything).Return([]crudTestEntity{{
		Entity: sqlr.Entity[int]{Id: 7},
		Name:   "before",
	}}, nil).Once()
	repository.EXPECT().Update(mock.Anything, mock.Anything, mock.Anything).Run(func(_ sqlr.TTx, entity *crudTestEntity, _ ...func(*sqlr.QueryBuilderUpdate)) {
		require.Equal(t, "", entity.Name)
	}).Return(&crudTestEntity{
		Entity: sqlr.Entity[int]{Id: 7},
		Name:   "",
	}, nil).Once()

	runner, err := NewTxRunnerWithClient(transactionTestClient{tx: tx})
	require.NoError(t, err)
	schema, err := sqlr.ParseSchema[crudTestEntity]()
	require.NoError(t, err)

	definition := NewCrudDefinition(
		func(_ context.Context, input *crudTestCreateInput) (*crudTestEntity, error) {
			return &crudTestEntity{Name: input.Name}, nil
		},
		func(_ context.Context, entity *crudTestEntity, input *crudTestUpdateInput) (*crudTestEntity, error) {
			entity.Name = input.Name

			return entity, nil
		},
		func(_ context.Context, entity *crudTestEntity) (crudTestOutput, error) {
			return crudTestOutput{ID: entity.Id, Name: entity.Name}, nil
		},
	)
	definition.PatchInputFromEntity = func(_ context.Context, entity *crudTestEntity) (*crudTestUpdateInput, error) {
		return &crudTestUpdateInput{
			InputByID: InputByID[int]{ID: entity.Id},
			Name:      entity.Name,
		}, nil
	}

	handler, err := newCRUD(repository, runner, schema, definition)
	require.NoError(t, err)

	input := PatchInput[int]{InputByID: InputByID[int]{ID: 7}}
	require.NoError(t, json.Unmarshal([]byte(`{"name":""}`), &input))

	output, err := handler.Patch(context.Background(), &input)
	require.NoError(t, err)
	require.Equal(t, crudTestOutput{ID: 7, Name: ""}, output)
}
