package structured

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/util/maputil"

	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		path      string
		source    interface{}
		expected  interface{}
		wantError bool
	}{
		{
			path:     "",
			source:   nil,
			expected: nil,
		},
		{
			path:     "/",
			source:   nil,
			expected: nil,
		},
		{
			path:      "//",
			source:    parse(testJson),
			expected:  nil,
			wantError: true, // there is no "empty" element in the JSON
		},
		{
			path:     "//",
			source:   parse(`{"":"value for empty key"}`),
			expected: "value for empty key",
		},
		{
			path:     "/",
			source:   parse(testJson),
			expected: parse(testJson),
		},
		{
			path:     "/expensive",
			source:   parse(testJson),
			expected: json.Number("10"),
		},
		{
			path:     "/store/bicycle",
			source:   parse(testJson),
			expected: parse(testJson).(jm)["store"].(jm)["bicycle"],
		},
		{
			path:     "/store/bicycle/color",
			source:   parse(testJson),
			expected: "red",
		},
		{
			path:     "/store/books/2/isbn",
			source:   parse(testJson),
			expected: "0-553-21311-3",
		},
	}
	for _, tt := range tests {
		t.Run("path["+tt.path+"]", func(t *testing.T) {
			res, err := Resolve(ParsePath(tt.path, "/"), tt.source)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, res)
			}
		})
		t.Run("pass-as-reference_path["+tt.path+"]", func(t *testing.T) {
			res, err := Resolve(ParsePath(tt.path, "/"), &tt.source)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.expected, res)
			}
		})
	}
}

func TestResolveErrors(t *testing.T) {
	tests := []struct {
		path     string
		source   interface{}
		contains jm
	}{
		{
			path:   "/does-not-exist",
			source: parse(testJson),
			contains: jm{
				"kind": errors.K.NotExist,
				"path": ParsePath("/does-not-exist")},
		},
		{
			path:   "/expensive/does-not-exist",
			source: parse(testJson),
			contains: jm{
				"kind":   errors.K.Invalid,
				"path":   ParsePath("/expensive"),
				"reason": "element is leaf",
			},
		},
		{
			path:   "/store/does-not-exist/a/b/c",
			source: parse(testJson),
			contains: jm{
				"kind":      errors.K.NotExist,
				"path":      ParsePath("/store/does-not-exist"),
				"full_path": ParsePath("/store/does-not-exist/a/b/c"),
			},
		},
		{
			path:   "/store/books/dummy",
			source: parse(testJson),
			contains: jm{
				"kind":   errors.K.Invalid,
				"path":   ParsePath("/store/books/dummy"),
				"reason": "invalid array index",
			},
		},
		{
			path:   "/store/books/77",
			source: parse(testJson),
			contains: jm{
				"kind":   errors.K.NotExist,
				"path":   ParsePath("/store/books/77"),
				"reason": "array index out of range",
			},
		},
		{
			path:   "/store/books/-1",
			source: parse(testJson),
			contains: jm{
				"kind":   errors.K.NotExist,
				"path":   ParsePath("/store/books/-1"),
				"reason": "array index out of range",
			},
		},
	}
	for _, tt := range tests {
		t.Run("path["+tt.path+"]", func(t *testing.T) {
			res, err := Resolve(ParsePath(tt.path, "/"), tt.source)
			// fmt.Printf("returned error: %s\n", err)
			require.Error(t, err)
			require.Nil(t, res)
			switch e := err.(type) {
			case *errors.Error:
				for k, v := range tt.contains {
					require.Equal(t, v, e.Fields[k])
				}

			}
		})
	}
}

func TestResolveSubCreate(t *testing.T) {
	tests := []struct {
		path     string
		source   string
		expected string
	}{
		{
			path:     "/new",
			source:   `{}`,
			expected: `{"new":null}`,
		},
		{
			path:     "/new/path",
			source:   `{}`,
			expected: `{"new":{"path":null}}`,
		},
		{
			path:     "/a/b/new/path",
			source:   `{"a":{"b":{}}}`,
			expected: `{"a":{"b":{"new":{"path":null}}}}`,
		},
		{
			path:     "/a/b/new/path",
			source:   `{"a":{"b":{"c":"d"}}}`,
			expected: `{"a":{"b":{"c":"d","new":{"path":null}}}}`,
		},
	}
	for _, tt := range tests {
		t.Run("path["+tt.path+"]", func(t *testing.T) {
			var src, exp interface{}
			require.NoError(t, json.Unmarshal([]byte(tt.source), &src))
			require.NoError(t, json.Unmarshal([]byte(tt.expected), &exp))
			{
				// ensure path does not exist
				sub, err := resolveSub(ParsePath(tt.path), src, false)
				require.Error(t, err)
				require.Nil(t, sub)
			}
			{
				// resolve with create
				sub, err := resolveSub(ParsePath(tt.path), src, true)
				require.NoError(t, err)
				require.IsType(t, (*subMap)(nil), sub)
				require.Equal(t, exp, src)
			}
			{
				// now resolve again without create and make sure there is no error
				sub, err := resolveSub(ParsePath(tt.path), src, false)
				require.NoError(t, err)
				require.IsType(t, (*subMap)(nil), sub)
			}
		})
	}
}

