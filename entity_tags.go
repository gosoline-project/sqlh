package sqlh

import (
	"fmt"
	"reflect"
	"slices"
	"strings"

	"github.com/gosoline-project/sqlr"
)

const (
	sqlhTagName        = "sqlh"
	sqlhFieldDirective = "field"
)

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
	value  string
}

func parseEntityBuilderTags[E any](schema *sqlr.EntitySchema) (*entityBuilderTags, error) {
	var zero E
	t := reflect.TypeOf(zero)
	if t == nil {
		t = reflect.TypeOf((*E)(nil)).Elem()
	}

	for t != nil && t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	if t == nil || t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("entity type %T is not a struct", zero)
	}

	tags := &entityBuilderTags{}
	visitedTypes := map[reflect.Type]struct{}{t: {}}
	if err := collectEntityBuilderTags(t, schema, nil, nil, tags, visitedTypes); err != nil {
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

func collectEntityBuilderTags(t reflect.Type, schema *sqlr.EntitySchema, parentPath []string, parentIndex []int, tags *entityBuilderTags, visitedTypes map[reflect.Type]struct{}) error {
	for i := range t.NumField() {
		field := t.Field(i)
		if len(parentIndex) > 0 {
			field.Index = make([]int, len(parentIndex)+1)
			copy(field.Index, parentIndex)
			field.Index[len(parentIndex)] = i
		}
		if err := collectEntityBuilderTagField(field, schema, parentPath, tags, visitedTypes); err != nil {
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

	directives, err := parseSqlhTagValues(tagValue, relationPath)
	if err != nil {
		return err
	}
	for _, directive := range directives {
		if directive.name == sqlhFieldDirective {
			if _, mapped := queryColumnAtIndex(schema, field.Index, field.Type); !field.IsExported() || !mapped {
				return fmt.Errorf("field %s: sqlh field directive requires an exported mapped scalar field", relationPath)
			}

			continue
		}
		if err := validateRelationTagUsage(schema, field, relationPath, tagValue); err != nil {
			return err
		}
		for _, phase := range directive.phases {
			if err := applySqlhTagPhase(directive.name, phase, relationPath, tags); err != nil {
				return err
			}
		}
	}

	return collectRelatedEntityBuilderTags(schema, field, fieldPath, relationPath, tags, visitedTypes)
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
		return collectEntityBuilderTags(fieldType, schema, parentPath, field.Index, tags, visitedTypes)
	})
}

func validateRelationTagUsage(schema *sqlr.EntitySchema, field reflect.StructField, relationPath string, tagValue string) error {
	if tagValue == "" {
		return nil
	}

	if relation, ok := schema.Relationships[field.Name]; ok && slices.Equal(relation.FieldIndex, field.Index) {
		return nil
	}

	return fmt.Errorf("field %s: %s tag requires an association field", relationPath, sqlhTagName)
}

func collectRelatedEntityBuilderTags(schema *sqlr.EntitySchema, field reflect.StructField, fieldPath []string, relationPath string, tags *entityBuilderTags, visitedTypes map[reflect.Type]struct{}) error {
	rel, ok := schema.Relationships[field.Name]
	if !ok || !slices.Equal(rel.FieldIndex, field.Index) {
		return nil
	}

	relSchema, err := rel.ResolveRelatedSchema()
	if err != nil {
		return fmt.Errorf("field %s: failed to resolve relation schema: %w", relationPath, err)
	}

	return visitEntityBuilderType(visitedTypes, rel.RelatedType, func() error {
		return collectEntityBuilderTags(rel.RelatedType, relSchema, fieldPath, nil, tags, visitedTypes)
	})
}

func parseSqlhTagValues(tagValue, fieldPath string) ([]*sqlhTagDirective, error) {
	if tagValue == "" {
		return nil, nil
	}
	var result []*sqlhTagDirective
	fieldSeen := false
	for _, raw := range strings.Split(tagValue, ";") {
		directive := strings.TrimSpace(raw)
		if directive == "" {
			return nil, fmt.Errorf("field %s: %s tag contains an empty directive", fieldPath, sqlhTagName)
		}
		parsed, err := parseSqlhTagDirective(directive, fieldPath)
		if err != nil {
			return nil, err
		}
		if parsed.name == sqlhFieldDirective {
			if fieldSeen {
				return nil, fmt.Errorf("field %s: duplicate sqlh field directive", fieldPath)
			}
			fieldSeen = true
		}
		result = append(result, parsed)
	}

	return result, nil
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

	if name == sqlhFieldDirective {
		if valuesRaw == "" {
			return nil, fmt.Errorf("field %s: sqlh field directive requires a name or -", relationPath)
		}

		return &sqlhTagDirective{name: name, value: valuesRaw}, nil
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
	for t.Kind() == reflect.Pointer {
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
