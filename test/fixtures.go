//go:build integration && fixtures

package test

import (
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/fixtures"
)

const (
	authorAlice     = "Alice Johnson"
	authorBob       = "Bob Smith"
	tagGolang       = "golang"
	tagDatabase     = "database"
	tagTesting      = "testing"
	tagTutorial     = "tutorial"
	statusPublished = "published"
	statusDraft     = "draft"
	titleAdvancedGo = "Advanced Go Patterns"
)

func fixtureTag(id int64, name string) Tag {
	return Tag{
		Entity: sqlr.Entity[int64]{Id: id},
		Name:   name,
	}
}

var authors = fixtures.NamedFixtures[Author]{
	fixtures.NewNamedFixture("author_1", Author{
		Entity: sqlr.FixtureEntity[int64](1, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   authorAlice,
	}),
	fixtures.NewNamedFixture("author_2", Author{
		Entity: sqlr.FixtureEntity[int64](2, "2024-01-02T11:00:00Z", "2024-01-02T11:00:00Z"),
		Name:   authorBob,
	}),
}

var tags = fixtures.NamedFixtures[Tag]{
	fixtures.NewNamedFixture("tag_1", Tag{
		Entity: sqlr.FixtureEntity[int64](1, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   tagGolang,
	}),
	fixtures.NewNamedFixture("tag_2", Tag{
		Entity: sqlr.FixtureEntity[int64](2, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   tagDatabase,
	}),
	fixtures.NewNamedFixture("tag_3", Tag{
		Entity: sqlr.FixtureEntity[int64](3, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   tagTesting,
	}),
	fixtures.NewNamedFixture("tag_4", Tag{
		Entity: sqlr.FixtureEntity[int64](4, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   "tutorial",
	}),
}

var posts = fixtures.NamedFixtures[Post]{
	fixtures.NewNamedFixture("post_1", Post{
		Entity:   sqlr.FixtureEntity[int64](1, "2024-01-05T10:00:00Z", "2024-01-05T10:00:00Z"),
		AuthorID: 1,
		Title:    "Getting Started with Go",
		Status:   statusPublished,
		Tags: []Tag{
			fixtureTag(1, tagGolang),
			fixtureTag(4, "tutorial"),
		},
	}),
	fixtures.NewNamedFixture("post_2", Post{
		Entity:   sqlr.FixtureEntity[int64](2, "2024-01-10T14:00:00Z", "2024-01-10T14:00:00Z"),
		AuthorID: 1,
		Title:    titleAdvancedGo,
		Status:   statusPublished,
		Tags: []Tag{
			fixtureTag(1, tagGolang),
			fixtureTag(3, tagTesting),
		},
	}),
	fixtures.NewNamedFixture("post_3", Post{
		Entity:   sqlr.FixtureEntity[int64](3, "2024-01-20T16:00:00Z", "2024-01-20T16:00:00Z"),
		AuthorID: 2,
		Title:    "SQL Query Optimization",
		Status:   statusDraft,
		Tags: []Tag{
			fixtureTag(2, tagDatabase),
		},
	}),
}

func Fixtures() fixtures.FixtureSetsFactory {
	return fixtures.NewFixtureSetsFactory(
		sqlr.FixtureSetFactory[int64, Author](authors),
		sqlr.FixtureSetFactory[int64, Tag](tags),
		sqlr.FixtureSetFactory[int64, Post](posts),
	)
}
