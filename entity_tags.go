package sqlh

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/gosoline-project/sqlr"
)

const sqlhTagName = "sqlh"

type entityBuilderTags struct {
	createPreloadPaths []string
	createSyncPaths    []string
	deleteSyncPaths    []string
	readPreloadPaths   []string
	queryPreloadPaths  []string
	updatePreloadPaths []string
	updateSyncPaths    []string
}

type sqlhTagDirective struct {
	name   string
	phases []string
}

func parseEntityBuilderTags[E any]() (*entityBuilderTags, error) {
	var zero E
	t := reflect.TypeOf(zero)
	for t != nil && t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	if t == nil || t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("entity type %T is not a struct", zero)
	}

	schema, err := sqlr.ParseSchemaType(t)
	if err != nil {
		return nil, err
	}

	tags := &entityBuilderTags{}
	visitedTypes := map[reflect.Type]struct{}{t: {}}
	if err := collectEntityBuilderTags(t, schema, nil, tags, visitedTypes); err != nil {
		return nil, err
	}

	tags.createPreloadPaths = uniqueSortedStrings(tags.createPreloadPaths)
	tags.createSyncPaths = uniqueSortedStrings(tags.createSyncPaths)
	tags.deleteSyncPaths = uniqueSortedStrings(tags.deleteSyncPaths)
	tags.readPreloadPaths = uniqueSortedStrings(tags.readPreloadPaths)
	tags.queryPreloadPaths = uniqueSortedStrings(tags.queryPreloadPaths)
	tags.updatePreloadPaths = uniqueSortedStrings(tags.updatePreloadPaths)
	tags.updateSyncPaths = uniqueSortedStrings(tags.updateSyncPaths)

	return tags, nil
}

func collectEntityBuilderTags(t reflect.Type, schema *sqlr.EntitySchema, parentPath []string, tags *entityBuilderTags, visitedTypes map[reflect.Type]struct{}) error {
	for i := range t.NumField() {
		if err := collectEntityBuilderTagField(t.Field(i), schema, parentPath, tags, visitedTypes); err != nil {
			return err
		}
	}

	return nil
}

func collectEntityBuilderTagField(field reflect.StructField, schema *sqlr.EntitySchema, parentPath []string, tags *entityBuilderTags, visitedTypes map[reflect.Type]struct{}) error {
	fieldPath := appendFieldPath(parentPath, field.Name)
	relationPath := strings.Join(fieldPath, ".")
	tagValue := strings.TrimSpace(field.Tag.Get(sqlhTagName))

	if field.Anonymous {
		return collectEmbeddedEntityBuilderTags(field, schema, parentPath, fieldPath, tags, visitedTypes, tagValue)
	}

	if err := validateRelationTagUsage(schema, field.Name, relationPath, tagValue); err != nil {
		return err
	}

	if tagValue != "" {
		if err := applySqlhTagValues(tagValue, relationPath, tags); err != nil {
			return err
		}
	}

	return collectRelatedEntityBuilderTags(schema, field.Name, fieldPath, relationPath, tags, visitedTypes)
}

func collectEmbeddedEntityBuilderTags(field reflect.StructField, schema *sqlr.EntitySchema, parentPath []string, fieldPath []string, tags *entityBuilderTags, visitedTypes map[reflect.Type]struct{}, tagValue string) error {
	if tagValue != "" {
		return fmt.Errorf("field %s: %s tag is not supported on embedded fields", strings.Join(fieldPath, "."), sqlhTagName)
	}

	fieldType := unwrapFieldType(field.Type)
	if fieldType.Kind() != reflect.Struct {
		return nil
	}

	return visitEntityBuilderType(visitedTypes, fieldType, func() error {
		return collectEntityBuilderTags(fieldType, schema, parentPath, tags, visitedTypes)
	})
}

func validateRelationTagUsage(schema *sqlr.EntitySchema, fieldName string, relationPath string, tagValue string) error {
	if tagValue == "" {
		return nil
	}

	if _, ok := schema.Relationships[fieldName]; ok {
		return nil
	}

	return fmt.Errorf("field %s: %s tag requires an association field", relationPath, sqlhTagName)
}