func TestResolveTransform(t *testing.T) {
	transErr := errors.Str("transformer error")
	tests := []struct {
		path      string
		source    interface{}
		trans     TransformerFn
		want      interface{}
		wantError bool
	}{
		{
			path:   "",
			source: nil,
			trans: func(elem interface{}, path Path, fullPath Path) (interface{}, bool, error) {
				return "transformed!", true, nil
			},
			want: "transformed!",
		},
		{
			path:   "",
			source: nil,
			trans: func(elem interface{}, path Path, fullPath Path) (interface{}, bool, error) {
				return nil, true, transErr
			},
			wantError: true,
		},
		{
			path:   "/a/b/c/d",
			source: parse(`{"a":{"b":"c"}}`),
			trans: func(elem interface{}, path Path, fullPath Path) (interface{}, bool, error) {
				if path.Equals(Path{"a", "b"}) {
					return parse(`{"c":{"d":"e"}}`), true, nil
				}
				return elem, true, nil
			},
			want: "e",
		},
		{
			path:   "/a/a/a/a",
			source: nil,
			trans: func(elem interface{}, path Path, fullPath Path) (interface{}, bool, error) {
				if len(path) < 4 {
					fmt.Println("path: ", path, "returning a->b")
					return maputil.From("a", "b"), true, nil
				}
				fmt.Println("path: ", path, "returning c")
				return "c", true, nil
			},
			want: "c",
		},
		{ // same as above, but stopping resolution immediately
			path:   "/a/a/a/a",
			source: nil,
			trans: func(elem interface{}, path Path, fullPath Path) (interface{}, bool, error) {
				return "c", false, nil
			},
			want: "c",
		},
		{ // resolve works with an inner map[string]string
			path: "/a/b",
			source: map[string]interface{}{
				"a": map[string]string{
					"b": "c",
				},
			},
			want: "c",
		},
		{ // resolve works with an inner []string
			path: "/a/b/0",
			source: map[string]interface{}{
				"a": map[string][]string{
					"b": {"c"},
				},
			},
			want: "c",
		},
		{ // resolve works with an inner map[string][]map[string]string
			path: "/a/b/0/c",
			source: map[string]interface{}{
				"a": map[string][]map[string]string{
					"b": {map[string]string{"c": "d"}},
				},
			},
			want: "d",
		},
		{ // resolve works with a struct
			path: "/Name",
			source: &testStruct{
				Name:        "James Bond",
				Description: "desc",
			},
			want: "James Bond",
		},
		{ // invalid struct field name
			path: "/Unknown",
			source: &testStruct{
				Name:        "James Bond",
				Description: "desc",
			},
			wantError: true,
		},
		{ // struct & struct tags
			path: "/name",
			source: &testStructWithTags{
				Name:        "James Bond",
				Description: "desc",
			},
			want: "James Bond",
		},
		{ // invalid struct & struct tags (tag overrides struct field name)
			path: "/Name",
			source: &testStructWithTags{
				Name:        "James Bond",
				Description: "desc",
			},
			wantError: true,
		},
		{ // struct & struct tags (tag same as field name)
			path: "/Description",
			source: &testStructWithTags{
				Name:        "James Bond",
				Description: "desc",
			},
			want: "desc",
		},
		{ // nested structs
			path: "/nested/name",
			source: &nestedStruct{
				Type: "nested",
				Nested: testStructWithTags{
					Name:        "James Bond",
					Description: "desc",
				},
			},
			want: "James Bond",
		},
	}
	for _, tt := range tests {
		t.Run("path["+tt.path+"]", func(t *testing.T) {
			res, err := Resolve(ParsePath(tt.path, "/"), tt.source, tt.trans)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, res)
			}
		})
		t.Run("pass-as-reference_path["+tt.path+"]", func(t *testing.T) {
			res, err := Resolve(ParsePath(tt.path, "/"), &tt.source, tt.trans)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, res)
			}
		})
	}
}

type testStruct struct {
	Name        string
	Description string
}

type testStructWithTags struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"Description"`
}

type nestedStruct struct {
	Type   string             `json:"type"`
	Nested testStructWithTags `json:"nested"`
}
