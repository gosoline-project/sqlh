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
	InputById[K]
	// Document is the parsed JSON Merge Patch body supplied with the request.
	Document PatchDocument
}

// UnmarshalJSON stores the complete JSON Merge Patch document. URI binding is
// performed separately by httpserver after JSON binding.
func (i *PatchInput[K]) UnmarshalJSON(data []byte) error {
	document, err := NewPatchDocument(data)
	if err != nil {
		return err
	}

	i.Document = document

	return nil
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

// MergeInto applies the JSON Merge Patch to completeInput using RFC 7396
// semantics. The complete input must be a non-nil pointer and must contain all
// values that an omitted patch field must preserve.
func (d PatchDocument) MergeInto(completeInput any) error {
	if len(d.raw) == 0 {
		return fmt.Errorf("patch document is empty")
	}

	value := reflect.ValueOf(completeInput)
	if !value.IsValid() || value.Kind() != reflect.Pointer || value.IsNil() {
		return fmt.Errorf("complete patch input must be a non-nil pointer")
	}

	before, err := json.Marshal(completeInput)
	if err != nil {
		return fmt.Errorf("failed to marshal complete patch input: %w", err)
	}

	after, err := jsonpatch.MergePatch(before, d.raw)
	if err != nil {
		return fmt.Errorf("failed to apply JSON Merge Patch: %w", err)
	}

	resetPatchInput(value.Elem())
	if err = json.Unmarshal(after, completeInput); err != nil {
		return fmt.Errorf("failed to unmarshal merged patch input: %w", err)
	}

	return nil
}

// json.Unmarshal does not clear map entries or struct fields that are absent
// from its input. Reset the target first so merge-patch nulls cannot leave
// values that the merged document removed. Preserve fields excluded from JSON,
// such as request identity and force filters, because they belong to request
// context rather than the patch body.
func resetPatchInput(value reflect.Value) {
	if !value.IsValid() {
		return
	}
	if value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return
		}
		resetPatchInput(value.Elem())

		return
	}
	if value.Kind() == reflect.Map {
		value.Clear()

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
		fieldType := structField.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}
		if structField.Anonymous && fieldType.Kind() == reflect.Struct && jsonName == lowerCamel(structField.Name) {
			resetPatchInput(field)

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

		patchPath, err := patchJSONPath[IU](relationPath)
		if err != nil {
			return nil, err
		}
		if existingRelationPath, ok := fields[patchPath]; ok && existingRelationPath != relationPath {
			return nil, fmt.Errorf(
				"patch associations %q and %q derive to the same JSON path %q",
				existingRelationPath,
				relationPath,
				patchPath,
			)
		}

		fields[patchPath] = relationPath
	}

	return fields, nil
}

func buildPatchAssociationTriggers(syncPaths []string, triggers map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(triggers))
	for patchPath, relationPath := range triggers {
		if !containsAssociationPath(syncPaths, relationPath) {
			return nil, fmt.Errorf("patch association trigger %q is not configured with sync:update", relationPath)
		}

		result[patchPath] = relationPath
	}

	return result, nil
}

func patchJSONPath[IU any](relationPath string) (string, error) {
	segments := strings.Split(relationPath, ".")
	result := make([]string, 0, len(segments))
	t := patchType[IU]()

	for _, segment := range segments {
		jsonName, nestedType, ok := patchJSONSegment(t, segment)
		if !ok {
			return "", fmt.Errorf("patch association %q has no matching update input field for segment %q", relationPath, segment)
		}

		result = append(result, jsonName)
		t = nestedType
	}

	return strings.Join(result, "."), nil
}

