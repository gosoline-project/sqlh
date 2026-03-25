package sqlh

import "github.com/gosoline-project/sqlr"

func composeBuilders[QB any](builders ...func(qb QB)) func(qb QB) {
	return func(qb QB) {
		for _, builder := range builders {
			if builder == nil {
				continue
			}

			builder(qb)
		}
	}
}

func builderCreateFromTags(tags *entityBuilderTags) func(qb *sqlr.QueryBuilderCreate) {
	if tags == nil || len(tags.createSyncPaths) == 0 {
		return nil
	}

	paths := append([]string(nil), tags.createSyncPaths...)

	return func(qb *sqlr.QueryBuilderCreate) {
		qb.SyncAssociation(paths...)
	}
}

func builderReadFromTags(tags *entityBuilderTags) func(qb *sqlr.QueryBuilderRead) {
	if tags == nil || len(tags.readPreloadPaths) == 0 {
		return nil
	}

	paths := append([]string(nil), tags.readPreloadPaths...)

	return func(qb *sqlr.QueryBuilderRead) {
		for _, path := range paths {
			qb.Preload(path)
		}
	}
}

func builderQueryFromTags(tags *entityBuilderTags) func(qb *sqlr.QueryBuilderSelect) {
	if tags == nil || len(tags.queryPreloadPaths) == 0 {
		return nil
	}

	paths := append([]string(nil), tags.queryPreloadPaths...)

	return func(qb *sqlr.QueryBuilderSelect) {
		for _, path := range paths {
			qb.Preload(path)
		}
	}
}

func builderDeleteFromTags(tags *entityBuilderTags) func(qb *sqlr.QueryBuilderDelete) {
	if tags == nil || len(tags.deleteSyncPaths) == 0 {
		return nil
	}

	paths := append([]string(nil), tags.deleteSyncPaths...)

	return func(qb *sqlr.QueryBuilderDelete) {
		qb.SyncAssociation(paths...)
	}
}

func builderUpdateReadFromTags(tags *entityBuilderTags) func(qb *sqlr.QueryBuilderRead) {
	if tags == nil || len(tags.updatePreloadPaths) == 0 {
		return nil
	}

	paths := append([]string(nil), tags.updatePreloadPaths...)

	return func(qb *sqlr.QueryBuilderRead) {
		for _, path := range paths {
			qb.Preload(path)
		}
	}
}

func builderUpdateWriteFromTags(tags *entityBuilderTags) func(qb *sqlr.QueryBuilderUpdate) {
	if tags == nil || (len(tags.updateSyncPaths) == 0 && len(tags.updatePreloadPaths) == 0) {
		return nil
	}

	preloadPaths := append([]string(nil), tags.updatePreloadPaths...)
	paths := append([]string(nil), tags.updateSyncPaths...)

	return func(qb *sqlr.QueryBuilderUpdate) {
		for _, path := range preloadPaths {
			qb.Preload(path)
		}

		qb.SyncAssociation(paths...)
	}
}