func collectRelatedEntityBuilderTags(schema *sqlr.EntitySchema, fieldName string, fieldPath []string, relationPath string, tags *entityBuilderTags, visitedTypes map[reflect.Type]struct{}) error {
	rel, ok := schema.Relationships[fieldName]
	if !ok {
		return nil
	}

	relSchema, err := rel.ResolveRelatedSchema()
	if err != nil {
		return fmt.Errorf("field %s: failed to resolve relation schema: %w", relationPath, err)
	}

	return visitEntityBuilderType(visitedTypes, rel.RelatedType, func() error {
		return collectEntityBuilderTags(rel.RelatedType, relSchema, fieldPath, tags, visitedTypes)
	})
}

func applySqlhTagValues(tagValue string, relationPath string, tags *entityBuilderTags) error {
	directives := strings.Split(tagValue, ";")
	for _, rawDirective := range directives {
		directive := strings.TrimSpace(rawDirective)
		if directive == "" {
			return fmt.Errorf("field %s: %s tag contains an empty directive", relationPath, sqlhTagName)
		}

		parsedDirective, err := parseSqlhTagDirective(directive, relationPath)
		if err != nil {
			return err
		}

		for _, phase := range parsedDirective.phases {
			if err := applySqlhTagPhase(parsedDirective.name, phase, relationPath, tags); err != nil {
				return err
			}
		}
	}

	return nil
}

func parseSqlhTagDirective(directive string, relationPath string) (*sqlhTagDirective, error) {
	name, valuesRaw, ok := strings.Cut(directive, ":")
	if !ok {
		return nil, fmt.Errorf("field %s: invalid %s tag directive %q", relationPath, sqlhTagName, directive)
	}

	name = strings.TrimSpace(name)
	valuesRaw = strings.TrimSpace(valuesRaw)
	if name == "" {
		return nil, fmt.Errorf("field %s: invalid %s tag directive %q", relationPath, sqlhTagName, directive)
	}

	if valuesRaw == "" {
		return nil, fmt.Errorf("field %s: %s directive %q requires at least one phase", relationPath, sqlhTagName, name)
	}

	phases := strings.Split(valuesRaw, ",")
	for i, rawPhase := range phases {
		phase := strings.TrimSpace(rawPhase)
		if phase == "" {
			return nil, fmt.Errorf("field %s: %s directive %q contains an empty phase", relationPath, sqlhTagName, name)
		}

		phases[i] = phase
	}

	return &sqlhTagDirective{name: name, phases: phases}, nil
}

func applySqlhTagPhase(name string, phase string, relationPath string, tags *entityBuilderTags) error {
	switch name {
	case "preload":
		return applySqlhPreloadPhase(phase, relationPath, tags)
	case "sync":
		return applySqlhSyncPhase(phase, relationPath, tags)
	default:
		return fmt.Errorf("field %s: unknown %s tag directive %q", relationPath, sqlhTagName, name)
	}
}

func applySqlhPreloadPhase(phase string, relationPath string, tags *entityBuilderTags) error {
	switch phase {
	case "create":
		tags.createPreloadPaths = append(tags.createPreloadPaths, relationPath)
	case "read":
		tags.readPreloadPaths = append(tags.readPreloadPaths, relationPath)
	case "query":
		tags.queryPreloadPaths = append(tags.queryPreloadPaths, relationPath)
	case "update":
		tags.updatePreloadPaths = append(tags.updatePreloadPaths, relationPath)
	default:
		return fmt.Errorf("field %s: unsupported %s phase %q for directive %q", relationPath, sqlhTagName, phase, "preload")
	}

	return nil
}

func applySqlhSyncPhase(phase string, relationPath string, tags *entityBuilderTags) error {
	switch phase {
	case "create":
		tags.createSyncPaths = append(tags.createSyncPaths, relationPath)
	case "delete":
		tags.deleteSyncPaths = append(tags.deleteSyncPaths, relationPath)
	case "update":
		tags.updateSyncPaths = append(tags.updateSyncPaths, relationPath)
	default:
		return fmt.Errorf("field %s: unsupported %s phase %q for directive %q", relationPath, sqlhTagName, phase, "sync")
	}

	return nil
}

func appendFieldPath(parentPath []string, fieldName string) []string {
	return append(append([]string(nil), parentPath...), fieldName)
}

func visitEntityBuilderType(visitedTypes map[reflect.Type]struct{}, t reflect.Type, fn func() error) error {
	if _, ok := visitedTypes[t]; ok {
		return nil
	}

	visitedTypes[t] = struct{}{}
	defer delete(visitedTypes, t)

	return fn()
}

func unwrapFieldType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}

	return t
}

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}

	result := slices.Clone(values)
	slices.Sort(result)

	return slices.Compact(result)
}
