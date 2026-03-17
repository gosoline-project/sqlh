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
	createSyncPaths    []string
	readPreloadPaths   []string
	queryPreloadPaths  []string
	updatePreloadPaths []string
	updateSyncPaths    []string
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

	tags.createSyncPaths = uniqueSortedStrings(tags.createSyncPaths)
	tags.readPreloadPaths = uniqueSortedStrings(tags.readPreloadPaths)
	tags.queryPreloadPaths = uniqueSortedStrings(tags.queryPreloadPaths)
	tags.updatePreloadPaths = uniqueSortedStrings(tags.updatePreloadPaths)
	tags.updateSyncPaths = uniqueSortedStrings(tags.updateSyncPaths)

	return tags, nil
}

func collectEntityBuilderTags(t reflect.Type, schema *sqlr.EntitySchema, parentPath []string, tags *entityBuilderTags, visitedTypes map[reflect.Type]struct{}) error {
	for i := range t.NumField() {
		field := t.Field(i)
		fieldPath := append(append([]string(nil), parentPath...), field.Name)
		relationPath := strings.Join(fieldPath, ".")
		tagValue := strings.TrimSpace(field.Tag.Get(sqlhTagName))

		if field.Anonymous {
			if tagValue != "" {
				return fmt.Errorf("field %s: %s tag is not supported on embedded fields", strings.Join(fieldPath, "."), sqlhTagName)
			}

			fieldType := unwrapFieldType(field.Type)
			if fieldType.Kind() == reflect.Struct {
				if _, ok := visitedTypes[fieldType]; ok {
					continue
				}

				visitedTypes[fieldType] = struct{}{}
				if err := collectEntityBuilderTags(fieldType, schema, parentPath, tags, visitedTypes); err != nil {
					delete(visitedTypes, fieldType)

					return err
				}
				delete(visitedTypes, fieldType)
			}

			continue
		}

		rel, ok := schema.Relationships[field.Name]
		if tagValue != "" && !ok {
			return fmt.Errorf("field %s: %s tag requires an association field", relationPath, sqlhTagName)
		}

		if tagValue != "" {
			if err := applySqlhTagValues(tagValue, relationPath, tags); err != nil {
				return err
			}
		}

		if !ok {
			continue
		}

		relSchema, err := rel.ResolveRelatedSchema()
		if err != nil {
			return fmt.Errorf("field %s: failed to resolve relation schema: %w", relationPath, err)
		}

		relType := rel.RelatedType

		if _, ok := visitedTypes[relType]; ok {
			continue
		}

		visitedTypes[relType] = struct{}{}
		if err := collectEntityBuilderTags(relType, relSchema, fieldPath, tags, visitedTypes); err != nil {
			delete(visitedTypes, relType)

			return err
		}
		delete(visitedTypes, relType)
	}

	return nil
}

func applySqlhTagValues(tagValue string, relationPath string, tags *entityBuilderTags) error {
	directives := strings.Split(tagValue, ";")
	for _, rawDirective := range directives {
		directive := strings.TrimSpace(rawDirective)
		if directive == "" {
			return fmt.Errorf("field %s: %s tag contains an empty directive", relationPath, sqlhTagName)
		}

		name, valuesRaw, ok := strings.Cut(directive, ":")
		if !ok {
			return fmt.Errorf("field %s: invalid %s tag directive %q", relationPath, sqlhTagName, directive)
		}

		name = strings.TrimSpace(name)
		valuesRaw = strings.TrimSpace(valuesRaw)
		if name == "" {
			return fmt.Errorf("field %s: invalid %s tag directive %q", relationPath, sqlhTagName, directive)
		}

		if valuesRaw == "" {
			return fmt.Errorf("field %s: %s directive %q requires at least one phase", relationPath, sqlhTagName, name)
		}

		values := strings.Split(valuesRaw, ",")
		for _, rawValue := range values {
			value := strings.TrimSpace(rawValue)
			if value == "" {
				return fmt.Errorf("field %s: %s directive %q contains an empty phase", relationPath, sqlhTagName, name)
			}

			switch name {
			case "preload":
				switch value {
				case "read":
					tags.readPreloadPaths = append(tags.readPreloadPaths, relationPath)
				case "query":
					tags.queryPreloadPaths = append(tags.queryPreloadPaths, relationPath)
				case "update":
					tags.updatePreloadPaths = append(tags.updatePreloadPaths, relationPath)
				default:
					return fmt.Errorf("field %s: unsupported %s phase %q for directive %q", relationPath, sqlhTagName, value, name)
				}
			case "sync":
				switch value {
				case "create":
					tags.createSyncPaths = append(tags.createSyncPaths, relationPath)
				case "update":
					tags.updateSyncPaths = append(tags.updateSyncPaths, relationPath)
				default:
					return fmt.Errorf("field %s: unsupported %s phase %q for directive %q", relationPath, sqlhTagName, value, name)
				}
			default:
				return fmt.Errorf("field %s: unknown %s tag directive %q", relationPath, sqlhTagName, name)
			}
		}
	}

	return nil
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
