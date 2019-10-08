package structured

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDelete(t *testing.T) {
	tests := []struct {
		target    string
		path      string
		expected  string
		expectMod bool
	}{
		{
			target:    `{"x":"vx"}`,
			path:      "/",
			expectMod: true,
			expected:  "null",
		},
		{
			target:    `"a string"`,
			path:      "/",
			expectMod: true,
			expected:  "null",
		},
		{
			target:    "null",
			path:      "/",
			expectMod: false,
		},
		{
			target:    "null",
			path:      "/one/two/three",
			expectMod: false,
		},
		{
			target:    testJson,
			path:      "/store/books/2/price",
			expectMod: true,
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
		               "isbn": "0-553-21311-3"
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
			target:    `{"a":"va","b":"vb","c":"vc"}`,
			path:      "/b",
			expectMod: true,
			expected:  `{"a":"va","c":"vc"}`,
		},
		{
			target:    `["a","b","c"]`,
			path:      "/0",
			expectMod: true,
			expected:  `["b","c"]`,
		},
		{
			target:    `["a","b","c"]`,
			path:      "/1",
			expectMod: true,
			expected:  `["a","c"]`,
		},
		{
			target:    `["a","b","c"]`,
			path:      "/2",
			expectMod: true,
			expected:  `["a","b"]`,
		},
		{
			target:    `["a","b","c"]`,
			path:      "/3",
			expectMod: false,
		},
		{
			target:    `{"a":"va", "b": ["a","b","c"]}`,
			path:      "/b/1",
			expectMod: true,
			expected:  `{"a":"va", "b": ["a","c"]}`,
		},
	}
	for _, tt := range tests {
		t.Run("path["+tt.path+"]", func(t *testing.T) {
			var tgt, exp interface{}
			require.NoError(t, json.Unmarshal([]byte(tt.target), &tgt))
			if tt.expectMod {
				require.NoError(t, json.Unmarshal([]byte(tt.expected), &exp))
			} else {
				exp = tgt
			}

			res, mod := Delete(tgt, ParsePath(tt.path))
			require.Equal(t, tt.expectMod, mod)
			require.Equal(t, exp, res)
		})
	}

	res, err := Set(nil, nil, nil)
	require.NoError(t, err)
	require.Equal(t, nil, res)
}

func TestDeleteAllNil(t *testing.T) {
	res, mod := Delete(nil, nil)
	require.False(t, mod)
	require.Equal(t, nil, res)
}
