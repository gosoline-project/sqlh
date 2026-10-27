package sqlh

import (
	"database/sql"
	"database/sql/driver"
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"

	"github.com/gosoline-project/sqlc"
	"github.com/gosoline-project/sqlr"
	"github.com/justtrackio/gosoline/pkg/validation"
)

var (
	queryIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
	queryGoPath     = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*$`)
)

type queryField struct {
	column   string
	relation string
}

type queryFieldResolver struct{ fields map[string]queryField }

type resolvedQuery struct {
	filter      *sqlc.Expression
	order       []any
	filterJoins []string
	orderJoins  []string
}

func newQueryFieldResolver(entity reflect.Type, schema *sqlr.EntitySchema, configured map[string]string) (*queryFieldResolver, error) {
	entity = indirectQueryType(entity)
	if configured == nil {
		return newAutomaticQueryFieldResolver(entity, schema)
	}

	resolver := &queryFieldResolver{fields: make(map[string]queryField)}
	origins := make(map[string]string)
	for _, public := range slices.Sorted(maps.Keys(configured)) {
		path := configured[public]
		field, err := resolveQueryGoPath(entity, schema, path)
		if err != nil {
			return nil, fmt.Errorf("query field %q: %w", public, err)
		}
		if err := resolver.add(public, path, field, origins); err != nil {
			return nil, err
		}
	}

	return resolver, nil
}

func newAutomaticQueryFieldResolver(entity reflect.Type, schema *sqlr.EntitySchema) (*queryFieldResolver, error) {
	resolver := &queryFieldResolver{fields: make(map[string]queryField)}
	origins := make(map[string]string)
	// Go visibility is resolved before aliases: hidden and ambiguous promoted
	// names do not participate, but aliases never silently shadow another field.
	for _, member := range reflect.VisibleFields(entity) {
		if !member.IsExported() {
			continue
		}
		column, ok := queryColumnAtIndex(schema, member.Index, member.Type)
		if !ok {
			continue
		}
		name, err := automaticQueryFieldName(member)
		if err != nil {
			return nil, err
		}
		if name == "-" {
			continue
		}
		field, err := queryMappedField(schema.TableName, column, "")
		if err != nil {
			return nil, err
		}
		if err := resolver.add(name, queryFieldPath(entity, member.Index), field, origins); err != nil {
			return nil, err
		}
	}

	return resolver, nil
}

// SQLR's exact physical column index, not a value object's Go representation,
// determines persistence. Collections need database value semantics of their own.
func queryColumnAtIndex(schema *sqlr.EntitySchema, index []int, t reflect.Type) (string, bool) {
	for _, column := range schema.Columns {
		if !slices.Equal(index, column.FieldIndex) {
			continue
		}
		t = indirectQueryType(t)
		switch t.Kind() {
		case reflect.Chan, reflect.Func, reflect.Interface, reflect.UnsafePointer, reflect.Complex64, reflect.Complex128:
			return "", false
		case reflect.Array, reflect.Slice, reflect.Map:
			if t.Kind() == reflect.Slice && t.Elem().Kind() == reflect.Uint8 {
				return column.Name, true
			}
			if !t.Implements(reflect.TypeFor[driver.Valuer]()) && !reflect.PointerTo(t).Implements(reflect.TypeFor[driver.Valuer]()) &&
				!t.Implements(reflect.TypeFor[sql.Scanner]()) && !reflect.PointerTo(t).Implements(reflect.TypeFor[sql.Scanner]()) {
				return "", false
			}
		}

		return column.Name, true
	}

	return "", false
}

func automaticQueryFieldName(field reflect.StructField) (string, error) {
	directives, err := parseSqlhTagValues(strings.TrimSpace(field.Tag.Get(sqlhTagName)), field.Name)
	if err != nil {
		return "", err
	}
	for _, directive := range directives {
		if directive.name == sqlhFieldDirective {
			return directive.value, nil
		}
	}

	return field.Name, nil
}

func queryFieldPath(t reflect.Type, index []int) string {
	parts := make([]string, 0, len(index))
	for _, i := range index {
		field := t.Field(i)
		parts = append(parts, field.Name)
		t = indirectQueryType(field.Type)
	}

	return strings.Join(parts, ".")
}

func (r *queryFieldResolver) add(name, path string, field queryField, origins map[string]string) error {
	key := foldQueryFieldName(name)
	description := fmt.Sprintf("%q (%s)", name, path)
	if previous, exists := origins[key]; exists {
		entries := []string{previous, description}
		slices.Sort(entries)

		return fmt.Errorf("ambiguous query fields %s and %s: names are case-insensitively equivalent", entries[0], entries[1])
	}
	origins[key] = description
	r.fields[key] = field

	return nil
}

// foldQueryFieldName chooses a representative from each Unicode SimpleFold
// cycle, exactly matching strings.EqualFold rather than Unicode lowercasing.
func foldQueryFieldName(name string) string {
	asciiLower := true
	for i := range len(name) {
		if name[i] >= 128 || name[i] >= 'A' && name[i] <= 'Z' {
			asciiLower = false

			break
		}
	}
	if asciiLower {
		return name
	}

	return strings.Map(func(r rune) rune {
		minimum := r
		for next := unicode.SimpleFold(r); next != r; next = unicode.SimpleFold(next) {
			if next < minimum {
				minimum = next
			}
		}
		if minimum >= 'A' && minimum <= 'Z' {
			return minimum + ('a' - 'A')
		}

		return minimum
	}, name)
}

func indirectQueryType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t
}

func queryMappedField(qualifier, column, relation string) (queryField, error) {
	if !queryIdentifier.MatchString(qualifier) || !queryIdentifier.MatchString(column) {
		return queryField{}, fmt.Errorf("unsupported query database identifier %q.%q", qualifier, column)
	}

	return queryField{column: qualifier + "." + column, relation: relation}, nil
}

func resolveQueryGoPath(entity reflect.Type, schema *sqlr.EntitySchema, path string) (queryField, error) {
	if !queryGoPath.MatchString(path) {
		return queryField{}, fmt.Errorf("invalid Go field path %q", path)
	}

	parts := strings.Split(path, ".")
	if relation, ok := schema.Relationships[parts[0]]; ok {
		member, visible := entity.FieldByName(parts[0])
		if visible && slices.Equal(member.Index, relation.FieldIndex) {
			return resolveQueryRelationPath(schema, path, parts)
		}
	}

	return resolveQueryMappedScalar(entity, schema, parts, schema.TableName, "", path)
}

func resolveQueryRelationPath(schema *sqlr.EntitySchema, path string, parts []string) (queryField, error) {
	// Relationships are deliberately limited to SQLR's direct to-one joins.
	relation := schema.Relationships[parts[0]]
	if len(parts) < 2 || (relation.Type != sqlr.HasOne && relation.Type != sqlr.BelongsTo) {
		return queryField{}, fmt.Errorf("unsupported relationship path %q", path)
	}

	related, err := relation.ResolveRelatedSchema()
	if err != nil {
		return queryField{}, err
	}
	if schema.PrimaryKey == nil || related.PrimaryKey == nil {
		return queryField{}, fmt.Errorf("relationship %q requires primary keys", relation.Name)
	}
	for _, identifier := range []string{schema.TableName, schema.PrimaryKey.Name, related.TableName, related.PrimaryKey.Name, relation.ForeignKey, relation.Name} {
		if !queryIdentifier.MatchString(identifier) {
			return queryField{}, fmt.Errorf("unsupported relationship database identifier %q", identifier)
		}
	}

	return resolveQueryMappedScalar(relation.RelatedType, related, parts[1:], relation.Name, relation.Name, path)
}

func resolveQueryMappedScalar(entity reflect.Type, schema *sqlr.EntitySchema, parts []string, qualifier, relation, path string) (queryField, error) {
	index, fieldType, err := queryGoFieldIndex(entity, parts)
	if err != nil {
		return queryField{}, err
	}
	if column, ok := queryColumnAtIndex(schema, index, fieldType); ok {
		return queryMappedField(qualifier, column, relation)
	}

	return queryField{}, fmt.Errorf("field %q is not a mapped scalar", path)
}

func queryGoFieldIndex(t reflect.Type, parts []string) ([]int, reflect.Type, error) {
	var index []int
	for _, part := range parts {
		t = indirectQueryType(t)
		if t.Kind() != reflect.Struct {
			return nil, nil, fmt.Errorf("unsupported Go field path component %q", part)
		}
		field, ok := t.FieldByName(part)
		if !ok || !field.IsExported() {
			return nil, nil, fmt.Errorf("unknown or unexported Go field %q", part)
		}
		index = append(index, field.Index...)
		t = field.Type
	}

	return index, t, nil
}

func (r *queryFieldResolver) resolve(filter JsonFilter, order []JsonOrder) (*resolvedQuery, error) {
	result := &resolvedQuery{}
	converted, err := r.convertFilter(filter, &result.filterJoins)
	if err != nil {
		return nil, validation.NewError(err)
	}
	result.filter, err = converted.ToExpression()
	if err != nil {
		return nil, validation.NewError(fmt.Errorf("filter: %w", err))
	}
	for i, key := range order {
		field, ok := r.fields[foldQueryFieldName(key.Field)]
		if !ok {
			return nil, validation.NewError(fmt.Errorf("orderBy[%d]: unknown query field %q", i, key.Field))
		}
		expression, err := (sqlc.JsonOrder{Column: field.column, Direction: key.Direction}).ToExpression()
		if err != nil {
			return nil, validation.NewError(fmt.Errorf("orderBy[%d]: %w", i, err))
		}
		result.order = append(result.order, expression)
		if field.relation != "" && !slices.Contains(result.filterJoins, field.relation) {
			result.orderJoins = appendUniqueQueryJoin(result.orderJoins, field.relation)
		}
	}

	return result, nil
}

func (r *queryFieldResolver) convertFilter(filter JsonFilter, joins *[]string) (sqlc.JsonFilter, error) {
	converted := sqlc.JsonFilter{Type: filter.Type, Value: filter.Value, Values: filter.Values, From: filter.From, To: filter.To}
	switch filter.Type {
	case "":
		if filter.Field != "" || len(filter.Fields) != 0 || filter.Value != nil || len(filter.Values) != 0 || filter.From != nil || filter.To != nil {
			return converted, fmt.Errorf("filter type is required")
		}

		return converted, nil
	case sqlc.JsonFilterAnd, sqlc.JsonFilterOr, sqlc.JsonFilterNot:
		if filter.Field != "" {
			return converted, fmt.Errorf("boolean filter must not specify a field")
		}
		converted.Fields = make([]sqlc.JsonFilter, len(filter.Fields))
		for i, child := range filter.Fields {
			if child.Type == "" {
				return converted, fmt.Errorf("filter child %d requires a type", i)
			}
			resolved, err := r.convertFilter(child, joins)
			if err != nil {
				return converted, fmt.Errorf("filter child %d: %w", i, err)
			}
			converted.Fields[i] = resolved
		}
	default:
		if len(filter.Fields) != 0 {
			return converted, fmt.Errorf("leaf filter must not contain children")
		}
		field, ok := r.fields[foldQueryFieldName(filter.Field)]
		if !ok {
			return converted, fmt.Errorf("unknown query field %q", filter.Field)
		}
		converted.Column = field.column
		if field.relation != "" {
			*joins = appendUniqueQueryJoin(*joins, field.relation)
		}
	}

	return converted, nil
}

func appendUniqueQueryJoin(joins []string, relation string) []string {
	if !slices.Contains(joins, relation) {
		return append(joins, relation)
	}

	return joins
}

func (q *resolvedQuery) applyFilters(qb *sqlr.QueryBuilderSelect) error {
	for _, relation := range q.filterJoins {
		qb.DefaultJoin(sqlc.JoinLeft, relation)
	}
	if q.filter != nil {
		qb.Where(q.filter)
	}

	return nil
}

func (q *resolvedQuery) applyOrder(qb *sqlr.QueryBuilderSelect) {
	for _, relation := range q.orderJoins {
		qb.DefaultJoin(sqlc.JoinLeft, relation)
	}
	if len(q.order) > 0 {
		qb.OrderBy(q.order...)
	}
}
