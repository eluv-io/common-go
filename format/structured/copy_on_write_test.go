package structured

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const cowTarget = `{"a":{"x":"one","y":{"z":"two"}},"b":{"x":"three"},"c":["four",{"x":"five"}]}`

func TestSetCopy(t *testing.T) {
	tests := []struct {
		path   string
		data   any
		exp    string
		shared []string // paths of subtrees that must be shared with the target
	}{
		{
			path:   "/a/y/z",
			data:   "new",
			exp:    `{"a":{"x":"one","y":{"z":"new"}},"b":{"x":"three"},"c":["four",{"x":"five"}]}`,
			shared: []string{"/b", "/c"},
		},
		{
			path:   "/b/new/path",
			data:   "new",
			exp:    `{"a":{"x":"one","y":{"z":"two"}},"b":{"new":{"path":"new"},"x":"three"},"c":["four",{"x":"five"}]}`,
			shared: []string{"/a", "/c"},
		},
		{
			path:   "/c/1/x",
			data:   "new",
			exp:    `{"a":{"x":"one","y":{"z":"two"}},"b":{"x":"three"},"c":["four",{"x":"new"}]}`,
			shared: []string{"/a", "/b"},
		},
		{
			path:   "/a",
			data:   nil,
			exp:    `{"b":{"x":"three"},"c":["four",{"x":"five"}]}`,
			shared: []string{"/b", "/c"},
		},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			target := parse(cowTarget)

			res, err := SetCopy(target, ParsePath(test.path), test.data)
			require.NoError(t, err)

			require.Equal(t, parse(test.exp), res)
			require.Equal(t, parse(cowTarget), target)
			requireShared(t, target, res, test.shared...)
		})
	}
}

func TestDeleteCopy(t *testing.T) {
	tests := []struct {
		path     string
		exp      string
		modified bool
		shared   []string // paths of subtrees that must be shared with the target
	}{
		{
			path:     "/a/y/z",
			exp:      `{"a":{"x":"one","y":{}},"b":{"x":"three"},"c":["four",{"x":"five"}]}`,
			modified: true,
			shared:   []string{"/b", "/c"},
		},
		{
			path:     "/c/0",
			exp:      `{"a":{"x":"one","y":{"z":"two"}},"b":{"x":"three"},"c":[{"x":"five"}]}`,
			modified: true,
			shared:   []string{"/a", "/b"},
		},
		{
			path:     "/b/does/not/exist",
			exp:      cowTarget,
			modified: false,
			shared:   []string{"/", "/a", "/b", "/c"},
		},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			target := parse(cowTarget)

			res, modified := DeleteCopy(target, ParsePath(test.path))
			require.Equal(t, test.modified, modified)

			require.Equal(t, parse(test.exp), res)
			require.Equal(t, parse(cowTarget), target)
			requireShared(t, target, res, test.shared...)
		})
	}

	t.Run("shifted slice element", func(t *testing.T) {
		target := parse(cowTarget)
		res, _ := DeleteCopy(target, ParsePath("/c/0"))
		v1, _ := Resolve(ParsePath("/c/1"), target)
		v2, _ := Resolve(ParsePath("/c/0"), res)
		require.Equal(t, reflect.ValueOf(v1).Pointer(), reflect.ValueOf(v2).Pointer())
	})

	t.Run("nil target", func(t *testing.T) {
		res, modified := DeleteCopy(nil, ParsePath("/a"))
		require.False(t, modified)
		require.Nil(t, res)
	})
}

func TestReplaceCopy(t *testing.T) {
	replaceString := func(old, new string) ReplaceFn {
		return func(path Path, val any) (bool, any, error) {
			if val == old {
				return true, new, nil
			}
			return false, nil, nil
		}
	}

	tests := []struct {
		name   string
		fn     ReplaceFn
		exp    string
		shared []string // paths of subtrees that must be shared with the target
	}{
		{
			name:   "nothing replaced",
			fn:     replaceString("does not exist", "new"),
			exp:    cowTarget,
			shared: []string{"/", "/a", "/a/y", "/b", "/c", "/c/1"},
		},
		{
			name:   "deep map element",
			fn:     replaceString("two", "new"),
			exp:    `{"a":{"x":"one","y":{"z":"new"}},"b":{"x":"three"},"c":["four",{"x":"five"}]}`,
			shared: []string{"/b", "/c"},
		},
		{
			name:   "slice element",
			fn:     replaceString("five", "new"),
			exp:    `{"a":{"x":"one","y":{"z":"two"}},"b":{"x":"three"},"c":["four",{"x":"new"}]}`,
			shared: []string{"/a", "/b"},
		},
		{
			name: "map",
			fn: func(path Path, val any) (bool, any, error) {
				if path.String() == "/a" {
					return true, "new", nil
				}
				return false, nil, nil
			},
			exp:    `{"a":"new","b":{"x":"three"},"c":["four",{"x":"five"}]}`,
			shared: []string{"/b", "/c"},
		},
		{
			name: "root",
			fn: func(path Path, val any) (bool, any, error) {
				return true, "new", nil
			},
			exp: `"new"`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			target := parse(cowTarget)

			res, err := ReplaceCopy(target, test.fn)
			require.NoError(t, err)

			require.Equal(t, parse(test.exp), res)
			require.Equal(t, parse(cowTarget), target)
			requireShared(t, target, res, test.shared...)
		})
	}

	t.Run("error", func(t *testing.T) {
		target := parse(cowTarget)
		res, err := ReplaceCopy(target, func(path Path, val any) (bool, any, error) {
			if val == "five" {
				return false, nil, fmt.Errorf("failed at %s", path)
			}
			return false, nil, nil
		})
		require.EqualError(t, err, "failed at /c/1/x")
		require.Nil(t, res)
		require.Equal(t, parse(cowTarget), target)
	})
}

