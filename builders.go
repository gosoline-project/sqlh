package sqlh

import (
	"strings"

	"github.com/gosoline-project/sqlr"
)

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
	if tags == nil || (len(tags.createPreloadPaths) == 0 && len(tags.createSyncPaths) == 0) {
		return nil
	}

	preloadPaths := append([]string(nil), tags.createPreloadPaths...)
	paths := append([]string(nil), tags.createSyncPaths...)

	return func(qb *sqlr.QueryBuilderCreate) {
		for _, path := range preloadPaths {
			qb.Preload(path)
		}

		qb.SyncAssociation(paths...)
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

func builderPatchWriteFromTags(preloadPaths []string, syncPaths []string, autoSyncPaths []string) func(qb *sqlr.QueryBuilderUpdate) {
	if len(preloadPaths) == 0 && len(syncPaths) == 0 && len(autoSyncPaths) == 0 {
		return nil
	}

	preloads := append([]string(nil), preloadPaths...)
	paths := append([]string(nil), syncPaths...)
	autoPaths := append([]string(nil), autoSyncPaths...)

	return func(qb *sqlr.QueryBuilderUpdate) {
		for _, path := range preloads {
			qb.Preload(path)
		}
		for _, path := range autoPaths {
			if patchAssociationPathSelected(path, paths) {
				continue
			}

			qb.OmitAssociation(path)
		}
		if len(paths) > 0 {
			qb.SyncAssociation(paths...)
		}
	}
}

func patchAssociationPathSelected(path string, selectedPaths []string) bool {
	for _, selectedPath := range selectedPaths {
		if path == selectedPath || strings.HasPrefix(path, selectedPath+".") || strings.HasPrefix(selectedPath, path+".") {
			return true
		}
	}

	return false
}

func builderLookupFromTags(tags *entityBuilderTags) func(qb *sqlr.QueryBuilderSelect) {
	if tags == nil || len(tags.readPreloadPaths) == 0 {
		return nil
	}

	paths := append([]string(nil), tags.readPreloadPaths...)

	return func(qb *sqlr.QueryBuilderSelect) {
		for _, path := range paths {
			qb.Preload(path)
		}
	}
}

func builderForUpdate(qb *sqlr.QueryBuilderSelect) {
	qb.ForUpdate()
}

// builderUpdateLookupFromTags returns preload options for the select-based
// lookup performed before an update.
func builderUpdateLookupFromTags(tags *entityBuilderTags) func(qb *sqlr.QueryBuilderSelect) {
	if tags == nil || len(tags.updatePreloadPaths) == 0 {
		return nil
	}

	paths := append([]string(nil), tags.updatePreloadPaths...)

	return func(qb *sqlr.QueryBuilderSelect) {
		for _, path := range paths {
			qb.Preload(path)
		}
	}
}
