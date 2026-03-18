//go:build integration && fixtures

package test

import (
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/fixtures"
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
		Name:   "Alice Johnson",
	}),
	fixtures.NewNamedFixture("author_2", Author{
		Entity: sqlr.FixtureEntity[int64](2, "2024-01-02T11:00:00Z", "2024-01-02T11:00:00Z"),
		Name:   "Bob Smith",
	}),
}

var tags = fixtures.NamedFixtures[Tag]{
	fixtures.NewNamedFixture("tag_1", Tag{
		Entity: sqlr.FixtureEntity[int64](1, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   "golang",
	}),
	fixtures.NewNamedFixture("tag_2", Tag{
		Entity: sqlr.FixtureEntity[int64](2, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   "database",
	}),
	fixtures.NewNamedFixture("tag_3", Tag{
		Entity: sqlr.FixtureEntity[int64](3, "2024-01-01T10:00:00Z", "2024-01-01T10:00:00Z"),
		Name:   "testing",
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
		Status:   "published",
		Tags: []Tag{
			fixtureTag(1, "golang"),
			fixtureTag(4, "tutorial"),
		},
	}),
	fixtures.NewNamedFixture("post_2", Post{
		Entity:   sqlr.FixtureEntity[int64](2, "2024-01-10T14:00:00Z", "2024-01-10T14:00:00Z"),
		AuthorID: 1,
		Title:    "Advanced Go Patterns",
		Status:   "published",
		Tags: []Tag{
			fixtureTag(1, "golang"),
			fixtureTag(3, "testing"),
		},
	}),
	fixtures.NewNamedFixture("post_3", Post{
		Entity:   sqlr.FixtureEntity[int64](3, "2024-01-20T16:00:00Z", "2024-01-20T16:00:00Z"),
		AuthorID: 2,
		Title:    "SQL Query Optimization",
		Status:   "draft",
		Tags: []Tag{
			fixtureTag(2, "database"),
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