func TestVisitReplacement(t *testing.T) {
	const replacement = `{"x":"lazy","y":{"z":"lazy"}}`

	// newFn replaces /b with a shared replacement structure and continues the visit within it, replacing all "lazy"
	// strings. It records the visited paths.
	newFn := func(shared any, visited *[]string) ReplaceFn {
		return func(path Path, val any) (bool, any, error) {
			*visited = append(*visited, path.String())
			switch {
			case path.String() == "/b":
				return true, VisitReplacement(shared), nil
			case val == "lazy":
				return true, "loaded", nil
			}
			return false, nil, nil
		}
	}
	exp := `{"a":{"x":"one","y":{"z":"two"}},"b":{"x":"loaded","y":{"z":"loaded"}},"c":["four",{"x":"five"}]}`

	t.Run("ReplaceCopy", func(t *testing.T) {
		target := parse(cowTarget)
		shared := parse(replacement)
		var visited []string

		res, err := ReplaceCopy(target, newFn(shared, &visited))
		require.NoError(t, err)

		require.Equal(t, parse(exp), res)
		require.Equal(t, parse(cowTarget), target)
		require.Equal(t, parse(replacement), shared, "replacement must not be modified")
		requireShared(t, target, res, "/a", "/c")
		requireVisitedOnce(t, visited, "/b", "/b/x", "/b/y", "/b/y/z")
	})

	t.Run("Replace", func(t *testing.T) {
		target := parse(cowTarget)
		shared := parse(replacement)
		var visited []string

		res, err := Replace(target, newFn(shared, &visited))
		require.NoError(t, err)

		require.Equal(t, parse(exp), res)
		requireVisitedOnce(t, visited, "/b", "/b/x", "/b/y", "/b/y/z")
	})
}

// Baseline results:
//
// goos: darwin
// goarch: arm64
// pkg: github.com/eluv-io/common-go/format/structured
// cpu: Apple M4 Max
// BenchmarkReplace
// BenchmarkReplace/Replace_(in_place,_after_Copy)
// BenchmarkReplace/Replace_(in_place,_after_Copy)-14         	    1405	    850881 ns/op	 1086382 B/op	   15057 allocs/op
// BenchmarkReplace/ReplaceCopy
// BenchmarkReplace/ReplaceCopy-14                            	    3069	    388551 ns/op	  425654 B/op	   11077 allocs/op
func BenchmarkReplace(b *testing.B) {
	// a structure with 1000 maps of 10 elements each, one link-like map out of every 100
	target := map[string]any{}
	for i := 0; i < 1000; i++ {
		m := map[string]any{}
		for j := 0; j < 10; j++ {
			m[fmt.Sprintf("key%d", j)] = fmt.Sprintf("value%d", j)
		}
		if i%100 == 0 {
			m["link"] = map[string]any{"/": "./meta/somewhere"}
		}
		target[fmt.Sprintf("map%d", i)] = m
	}
	fn := func(path Path, val any) (bool, any, error) {
		if m, ok := val.(map[string]any); ok {
			if lnk, ok := m["/"]; ok {
				return true, strings.ToUpper(lnk.(string)), nil
			}
		}
		return false, nil, nil
	}

	b.Run("Replace (in place, after Copy)", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = Replace(Copy(target), fn)
		}
	})
	b.Run("ReplaceCopy", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			_, _ = ReplaceCopy(target, fn)
		}
	})
}

// requireShared asserts that the maps or slices at the given paths are the same instances in both structures.
func requireShared(t *testing.T, s1, s2 any, paths ...string) {
	for _, p := range paths {
		v1, err := Resolve(ParsePath(p), s1)
		require.NoError(t, err)
		v2, err := Resolve(ParsePath(p), s2)
		require.NoError(t, err)
		require.Equal(t, reflect.ValueOf(v1).Pointer(), reflect.ValueOf(v2).Pointer(), "not shared: %s", p)
	}
}

// requireVisitedOnce asserts that each of the given paths was visited exactly once.
func requireVisitedOnce(t *testing.T, visited []string, paths ...string) {
	for _, p := range paths {
		count := 0
		for _, v := range visited {
			if v == p {
				count++
			}
		}
		require.Equal(t, 1, count, "path %s visited %d times", p, count)
	}
}
