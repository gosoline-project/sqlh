package sqlh

import (
	"reflect"
	"strings"
	"unicode"
)

type queryJSONCandidate struct {
	index     []int
	tagged    bool
	ambiguous bool
}

// queryJSONFields discovers field names, not runtime values: omitempty and
// omitzero do not remove fields, and embedded pointers need not be populated.
func queryJSONFields(t reflect.Type) map[string][]int {
	fields := make(map[string]queryJSONCandidate)
	if t != nil {
		queryJSONVisit(t, nil, make(map[reflect.Type]bool), fields)
	}

	visible := make(map[string][]int, len(fields))
	for name, field := range fields {
		if !field.ambiguous {
			visible[name] = field.index
		}
	}

	return visible
}

func queryJSONVisit(t reflect.Type, path []int, ancestors map[reflect.Type]bool, fields map[string]queryJSONCandidate) {
	if t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || ancestors[t] {
		return
	}
	ancestors[t] = true
	defer delete(ancestors, t)
	index := make([]int, len(path)+1)
	copy(index, path)

	for i := range t.NumField() {
		field := t.Field(i)
		name, tagged, embedded := queryJSONField(field)
		index[len(path)] = i
		if embedded {
			queryJSONVisit(field.Type, index, ancestors, fields)
		} else if name != "" {
			queryJSONRecord(fields, name, index, tagged)
		}
	}
}

func queryJSONField(field reflect.StructField) (name string, tagged bool, embedded bool) {
	fieldType := field.Type
	if fieldType.Kind() == reflect.Pointer {
		fieldType = fieldType.Elem()
	}
	if !field.IsExported() && (!field.Anonymous || fieldType.Kind() != reflect.Struct) {
		return "", false, false
	}
	tag := field.Tag.Get("json")
	if tag == "-" {
		return "", false, false
	}
	if name = queryJSONTagName(tag); name != "" {
		return name, true, false
	}
	if field.Anonymous && fieldType.Kind() == reflect.Struct {
		return "", false, true
	}

	return field.Name, false, false
}

func queryJSONRecord(fields map[string]queryJSONCandidate, name string, index []int, tagged bool) {
	previous, found := fields[name]
	if !found || len(index) < len(previous.index) ||
		(len(index) == len(previous.index) && tagged && !previous.tagged) {
		fields[name] = queryJSONCandidate{index: append([]int(nil), index...), tagged: tagged}
	} else if len(index) == len(previous.index) && tagged == previous.tagged {
		previous.ambiguous = true
		fields[name] = previous
	}
}

// queryJSONTagName follows Go 1.27's encoding/json tag-name parsing. Ordinary
// names accept Unicode and punctuation. Reserved quoting characters trigger
// the legacy option-token parser, which consumes an identifier prefix.
func queryJSONTagName(tag string) string {
	name, _, _ := strings.Cut(tag, ",")
	if !strings.ContainsAny(name, "\\'\"`") {
		return name
	}
	for i, r := range name {
		if i == 0 && r != '_' && !unicode.IsLetter(r) {
			return ""
		}
		if r != '_' && !unicode.IsLetter(r) && !unicode.IsDigit(r) {
			return name[:i]
		}
	}

	return name
}
