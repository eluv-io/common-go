package structured

import (
	"encoding/json"
	"testing"

	"github.com/qluvio/content-fabric/errors"

	"github.com/stretchr/testify/require"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		path     string
		source   interface{}
		expected interface{}
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
			require.NoError(t, err)
			require.Equal(t, tt.expected, res)
		})
		t.Run("pass-as-reference_path["+tt.path+"]", func(t *testing.T) {
			res, err := Resolve(ParsePath(tt.path, "/"), &tt.source)
			require.NoError(t, err)
			require.Equal(t, tt.expected, res)
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
				"path": "/does-not-exist"},
		},
		{
			path:   "/expensive/does-not-exist",
			source: parse(testJson),
			contains: jm{
				"kind":   errors.K.Invalid,
				"path":   "/expensive/does-not-exist",
				"reason": "element is leaf",
			},
		},
		{
			path:   "/store/does-not-exist/a/b/c",
			source: parse(testJson),
			contains: jm{
				"kind":      errors.K.NotExist,
				"path":      "/store/does-not-exist",
				"full_path": "/store/does-not-exist/a/b/c",
			},
		},
		{
			path:   "/store/books/dummy",
			source: parse(testJson),
			contains: jm{
				"kind":   errors.K.Invalid,
				"path":   "/store/books/dummy",
				"reason": "invalid array index",
			},
		},
		{
			path:   "/store/books/77",
			source: parse(testJson),
			contains: jm{
				"kind":   errors.K.NotExist,
				"path":   "/store/books/77",
				"reason": "array index out of range",
			},
		},
		{
			path:   "/store/books/-1",
			source: parse(testJson),
			contains: jm{
				"kind":   errors.K.NotExist,
				"path":   "/store/books/-1",
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
