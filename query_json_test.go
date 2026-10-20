package sqlh

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"
)

type queryJSONRecursive struct {
	*queryJSONRecursive
	ID int
}

func TestQueryJSONFieldsMatchesMarshal(t *testing.T) {
	type plain struct {
		ID      int
		Renamed int `json:"renamed"`
		Hidden  int `json:"-"`
		private int
	}
	type left struct {
		Value int
	}
	type right struct {
		Value int
	}
	type tagged struct {
		Other int `json:"Value"`
	}
	type taggedAgain struct {
		Another int `json:"Value"`
	}
	type middle struct {
		left
	}
	type diamondLeft struct {
		left
	}
	type diamondRight struct {
		left
	}
	type privateScalar int
	type PublicScalar int

	recursive := queryJSONRecursive{queryJSONRecursive: &queryJSONRecursive{ID: 2}, ID: 1}
	require.Equal(t, 2, recursive.queryJSONRecursive.ID)

	tests := []struct {
		name  string
		value any
		want  map[string][]int
	}{
		{
			name:  "exact Go names, explicit tags, and hidden fields",
			value: plain{ID: 1, Renamed: 2, Hidden: 3, private: 4},
			want:  map[string][]int{"ID": {0}, "renamed": {1}},
		},
		{
			name: "unexported anonymous struct exposes exported children",
			value: struct {
				plain
			}{plain: plain{ID: 1, Renamed: 2}},
			want: map[string][]int{"ID": {0, 0}, "renamed": {0, 1}},
		},
		{
			name: "anonymous pointer flattens",
			value: struct {
				*plain
			}{plain: &plain{ID: 1, Renamed: 2}},
			want: map[string][]int{"ID": {0, 0}, "renamed": {0, 1}},
		},
		{
			name: "named struct remains a top-level field",
			value: struct {
				Nested plain `json:"nested"`
			}{Nested: plain{ID: 1}},
			want: map[string][]int{"nested": {0}},
		},
		{
			name: "tagged anonymous struct remains a top-level field",
			value: struct {
				plain `json:"nested"`
			}{plain: plain{ID: 1}},
			want: map[string][]int{"nested": {0}},
		},
		{
			name: "ignored anonymous field excludes its children",
			value: struct {
				plain `json:"-"`
			}{plain: plain{ID: 1}},
			want: map[string][]int{},
		},
		{
			name: "shallow field beats deeper tagged field",
			value: struct {
				tagged
				Value int
			}{tagged: tagged{Other: 1}, Value: 2},
			want: map[string][]int{"Value": {1}},
		},
		{
			name: "shallower embedded field wins independent of traversal order",
			value: struct {
				middle
				right
			}{middle: middle{left: left{Value: 1}}, right: right{Value: 2}},
			want: map[string][]int{"Value": {1, 0}},
		},
		{
			name: "equal untagged fields conflict",
			value: struct {
				left
				right
			}{left: left{Value: 1}, right: right{Value: 2}},
			want: map[string][]int{},
		},
		{
			name: "tagged field wins at equal depth",
			value: struct {
				left
				tagged
			}{left: left{Value: 1}, tagged: tagged{Other: 2}},
			want: map[string][]int{"Value": {1, 0}},
		},
		{
			name: "later untagged field cannot replace tagged field",
			value: struct {
				tagged
				left
			}{tagged: tagged{Other: 1}, left: left{Value: 2}},
			want: map[string][]int{"Value": {0, 0}},
		},
		{
			name: "tagged field resolves an earlier untagged conflict",
			value: struct {
				left
				right
				tagged
			}{left: left{Value: 1}, right: right{Value: 2}, tagged: tagged{Other: 3}},
			want: map[string][]int{"Value": {2, 0}},
		},
		{
			name: "equal tagged fields conflict without falling back to untagged",
			value: reflect.New(reflect.StructOf([]reflect.StructField{
				{Name: "Tagged", Type: reflect.TypeFor[tagged](), Anonymous: true},
				{Name: "TaggedAgain", Type: reflect.TypeFor[taggedAgain](), Anonymous: true},
				{Name: "Left", Type: reflect.TypeFor[left](), Anonymous: true},
			})).Elem().Interface(),
			want: map[string][]int{},
		},
		{
			name: "repeated embedded type through a diamond conflicts",
			value: struct {
				diamondLeft
				diamondRight
			}{diamondLeft: diamondLeft{left: left{Value: 1}}, diamondRight: diamondRight{left: left{Value: 2}}},
			want: map[string][]int{},
		},
		{
			name: "options do not turn an unnamed tag into a tagged name",
			value: struct {
				left `json:",omitempty"`
				right
			}{left: left{Value: 1}, right: right{Value: 2}},
			want: map[string][]int{},
		},
		{
			name: "valid tag punctuation, unicode, and JSON options",
			value: struct {
				Count int `json:"count,omitempty,string,omitzero"`
				Label int `json:"é 名!#$%&()*+-./:;<=>?@[]^_{|}~"`
			}{Count: 1, Label: 3},
			want: map[string][]int{"count": {0}, "é 名!#$%&()*+-./:;<=>?@[]^_{|}~": {1}},
		},
		{
			name: "anonymous scalars obey export visibility",
			value: struct {
				privateScalar
				PublicScalar
			}{privateScalar: 1, PublicScalar: 2},
			want: map[string][]int{"PublicScalar": {1}},
		},
		{
			name:  "recursive anonymous pointer terminates",
			value: recursive,
			want:  map[string][]int{"ID": {1}},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fields := queryJSONFields(reflect.TypeOf(test.value))
			require.Equal(t, test.want, fields)

			encoded, err := json.Marshal(test.value)
			require.NoError(t, err)
			var marshaled map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(encoded, &marshaled))
			require.Len(t, marshaled, len(fields))
			for name := range fields {
				require.Contains(t, marshaled, name)
			}
		})
	}
}

func TestQueryJSONFieldsIsIndependentOfRuntimeOmission(t *testing.T) {
	type embedded struct {
		Value int `json:"value,omitempty"`
	}
	type output struct {
		*embedded
		Zero int `json:"zero,omitzero"`
	}

	require.Equal(t, map[string][]int{"value": {0, 0}, "zero": {1}}, queryJSONFields(reflect.TypeFor[output]()))
	require.Equal(t, map[string][]int{"value": {0, 0}, "zero": {1}}, queryJSONFields(reflect.TypeFor[*output]()))
}

func TestQueryJSONFieldsNonStruct(t *testing.T) {
	require.Empty(t, queryJSONFields(nil))
	require.Empty(t, queryJSONFields(reflect.TypeFor[int]()))
}
