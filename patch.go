package sqlh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"unicode"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/gosoline-project/sqlr"
)

// PatchInput is the standard URI and JSON Merge Patch input for one entity.
// The request body is retained as a PatchDocument so SQLH can distinguish
// omitted fields from fields explicitly supplied as null.
type PatchInput[K sqlr.KeyTypes] struct {
	InputByID[K]
	document PatchDocument
}

// UnmarshalJSON stores the complete JSON Merge Patch document. URI binding is
// performed separately by httpserver after JSON binding.
func (i *PatchInput[K]) UnmarshalJSON(data []byte) error {
	document, err := NewPatchDocument(data)
	if err != nil {
		return err
	}

	i.document = document

	return nil
}

// Document returns the JSON Merge Patch document supplied with the request.
func (i PatchInput[K]) Document() PatchDocument {
	return i.document
}

// PatchDocument is an immutable JSON Merge Patch object. It provides presence
// checks for request-aware association synchronization and can merge the
// document into an application-owned update target.
type PatchDocument struct {
	raw    json.RawMessage
	fields map[string]json.RawMessage
}

// NewPatchDocument validates and stores a JSON Merge Patch object.
func NewPatchDocument(data []byte) (PatchDocument, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return PatchDocument{}, fmt.Errorf("patch document is empty")
	}

	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return PatchDocument{}, fmt.Errorf("patch document must be a JSON object: %w", err)
	}
	if fields == nil {
		return PatchDocument{}, fmt.Errorf("patch document must be a JSON object")
	}

	return PatchDocument{
		raw:    bytes.Clone(trimmed),
		fields: fields,
	}, nil
}

// Raw returns a copy of the original JSON Merge Patch document.
func (d PatchDocument) Raw() []byte {
	return bytes.Clone(d.raw)
}

func (d PatchDocument) valid() bool {
	return len(d.raw) > 0 && d.fields != nil
}

// Has reports whether the JSON Merge Patch contains the supplied object path.
// A path is separated by dots, for example "profile.name".
func (d PatchDocument) Has(path string) bool {
	_, ok := d.value(path)

	return ok
}

// IsNull reports whether the JSON Merge Patch explicitly sets the supplied
// object path to JSON null.
func (d PatchDocument) IsNull(path string) bool {
	value, ok := d.value(path)
	if !ok {
		return false
	}

	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func (d PatchDocument) isEmptyArray(path string) bool {
	value, ok := d.value(path)
	if !ok {
		return false
	}

	trimmed := bytes.TrimSpace(value)
	if len(trimmed) < 2 || trimmed[0] != '[' || trimmed[len(trimmed)-1] != ']' {
		return false
	}

	var values []json.RawMessage
	if err := json.Unmarshal(trimmed, &values); err != nil {
		return false
	}

	return len(values) == 0
}

// MergeInto applies the JSON Merge Patch to target using RFC 7396 semantics.
// The target must be a non-nil pointer.
func (d PatchDocument) MergeInto(target any) error {
	if len(d.raw) == 0 {
		return fmt.Errorf("patch document is empty")
	}

	value := reflect.ValueOf(target)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return fmt.Errorf("patch target must be a non-nil pointer")
	}

	before, err := json.Marshal(target)
	if err != nil {
		return fmt.Errorf("failed to marshal patch target: %w", err)
	}

	after, err := jsonpatch.MergePatch(before, d.raw)
	if err != nil {
		return fmt.Errorf("failed to apply JSON Merge Patch: %w", err)
	}

	resetPatchTarget(value.Elem())
	if err = json.Unmarshal(after, target); err != nil {
		return fmt.Errorf("failed to unmarshal merged patch target: %w", err)
	}

	return nil
}

func resetPatchTarget(value reflect.Value) {
	if !value.IsValid() {
		return
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		resetPatchTarget(value.Elem())

		return
	}
	if value.Kind() != reflect.Struct {
		return
	}

	typeOfValue := value.Type()
	for index := 0; index < value.NumField(); index++ {
		field := value.Field(index)
		structField := typeOfValue.Field(index)
		if structField.PkgPath != "" {
			continue
		}

		jsonName, valid := jsonFieldName(structField)
		if !valid {
			continue
		}
		if structField.Anonymous && jsonName == lowerCamel(structField.Name) {
			resetPatchTarget(field)

			continue
		}
		if field.CanSet() {
			field.Set(reflect.Zero(field.Type()))
		}
	}
}

