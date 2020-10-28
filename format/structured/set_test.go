package structured

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/util/jsonutil"
)

func TestSet(t *testing.T) {
	tests := []struct {
		target            string
		path              string
		val               string
		expected          string
		expectedEvenIfNil string
		expectError       bool
	}{
		{
			target:   `{"x":"vx"}`,
			path:     "/",
			val:      `{"a":"va"}`,
			expected: `{"a":"va"}`,
		},
		{
			target:   "null",
			path:     "/",
			val:      `"val"`,
			expected: `"val"`,
		},
		{
			target:   "null",
			path:     "/",
			val:      `"null"`,
			expected: `"null"`,
		},
		{
			target:   "null",
			path:     "/",
			val:      `{"a":"va"}`,
			expected: `{"a":"va"}`,
		},
		{
			target:   "null",
			path:     "/one/two/three",
			val:      `{"a":"va"}`,
			expected: `{"one":{"two":{"three":{"a":"va"}}}}`,
		},
		{
			target: testJson,
			path:   "/store/books/2/price",
			val:    `6.49`,
			expected: `{
    "store": {
        "books": [
            {
                "category": "reference",
                "author": "Nigel Rees",
                "title": "Sayings of the Century",
                "price": 8.95
            },
            {
                "category": "fiction",
                "author": "Evelyn Waugh",
                "title": "Sword of Honour",
                "price": 12.99
            },
            {
                "category": "fiction",
                "author": "Herman Melville",
                "title": "Moby Dick",
                "isbn": "0-553-21311-3",
                "price": 6.49
            },
            {
                "category": "fiction",
                "author": "J. R. R. Tolkien",
                "title": "The Lord of the Rings",
                "isbn": "0-395-19395-8",
                "price": 22.99
            }
        ],
        "bicycle": {
            "color": "red",
            "price": 19.95
        }
    },
    "expensive": 10
}
`,
		},
		{
			target:   `{"a":"va","b":"vb","c":"vc"}`,
			path:     "/b",
			val:      `null`,
			expected: `{"a":"va","c":"vc"}`,
		},
		{
			target:   `["a","b","c"]`,
			path:     "/0",
			val:      `null`,
			expected: `["b","c"]`,
		},
		{
			target:   `["a","b","c"]`,
			path:     "/1",
			val:      `null`,
			expected: `["a","c"]`,
		},
		{
			target:   `["a","b","c"]`,
			path:     "/2",
			val:      `null`,
			expected: `["a","b"]`,
		},
		{
			target:      `["a","b","c"]`,
			path:        "/3",
			val:         `null`,
			expectError: true,
		},
		{
			target:   `{"a":"va", "b": ["a","b","c"]}`,
			path:     "/b/1",
			val:      `null`,
			expected: `{"a":"va", "b": ["a","c"]}`,
		},
		{
			target:            `{"a":{"b":null}}`,
			path:              "/a/b/c",
			val:               `null`,
			expected:          `{"a":{"b":{}}}`,
			expectedEvenIfNil: `{"a":{"b":{"c":null}}}`,
		},
		{
			target:            `{"a":{"b":null}}`,
			path:              "/a/b/c/d",
			val:               `null`,
			expected:          `{"a":{"b":{"c":{}}}}`,
			expectedEvenIfNil: `{"a":{"b":{"c":{"d":null}}}}`,
		},
	}
	for _, tt := range tests {
		t.Run("path["+tt.path+"]", func(t *testing.T) {
			var tgt, exp, val interface{}
			tgt = jsonutil.UnmarshalStringToAny(tt.target)
			if !tt.expectError {
				exp = jsonutil.UnmarshalStringToAny(tt.expected)
			}
			val = jsonutil.UnmarshalStringToAny(tt.val)
			res, err := Set(&tgt, ParsePath(tt.path), &val)
			if tt.expectError {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				require.Equal(t, exp, res)
			}
			if tt.expectedEvenIfNil != "" {
				tgt = jsonutil.UnmarshalStringToAny(tt.target)
				res, err = SetEvenIfNil(&tgt, ParsePath(tt.path), nil)
				require.NoError(t, err)
				require.Equal(t, jsonutil.UnmarshalStringToAny(tt.expectedEvenIfNil), res)
			}
		})
	}

	res, err := Set(nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, nil, res)
}

func TestAllNil(t *testing.T) {
	res, err := Set(nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, nil, res)
}

func TestNil(t *testing.T) {
	res, err := Set(nil, nil, "val")
	require.NoError(t, err)
	require.Equal(t, "val", res)
}
