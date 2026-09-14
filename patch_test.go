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
	InputById[int]
	Tags  []int `json:"tags"`
	Other []int `json:"other"`
}

type patchTestAssociationEntity struct {
	sqlr.Entity[int]
	Tags  []patchTestAssociationEntity `db:"-" sqlr:"many2many:patch_test_tags" sqlh:"sync:update"`
	Other []patchTestAssociationEntity `db:"-" sqlr:"many2many:patch_test_other" sqlh:"sync:update"`
}

type patchTestBelongsToInput struct {
	InputById[int]
	Author patchTestBelongsToAuthorInput `json:"author"`
}

type patchTestBelongsToAuthorInput struct {
	Profile patchTestBelongsToProfileInput `json:"profile"`
}

type patchTestBelongsToProfileInput struct{}

type patchTestBelongsToEntity struct {
	sqlr.Entity[int]
	AuthorId *int                     `db:"author_id"`
	Author   patchTestBelongsToAuthor `db:"-" sqlr:"belongsTo:author_id"`
}

type patchTestBelongsToAuthor struct {
	sqlr.Entity[int]
	ProfileId *int                      `db:"profile_id"`
	Profile   patchTestBelongsToProfile `db:"-" sqlr:"belongsTo:profile_id"`
}

type patchTestBelongsToProfile struct {
	sqlr.Entity[int]
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

func TestPatchDocumentMergeIntoRemovesMapEntries(t *testing.T) {
	target := map[string]any{
		"nickname": "Ace",
		"name":     "Ada",
		"profile": map[string]any{
			"city":    "Berlin",
			"country": "Germany",
		},
	}
	document, err := NewPatchDocument([]byte(`{"nickname":null,"profile":{"city":null}}`))
	require.NoError(t, err)

	require.NoError(t, document.MergeInto(&target))
	require.Equal(t, map[string]any{
		"name":    "Ada",
		"profile": map[string]any{"country": "Germany"},
	}, target)
}

func TestPatchDocumentMergeIntoClearsEmbeddedValues(t *testing.T) {
	type Nickname string
	type Rating int
	type Labels map[string]string
	rating := Rating(5)
	target := struct {
		Nickname
		*Rating
		Labels
		Name string `json:"name"`
	}{
		Nickname: "Ace",
		Rating:   &rating,
		Labels:   Labels{"status": "active"},
		Name:     "Ada",
	}
	document, err := NewPatchDocument([]byte(`{"Nickname":null,"Rating":null,"Labels":null}`))
	require.NoError(t, err)

	require.NoError(t, document.MergeInto(&target))
	require.Empty(t, target.Nickname)
	require.Nil(t, target.Rating)
	require.Nil(t, target.Labels)
	require.Equal(t, "Ada", target.Name)
}

func TestPatchDocumentMergeIntoPreservesServerOwnedFields(t *testing.T) {
	target := struct {
		*InputById[int]
		Name string `json:"name"`
	}{
		InputById: &InputById[int]{Id: 9},
		Name:      "Ada",
	}
	target.AddForceFilter(func(qb *sqlr.QueryBuilderSelect) {
		qb.Where("account_id = ?", 42)
	})
	document, err := NewPatchDocument([]byte(`{"name":null,"id":100}`))
	require.NoError(t, err)

	require.NoError(t, document.MergeInto(&target))
	require.Empty(t, target.Name)
	require.Equal(t, 9, target.GetId())
	qb := sqlr.NewQueryBuilderSelect()
	applyForceFilters(target, qb)
	query, arguments, err := qb.ToSql()
	require.NoError(t, err)
	require.Contains(t, query, "account_id")
	require.Equal(t, []any{42}, arguments)
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
	input := PatchInput[int]{InputById: InputById[int]{Id: 9}}

	require.NoError(t, json.Unmarshal([]byte(`{"name":"updated"}`), &input))
	require.Equal(t, 9, input.GetId())
	require.True(t, input.Document().Has("name"))
}

func TestSelectPatchAssociationPathsUsesOnlySuppliedSyncPaths(t *testing.T) {
	document, err := NewPatchDocument([]byte(`{"tags":[]}`))
	require.NoError(t, err)

	fields, err := buildPatchAssociationFields[patchTestAssociationInput]([]string{"Tags", "Other"}, nil)
	require.NoError(t, err)

	require.Equal(t, []string{"Tags"}, selectPatchAssociationPaths(document, fields))
}

func TestNormalizePatchAssociationNullsClearsBelongsToForeignKey(t *testing.T) {
	document, err := NewPatchDocument([]byte(`{"author":null}`))
	require.NoError(t, err)

	fields, err := buildPatchAssociationFields[patchTestBelongsToInput]([]string{"Author"}, nil)
	require.NoError(t, err)

	schema, err := sqlr.ParseSchema[patchTestBelongsToEntity]()
	require.NoError(t, err)

	authorId := 7
	entity := &patchTestBelongsToEntity{
		AuthorId: &authorId,
		Author: patchTestBelongsToAuthor{
			Entity: sqlr.Entity[int]{Id: authorId},
		},
	}
	selected := selectPatchAssociationPaths(document, fields)

	require.Equal(t, []string{"Author"}, selected)
	require.NoError(t, normalizePatchAssociationNulls(entity, schema, document, fields, selected))
	require.Nil(t, entity.AuthorId)
	require.Zero(t, entity.Author)
}

func TestNormalizePatchAssociationNullsClearsNestedBelongsToForeignKey(t *testing.T) {
	document, err := NewPatchDocument([]byte(`{"author":{"profile":null}}`))
	require.NoError(t, err)

	fields, err := buildPatchAssociationFields[patchTestBelongsToInput]([]string{"Author.Profile"}, nil)
	require.NoError(t, err)

	schema, err := sqlr.ParseSchema[patchTestBelongsToEntity]()
	require.NoError(t, err)

	authorId := 7
	profileId := 11
	entity := &patchTestBelongsToEntity{
		AuthorId: &authorId,
		Author: patchTestBelongsToAuthor{
			Entity:    sqlr.Entity[int]{Id: authorId},
			ProfileId: &profileId,
			Profile: patchTestBelongsToProfile{
				Entity: sqlr.Entity[int]{Id: profileId},
			},
		},
	}
	selected := selectPatchAssociationPaths(document, fields)

	require.Equal(t, []string{"Author.Profile"}, selected)
	require.NoError(t, normalizePatchAssociationNulls(entity, schema, document, fields, selected))
	require.Equal(t, &authorId, entity.AuthorId)
	require.Nil(t, entity.Author.ProfileId)
	require.Zero(t, entity.Author.Profile)
}

func TestNormalizePatchAssociationNullsLeavesOmittedBelongsToUntouched(t *testing.T) {
	document, err := NewPatchDocument([]byte(`{}`))
	require.NoError(t, err)

	fields, err := buildPatchAssociationFields[patchTestBelongsToInput]([]string{"Author"}, nil)
	require.NoError(t, err)

	schema, err := sqlr.ParseSchema[patchTestBelongsToEntity]()
	require.NoError(t, err)

	authorId := 7
	entity := &patchTestBelongsToEntity{
		AuthorId: &authorId,
		Author: patchTestBelongsToAuthor{
			Entity: sqlr.Entity[int]{Id: authorId},
		},
	}
	selected := selectPatchAssociationPaths(document, fields)

	require.Empty(t, selected)
	require.NoError(t, normalizePatchAssociationNulls(entity, schema, document, fields, selected))
	require.Same(t, &authorId, entity.AuthorId)
	require.Equal(t, authorId, entity.Author.Id)
}

func TestNormalizePatchAssociationNullsCreatesEmptySlice(t *testing.T) {
	document, err := NewPatchDocument([]byte(`{"tags":null}`))
	require.NoError(t, err)

	fields, err := buildPatchAssociationFields[patchTestAssociationInput]([]string{"Tags"}, nil)
	require.NoError(t, err)

	schema, err := sqlr.ParseSchema[patchTestAssociationEntity]()
	require.NoError(t, err)

	entity := &patchTestAssociationEntity{
		Tags: []patchTestAssociationEntity{{Entity: sqlr.Entity[int]{Id: 1}}},
	}
	require.NoError(t, normalizePatchAssociationNulls(entity, schema, document, fields, []string{"Tags"}))
	require.NotNil(t, entity.Tags)
	require.Empty(t, entity.Tags)
}

func TestNormalizePatchAssociationEmptyArrayCreatesEmptySlice(t *testing.T) {
	document, err := NewPatchDocument([]byte(`{"tags":[]}`))
	require.NoError(t, err)

	fields, err := buildPatchAssociationFields[patchTestAssociationInput]([]string{"Tags"}, nil)
	require.NoError(t, err)

	schema, err := sqlr.ParseSchema[patchTestAssociationEntity]()
	require.NoError(t, err)

	entity := &patchTestAssociationEntity{
		Tags: []patchTestAssociationEntity{{Entity: sqlr.Entity[int]{Id: 1}}},
	}
	require.NoError(t, normalizePatchAssociationNulls(entity, schema, document, fields, []string{"Tags"}))
	require.NotNil(t, entity.Tags)
	require.Empty(t, entity.Tags)
}

func TestCrudHandlerPatchAppliesMergePatchInTransaction(t *testing.T) {
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
		func(_ context.Context, entity *crudTestEntity) (*crudTestUpdateInput, error) {
			return &crudTestUpdateInput{
				InputById: InputById[int]{Id: entity.Id},
				Name:      entity.Name,
			}, nil
		},
		func(_ context.Context, entity *crudTestEntity) (crudTestOutput, error) {
			return crudTestOutput{Id: entity.Id, Name: entity.Name}, nil
		},
	)

	handler, err := newCrudHandler(repository, runner, schema, definition)
	require.NoError(t, err)

	input := PatchInput[int]{InputById: InputById[int]{Id: 7}}
	require.NoError(t, json.Unmarshal([]byte(`{"name":""}`), &input))

	output, err := handler.Patch(context.Background(), &input)
	require.NoError(t, err)
	require.Equal(t, crudTestOutput{Id: 7, Name: ""}, output)
}