func (d PatchDocument) value(path string) (json.RawMessage, bool) {
	segments := strings.Split(path, ".")
	if len(segments) == 0 || segments[0] == "" || d.fields == nil {
		return nil, false
	}

	fields := d.fields
	for index, segment := range segments {
		value, ok := fields[segment]
		if !ok {
			return nil, false
		}
		if index == len(segments)-1 {
			return value, true
		}

		var nested map[string]json.RawMessage
		if err := json.Unmarshal(value, &nested); err != nil || nested == nil {
			return nil, false
		}

		fields = nested
	}

	return nil, false
}

func buildPatchAssociationFields[IU any](syncPaths []string, overrides map[string]string) (map[string]string, error) {
	fields := make(map[string]string, len(syncPaths)+len(overrides))

	for patchPath, relationPath := range overrides {
		if !containsAssociationPath(syncPaths, relationPath) {
			return nil, fmt.Errorf("patch association %q is not configured with sync:update", relationPath)
		}

		fields[patchPath] = relationPath
	}

	for _, relationPath := range syncPaths {
		if hasRelationPathValue(fields, relationPath) {
			continue
		}

		patchPath := patchJSONPath[IU](relationPath)
		fields[patchPath] = relationPath
	}

	return fields, nil
}

func patchJSONPath[IU any](relationPath string) string {
	segments := strings.Split(relationPath, ".")
	result := make([]string, 0, len(segments))
	t := patchType[IU]()

	for _, segment := range segments {
		jsonName, nestedType := patchJSONSegment(t, segment)
		result = append(result, jsonName)
		t = nestedType
	}

	return strings.Join(result, ".")
}

func patchJSONSegment(t reflect.Type, segment string) (string, reflect.Type) {
	jsonName := lowerCamel(segment)
	if t == nil || t.Kind() != reflect.Struct {
		return jsonName, nil
	}

	field, ok := t.FieldByName(segment)
	if !ok {
		return jsonName, nil
	}

	if name, valid := jsonFieldName(field); valid {
		jsonName = name
	}

	return jsonName, patchNestedType(field.Type)
}

func patchType[T any]() reflect.Type {
	t := reflect.TypeOf((*T)(nil)).Elem()
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}

	return t
}

func patchNestedType(t reflect.Type) reflect.Type {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}

	return t
}

func jsonFieldName(field reflect.StructField) (string, bool) {
	name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
	if name == "-" {
		return "", false
	}
	if name != "" {
		return name, true
	}

	return lowerCamel(field.Name), true
}

func lowerCamel(value string) string {
	if value == "" {
		return value
	}

	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])

	return string(runes)
}

func containsAssociationPath(paths []string, path string) bool {
	for _, current := range paths {
		if current == path {
			return true
		}
	}

	return false
}

func hasRelationPathValue(fields map[string]string, relationPath string) bool {
	for _, current := range fields {
		if current == relationPath {
			return true
		}
	}

	return false
}

func selectPatchAssociationPaths(document PatchDocument, fields map[string]string) []string {
	selected := make([]string, 0)
	for patchPath, relationPath := range fields {
		if document.Has(patchPath) {
			selected = append(selected, relationPath)
		}
	}

	sort.Strings(selected)

	return selected
}

func normalizePatchAssociationNulls[E any](entity *E, document PatchDocument, fields map[string]string, selected []string) error {
	if entity == nil {
		return fmt.Errorf("patch entity is nil")
	}

	root := reflect.ValueOf(entity).Elem()
	for _, relationPath := range selected {
		patchPath, ok := patchPathForRelation(fields, relationPath)
		if !ok || (!document.IsNull(patchPath) && !document.isEmptyArray(patchPath)) {
			continue
		}

		if err := clearPatchRelation(root, relationPath); err != nil {
			return err
		}
	}

	return nil
}

func patchPathForRelation(fields map[string]string, relationPath string) (string, bool) {
	for patchPath, current := range fields {
		if current == relationPath {
			return patchPath, true
		}
	}

	return "", false
}

func clearPatchRelation(root reflect.Value, relationPath string) error {
	current := root
	segments := strings.Split(relationPath, ".")
	for index, segment := range segments {
		for current.Kind() == reflect.Pointer {
			if current.IsNil() {
				return nil
			}
			current = current.Elem()
		}
		if current.Kind() != reflect.Struct {
			return nil
		}

		field := current.FieldByName(segment)
		if !field.IsValid() {
			return fmt.Errorf("patch association %q is not present on the entity", relationPath)
		}
		if index == len(segments)-1 {
			if !field.CanSet() {
				return fmt.Errorf("patch association %q cannot be updated", relationPath)
			}

			if field.Kind() == reflect.Slice {
				field.Set(reflect.MakeSlice(field.Type(), 0, 0))
			} else {
				field.Set(reflect.Zero(field.Type()))
			}

			return nil
		}

		current = field
	}

	return nil
}
