package structured

import (
	"strings"
	"testing"

	"eluvio/util/jsonutil"

	"github.com/stretchr/testify/assert"
)

func TestVisitPaths(t *testing.T) {
	tests := []struct {
		name   string
		target string
		exp    []string
	}{
		{
			name:   "empty struct",
			target: `{}`,
			exp:    []string{"/"},
		},
		{
			name:   "map with 1 element",
			target: `{"a":"one"}`,
			exp:    []string{"/", "/a"},
		},
		{
			name:   "map with 2 element",
			target: `{"a":"one","b":"two"}`,
			exp:    []string{"/", "/a", "/b"},
		},
		{
			name:   "array with 3 elements",
			target: `["a","b","c"]`,
			exp:    []string{"/", "/0", "/1", "/2"},
		},
		{
			name:   "maps and arrays",
			target: `{"map":{"a":"one","b":"two"},"arr":["a","b"]}`,
			exp:    []string{"/", "/arr", "/arr/0", "/arr/1", "/map", "/map/a", "/map/b"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var val interface{}
			res := make([]string, 0)
			jsonutil.UnmarshalString(test.target, &val)
			Visit(val, true, func(path Path, val interface{}) bool {
				res = append(res, path.String())
				return true
			})
			assert.Equal(t, test.exp, res)
		})
	}
}

func TestReplace(t *testing.T) {
	tests := []struct {
		target              string
		expReplaceA         string
		expReplaceMapsWithA string
	}{
		//{target: ``, expected: ``},
		{
			target:              `{}`,
			expReplaceA:         `{}`,
			expReplaceMapsWithA: `{}`,
		},
		{
			target:              `[]`,
			expReplaceA:         `[]`,
			expReplaceMapsWithA: `[]`,
		},
		{
			target:              `{"a":"one"}`,
			expReplaceA:         `{"a":"one"}`,
			expReplaceMapsWithA: `"x"`,
		},
		{
			target:              `{"one":"a"}`,
			expReplaceA:         `{"one":"x"}`,
			expReplaceMapsWithA: `{"one":"a"}`,
		},
		{
			target:              `{"a":"one","b":"two"}`,
			expReplaceA:         `{"a":"one","b":"two"}`,
			expReplaceMapsWithA: `"x"`,
		},
		{
			target:              `["a","b","c"]`,
			expReplaceA:         `["x","b","c"]`,
			expReplaceMapsWithA: `["a","b","c"]`,
		},
		{
			target:              `{"arr":["a","b"],"map":{"a":"one","b":"two"}}`,
			expReplaceA:         `{"arr":["x","b"],"map":{"a":"one","b":"two"}}`,
			expReplaceMapsWithA: `{"arr":["a","b"],"map":"x"}`,
		},
	}
	for _, test := range tests {
		t.Run("none: "+test.target, func(t *testing.T) {
			var target interface{}
			var expected interface{}
			jsonutil.UnmarshalString(test.target, &target)
			jsonutil.UnmarshalString(test.target, &expected)
			res, err := Replace(target, func(path Path, val interface{}) (replace bool, newVal interface{}, err error) {
				return false, nil, nil
			})
			assert.NoError(t, err)
			assert.EqualValues(t, expected, res)
		})
		t.Run("all elements starting with 'a': "+test.target, func(t *testing.T) {
			var target interface{}
			var expected interface{}
			jsonutil.UnmarshalString(test.target, &target)
			jsonutil.UnmarshalString(test.expReplaceA, &expected)
			res, err := Replace(target, func(path Path, val interface{}) (replace bool, newVal interface{}, err error) {
				switch t := val.(type) {
				case string:
					if strings.HasPrefix(t, "a") {
						return true, "x", nil
					}
				}
				return false, nil, nil
			})
			assert.NoError(t, err)
			assert.EqualValues(t, expected, res)
		})
		t.Run("all maps with an 'a' key: "+test.target, func(t *testing.T) {
			var target interface{}
			var expected interface{}
			jsonutil.UnmarshalString(test.target, &target)
			jsonutil.UnmarshalString(test.expReplaceMapsWithA, &expected)
			res, err := Replace(target, func(path Path, val interface{}) (replace bool, newVal interface{}, err error) {
				switch t := val.(type) {
				case map[string]interface{}:
					if _, found := t["a"]; found {
						return true, "x", nil
					}
				}
				return false, nil, nil
			})
			assert.NoError(t, err)
			assert.EqualValues(t, expected, res)
		})
	}
}
