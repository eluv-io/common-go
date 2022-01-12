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
		{
			path:     "/store/books/-1/author", // last book
			source:   parse(testJson),
			expected: "J. R. R. Tolkien",
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
					require.Equal(t, v, e.Field(k))
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
		for _, cpy := range []bool{false, true} {
			t.Run(fmt.Sprintf("path[%s] copy[%t]", tt.path, cpy), func(t *testing.T) {
				var src, src2, exp, resFromCreate interface{}
				require.NoError(t, json.Unmarshal([]byte(tt.source), &src))
				require.NoError(t, json.Unmarshal([]byte(tt.source), &src2))
				require.NoError(t, json.Unmarshal([]byte(tt.expected), &exp))
				{
					// ensure path does not exist
					res, err := resolveSub(ParsePath(tt.path), src, false, cpy)
					require.Error(t, err)
					require.Nil(t, res)
					if cpy {
						require.Equal(t, src2, src) // src unchanged!
					}
				}
				{
					// resolve with create
					res, err := resolveSub(ParsePath(tt.path), src, true, cpy)
					require.NoError(t, err)
					require.IsType(t, (*subMap)(nil), res)
					require.Equal(t, exp, res.Root())
					if cpy {
						require.Equal(t, src2, src) // src unchanged!
					}
					resFromCreate = res.Root()
				}
				{
					// now resolve again without create and make sure there is no error
					res, err := resolveSub(ParsePath(tt.path), resFromCreate, false, cpy)
					require.NoError(t, err)
					require.IsType(t, (*subMap)(nil), res)
					if cpy {
						require.Equal(t, src2, src) // src unchanged!
					}
				}
			})
		}
	}
}

func TestResolveTransform(t *testing.T) {
	transErr := errors.Str("transformer error")
	tests := []struct {
		name      string
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
		{
			name: "struct",
			path: "/Name",
			source: &testStruct{
				Name:        "James Bond",
				Description: "desc",
			},
			want: "James Bond",
		},
		{
			name: "invalid struct field name",
			path: "/Unknown",
			source: &testStruct{
				Name:        "James Bond",
				Description: "desc",
			},
			wantError: true,
		},
		{
			name: "struct & struct tags",
			path: "/name",
			source: &ResRestStructWithTags{
				Name:        "James Bond",
				Description: "desc",
			},
			want: "James Bond",
		},
		{
			name: "invalid struct & struct tags (tag overrides struct field name)",
			path: "/Name",
			source: &ResRestStructWithTags{
				Name:        "James Bond",
				Description: "desc",
			},
			wantError: true,
		},
		{
			name: "struct & struct tags (tag same as field name)",
			path: "/Description",
			source: &ResRestStructWithTags{
				Name:        "James Bond",
				Description: "desc",
			},
			want: "desc",
		},
		{
			name: "struct with pointer to struct",
			path: "/nested/name",
			source: &testStructWithPointers{
				&ResRestStructWithTags{
					Name:        "James Bond",
					Description: "desc",
				},
			},
			want: "James Bond",
		},
		{
			name: "struct with nil pointer to struct",
			path: "/nested/name",
			source: &testStructWithPointers{
				nil,
			},
			wantError: true,
		},
		{
			name: "nested struct",
			path: "/nested/name",
			source: &nestedStruct{
				Type: "nested",
				Nested: ResRestStructWithTags{
					Name:        "James Bond",
					Description: "desc",
				},
			},
			want: "James Bond",
		},
		{
			name: "nested anonymous struct",
			path: "/nested/name",
			source: &nestedAnonymousStruct{
				Type: "nested anonymous",
				ResRestStructWithTags: ResRestStructWithTags{
					Name:        "James Bond",
					Description: "desc",
				},
			},
			want: "James Bond",
		},
		{
			name: "nested anonymous squashed struct",
			path: "/name",
			source: &nestedAnonymousSquashedStruct{
				Type: "nested anonymous squashed",
				ResRestStructWithTags: ResRestStructWithTags{
					Name:        "James Bond",
					Description: "desc",
				},
			},
			want: "James Bond",
		},
		{
			name: "nested anonymous squashed struct - access map",
			path: "/key2",
			source: &nestedAnonymousSquashedStruct{
				Type: "nested anonymous squashed",
				ResRestStructWithTags: ResRestStructWithTags{
					Name:        "James Bond",
					Description: "desc",
				},
				AMap: map[string]interface{}{
					"key1": "val1",
					"key2": "val2",
				},
			},
			want: "val2",
		},
	}
	for _, tt := range tests {
		name := tt.name
		if name == "" {
			name = "path[" + tt.path + "]"
		}
		t.Run(name, func(t *testing.T) {
			res, err := Resolve(ParsePath(tt.path, "/"), tt.source, tt.trans)
			if tt.wantError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, tt.want, res)
			}
		})
		t.Run("pass-as-reference:"+name, func(t *testing.T) {
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

type ResRestStructWithTags struct {
	Name        string `json:"name,omitempty"`
	Description string `json:"Description"`
}

type nestedStruct struct {
	Type   string                `json:"type"`
	Nested ResRestStructWithTags `json:"nested"`
}

type nestedAnonymousStruct struct {
	Type                  string `json:"type"`
	ResRestStructWithTags `json:"nested"`
}

type nestedAnonymousSquashedStruct struct {
	Type                  string `json:"type"`
	ResRestStructWithTags `json:",squash"`
	AMap                  map[string]interface{} `json:",squash"`
}

type testStructWithPointers struct {
	*ResRestStructWithTags `json:"nested"`
}
