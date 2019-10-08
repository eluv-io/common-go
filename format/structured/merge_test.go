package structured

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/qluvio/content-fabric/util/jsonutil"

	"github.com/stretchr/testify/require"
)

func TestMerge(t *testing.T) {
	tests := []struct {
		name     string
		target   string // json
		path     string
		source   string   // json
		sources  []string // json
		expected string   // json
		dbreak   bool
	}{
		{
			name:     "merge nil into nil",
			target:   `null`,
			path:     "/",
			source:   `null`,
			expected: `null`,
		},
		{
			name:     "merge string into nil",
			target:   `null`,
			path:     "/",
			source:   `"a string"`,
			expected: `"a string"`,
		},
		{
			name:     "merge map into nil",
			target:   `null`,
			path:     "/",
			source:   `{"a":"va"}`,
			expected: `{"a":"va"}`,
		},
		{
			name:     "merge map into empty map",
			target:   `{}`,
			path:     "/",
			source:   `{"a":"va"}`,
			expected: `{"a":"va"}`,
		},
		{
			name:     "merge int into nil with path",
			target:   `null`,
			path:     "/path/to/create",
			source:   `99`,
			expected: `{"path":{"to":{"create":99}}}`,
		},
		{
			name:     "merge map into nil with path",
			target:   `null`,
			path:     "/path/to/create",
			source:   `{"a":"va"}`,
			expected: `{"path":{"to":{"create":{"a":"va"}}}}`,
		},
		{
			name:     "overwrite map entry",
			target:   `{"a":"va"}`,
			path:     "/",
			source:   `{"a":"va-new"}`,
			expected: `{"a":"va-new"}`,
		},
		{
			name:     "replace map entry (string)",
			target:   `{"a":"va"}`,
			path:     "/a",
			source:   `"va-new"`,
			expected: `{"a":"va-new"}`,
		},
		{
			name:     "merge map entry (map)",
			target:   `{"a":{"b":"vb"}}`,
			path:     "/a",
			source:   `{"c":"vc"}`,
			expected: `{"a":{"b":"vb","c":"vc"}}`,
		},
		{
			name:     "merge map into map (add)",
			target:   `{"a":"va"}`,
			path:     "/",
			source:   `{"b":"vb"}`,
			expected: `{"a":"va","b":"vb"}`,
		},
		{
			name:     "merge multiple maps into map (add)",
			target:   `{"a":"va"}`,
			path:     "/",
			sources:  []string{`{"b":"vb"}`, `{"c":"vc"}`},
			expected: `{"a":"va","b":"vb","c":"vc"}`,
		},
		{
			name:     "replace map entry with map",
			target:   `{"a":"va"}`,
			path:     "/a",
			source:   `{"b":"vb"}`,
			expected: `{"a":{"b":"vb"}}`,
		},
		{
			name:     "delete map entry",
			target:   `{"a":"va"}`,
			path:     "/",
			source:   `{"a":null}`,
			expected: `{}`,
		},
		{
			name:     "merge array into empty array",
			target:   `[]`,
			path:     "/",
			source:   `["a"]`,
			expected: `["a"]`,
		},
		{
			name:     "merge array into array",
			target:   `["a","b"]`,
			path:     "/",
			source:   `["c","d"]`,
			expected: `["a","b","c","d"]`,
		},
		{
			name:     "merge array into array with duplicates",
			target:   `["a","b"]`,
			path:     "/",
			source:   `["b","c"]`,
			expected: `["a","b","b","c"]`,
		},
		{
			name:     "replace array entry",
			target:   `["a","b"]`,
			path:     "/1",
			source:   `"c"`,
			expected: `["a","c"]`,
			dbreak:   true,
		},
		{
			name:     "replace array entry",
			target:   `["a","b"]`,
			path:     "/0",
			source:   `["b","c"]`,
			expected: `[["b","c"],"b"]`,
		},
		{
			name:     "replace root array with string",
			target:   `["a","b"]`,
			path:     "/",
			source:   `"string"`,
			expected: `"string"`,
		},
		{
			name:     "replace array with string",
			target:   `{"k":["a","b"]}`,
			path:     "/",
			source:   `{"k":"string"}`,
			expected: `{"k":"string"}`,
		},
		{
			name:     "replace map with string",
			target:   `{"k":{"a":"va","b":"vb"}}`,
			path:     "/",
			source:   `{"k":"string"}`,
			expected: `{"k":"string"}`,
		},
		{
			name:     "complex example",
			target:   `{"a":"va","b":["one","two"],"c":"vc","d":{"e":"ve"}}`,
			path:     "/",
			source:   `{"a":"new va","b":["three","four"],"c":null,"d":"vd"}`,
			expected: `{"a":"new va","b":["one","two","three","four"],"d":"vd"}`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name+"|path["+tt.path+"]", func(t *testing.T) {
			var tgt, exp interface{}
			var sources []interface{}
			require.NoError(t, json.Unmarshal([]byte(tt.target), &tgt))
			require.NoError(t, json.Unmarshal([]byte(tt.expected), &exp))
			if len(tt.sources) == 0 {
				tt.sources = append(tt.sources, tt.source)
			}
			for _, s := range tt.sources {
				var src interface{}
				require.NoError(t, json.Unmarshal([]byte(s), &src))
				sources = append(sources, src)
			}
			if tt.dbreak {
				fmt.Println("break!")
			}

			require.NotNil(t, sources)

			fmt.Println(jsonutil.MarshalString(&tgt))
			fmt.Println(jsonutil.MarshalString(&sources[0]))

			res, err := Merge(tgt, ParsePath(tt.path, "/"), sources...)
			require.NoError(t, err)
			require.Equal(t, exp, res)

			fmt.Println(jsonutil.MarshalString(&res))
		})
	}
}

func TestMerge2(t *testing.T) {
	res, err := Merge(nil, Path{"a", "b"}, "just a string")
	require.NoError(t, err)
	require.EqualValues(t, jm{"a": jm{"b": "just a string"}}, res)
}

func TestMergeAtPath(t *testing.T) {
	m1 := jm{"a": jm{"b": jm{"c": "val1"}}}
	m2 := jm{"a": jm{"b": jm{"c": "val2"}}}
	res := merge(Path{"a", "b"}, m1, m2)
	require.EqualValues(t, m2, res)

	m1 = jm{"a": jm{"b": jm{"c": jm{"d": jm{"e": jm{"f": jm{"g": jm{"h": "val1"}}}}}}}}
	m2 = jm{"a": jm{"b": jm{"c": jm{"d": jm{"e": jm{"f": jm{"g": jm{"h": jm{"i": jm{"j": jm{"k": "val2"}}}}}}}}}}}
	res = merge(Path{"a"}, m1, m2)
	require.EqualValues(t, m2, res)

	m1 = jm{"a": jm{"b": jm{"c": jm{"d": jm{"e": jm{"f": jm{"g": jm{"h": jm{"i": jm{"j": jm{"k": "val2"}}}}}}}}}}}
	m2 = jm{"a": jm{"b": jm{"c": jm{"d": jm{"e": jm{"f": jm{"g": jm{"h": "val1"}}}}}}}}
	res = merge(Path{"a"}, m1, m2)
	require.EqualValues(t, m2, res)
}