func patchJSONSegment(t reflect.Type, segment string) (string, reflect.Type, bool) {
	if t == nil || t.Kind() != reflect.Struct {
		return "", nil, false
	}

	field, ok := t.FieldByName(segment)
	if !ok {
		return "", nil, false
	}

	jsonName, valid := jsonFieldName(field)
	if !valid {
		return "", nil, false
	}

	return jsonName, patchNestedType(field.Type), true
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

// Select from the original document, not the merged input. MergeInto fills
// omitted fields, but an omitted association must not trigger synchronization.
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

// The update mapper receives only the merged value. It cannot reliably
// distinguish an explicit null or empty-array clear from an omitted association.
// Restore those markers from the original document before SQLR persists the
// entity.
func normalizePatchAssociationNulls[E any](entity *E, schema *sqlr.EntitySchema, document PatchDocument, fields map[string]string, selected []string) error {
	if entity == nil {
		return fmt.Errorf("patch entity is nil")
	}
	if schema == nil {
		return fmt.Errorf("patch entity schema is nil")
	}

	root := reflect.ValueOf(entity).Elem()
	for _, relationPath := range selected {
		patchPath, ok := patchPathForRelation(fields, relationPath)
		if !ok {
			continue
		}

		isNull := document.IsNull(patchPath)
		if !isNull && !document.isEmptyArray(patchPath) {
			continue
		}

		if err := clearPatchRelation(root, schema, relationPath, isNull); err != nil {
			return err
		}
	}

	return nil
}

func clearPatchRelation(root reflect.Value, schema *sqlr.EntitySchema, relationPath string, clearBelongsToForeignKey bool) error {
	segments := strings.Split(relationPath, ".")

	return clearPatchRelationValue(root, schema, segments, relationPath, clearBelongsToForeignKey)
}

func clearPatchRelationValue(value reflect.Value, schema *sqlr.EntitySchema, segments []string, relationPath string, clearBelongsToForeignKey bool) error {
	value = unwrapPatchValue(value)
	if !value.IsValid() {
		return nil
	}

	switch value.Kind() {
	case reflect.Slice, reflect.Array:
		for index := range value.Len() {
			if err := clearPatchRelationValue(value.Index(index), schema, segments, relationPath, clearBelongsToForeignKey); err != nil {
				return err
			}
		}

		return nil
	case reflect.Struct:
	default:
		return nil
	}

	segment := segments[0]
	rel, ok := schema.Relationships[segment]
	if !ok {
		return fmt.Errorf("patch association %q is not present in the entity schema", relationPath)
	}

	field := value.FieldByIndex(rel.FieldIndex)
	if len(segments) > 1 {
		nestedSchema, err := rel.ResolveRelatedSchema()
		if err != nil {
			return fmt.Errorf("failed to resolve patch association %q schema: %w", relationPath, err)
		}

		return clearPatchRelationValue(field, nestedSchema, segments[1:], relationPath, clearBelongsToForeignKey)
	}

	if clearBelongsToForeignKey && rel.Type == sqlr.BelongsTo {
		fkColumn, ok := schema.ColumnByName(rel.ForeignKey)
		if !ok {
			return fmt.Errorf("patch association %q belongs-to foreign key column %q is not present in the entity schema", relationPath, rel.ForeignKey)
		}
		if err := clearPatchField(value.FieldByIndex(fkColumn.FieldIndex), relationPath); err != nil {
			return err
		}
	}

	return clearPatchField(field, relationPath)
}

func patchPathForRelation(fields map[string]string, relationPath string) (string, bool) {
	for patchPath, current := range fields {
		if current == relationPath {
			return patchPath, true
		}
	}

	return "", false
}

func clearPatchField(field reflect.Value, relationPath string) error {
	if !field.IsValid() {
		return fmt.Errorf("patch association %q field is not present on the entity", relationPath)
	}
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

func unwrapPatchValue(value reflect.Value) reflect.Value {
	for value.IsValid() && (value.Kind() == reflect.Pointer || value.Kind() == reflect.Interface) {
		if value.IsNil() {
			return reflect.Value{}
		}

		value = value.Elem()
	}

	return value
}
