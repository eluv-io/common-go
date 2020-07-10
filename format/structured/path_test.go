package structured

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/util/jsonutil"
)

func TestNewPath(t *testing.T) {
	ss := []string{"z", "b", "c"}
	require.EqualValues(t, NewPath(ss...), NewPath("z", "b", "c"))
	require.EqualValues(t, NewPath(ss...), PathWith("z", ss[1:]...))
}

func TestPathParseAndFormat(t *testing.T) {
	tests := []struct {
		sep    string
		s      string
		p      Path
		oneway bool
	}{
		{sep: "/", s: "", p: nil, oneway: true},
		{sep: "/", s: "/", p: Path{}},
		{sep: "/", s: "//one", p: Path{"", "one"}},
		{sep: "/", s: "/one", p: Path{"one"}},
		{sep: "/", s: "/one/", p: Path{"one"}, oneway: true},
		{sep: "/", s: "/one/two", p: Path{"one", "two"}},
		{sep: "/", s: "/one/two/three", p: Path{"one", "two", "three"}},
		{sep: "/", s: "/one~1two/three", p: Path{"one/two", "three"}},
		{sep: "/", s: "/one~0two/three", p: Path{"one~two", "three"}},
		{sep: "/", s: "/one~0two~1three", p: Path{"one~two/three"}},
		{sep: "/", s: "/one~01two~11three", p: Path{"one~1two/1three"}},
		{sep: ".", s: ".one.two.three", p: Path{"one", "two", "three"}},
	}
	for _, tt := range tests {
		t.Run("parse["+tt.s+"]", func(t *testing.T) {
			p := ParsePath(tt.s, tt.sep)
			equalArrays(t, p, tt.p)
		})
	}
	for _, tt := range tests {
		if !tt.oneway {
			t.Run("format["+tt.s+"]", func(t *testing.T) {
				assert.Equal(t, tt.p.Format(tt.sep), tt.s)
			})
		}
	}
}

func TestPathCopyAppend(t *testing.T) {
	tests := []struct {
		base     string
		app      []string
		expected string
	}{
		{base: "/", app: []string{"one"}, expected: "/one"},
		{base: "/", app: []string{"one", "two"}, expected: "/one/two"},
		{base: "/a/b", app: []string{"one"}, expected: "/a/b/one"},
		{base: "/a/b", app: []string{"one", "two"}, expected: "/a/b/one/two"},
	}
	for _, tt := range tests {
		t.Run("CopyAppend["+tt.expected+"]", func(t *testing.T) {
			base := ParsePath(tt.base)
			copy := base.CopyAppend(tt.app...)
			assert.Equal(t, tt.expected, copy.Format())
			assert.Equal(t, tt.base, base.Format(), "base has changed!")
		})
	}
}

func TestPathContains(t *testing.T) {
	tests := []struct {
		target   string
		contains string
		exp      bool
	}{
		{"/", "/", true},
		{"/a", "/", true},
		{"/a", "/a", true},
		{"/a/b", "/", true},
		{"/a/b", "/a", true},
		{"/a/b", "/a/b", true},
		{"/a/b/c", "/", true},
		{"/a/b/c", "/a", true},
		{"/a/b/c", "/a/b", true},
		{"/a/b/c", "/a/b/c", true},
		{"/", "/foo", false},
		{"/a", "/foo", false},
		{"/a/b", "/foo", false},
		{"/a/b/c", "/b/c", false},
		{"/a", "/a_b", false},
		{"/a_b", "/a", false},
	}
	for _, test := range tests {
		t.Run("["+test.target+"].contains["+test.contains+"]", func(t *testing.T) {
			assert.Equal(t, test.exp, ParsePath(test.target).Contains(ParsePath(test.contains)), "target [%s] contains [%s]", test.target, test.contains)
		})
	}
}

func equalArrays(t *testing.T, actual, expected interface{}) {
	v1 := reflect.ValueOf(actual)
	v2 := reflect.ValueOf(expected)

	if !v1.IsValid() || !v2.IsValid() {
		if v1.IsValid() != v2.IsValid() {
			t.Errorf("arrays differ: actual [%T] valid [%t] <> expected [%T] valid [%t]", v1, v1.IsValid(), v2, v2.IsValid())
			return
		}
	}
	if v1.Type() != v2.Type() {
		t.Errorf("array types differ: actual type [%T] <> expected type [%T]", v1, v2)
		return
	}

	switch v1.Kind() {
	case reflect.Array:
		for i := 0; i < v1.Len(); i++ {
			if !reflect.DeepEqual(v1.Index(i), v2.Index(i)) {
				t.Errorf("arrays differ at index [%d]: [%v] <> [%v]", i, v1.Index(i), v2.Index(i))
				return
			}
		}
		return
	case reflect.Slice:
		if v1.IsNil() != v2.IsNil() {
			t.Errorf("slices differ: actual [%v] <> expected [%v]", v1, v2)
			return
		}
		if v1.Len() != v2.Len() {
			t.Errorf("slice length differs: actual len [%d] <> expected len [%v]", v1.Len(), v2.Len())
			return
		}
		if v1.Pointer() == v2.Pointer() {
			return
		}
		for i := 0; i < v1.Len(); i++ {
			if !reflect.DeepEqual(v1.Index(i).Interface(), v2.Index(i).Interface()) {
				t.Errorf("slices differ at index [%d]: actual [%v] <> expected [%v]", i, v1.Index(i).Interface(), v2.Index(i).Interface())
				return
			}
		}
		return
	default:
		t.Errorf("not arrays: actual type [%T], expected type [%T]", v1, v2)
		return
	}
}

func TestEscapeSeparators(t *testing.T) {
	tests := []struct {
		path string
		sep  []string
		want string
	}{
		{"abcd", nil, "abcd"},
		{"/a/b", nil, "~1a~1b"},
		{"/a/b", []string{"/"}, "~1a~1b"},
		{"/a/b", []string{"."}, "/a/b"},
		{".a.b", []string{"."}, "~1a~1b"},
	}
	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			if test.sep == nil {
				require.Equal(t, test.want, EscapeSeparators(test.path))
			} else {
				require.Equal(t, test.want, EscapeSeparators(test.path, test.sep...))
			}
		})
	}
}

func TestPathParsePaths(t *testing.T) {
	tests := []struct {
		paths []string
		want  []Path
	}{
		{
			paths: nil,
			want:  nil,
		},
		{
			paths: []string{},
			want:  []Path{},
		},
		{
			paths: []string{"/", "/a", "/b/c"},
			want:  []Path{{}, {"a"}, {"b", "c"}},
		},
	}
	for _, tt := range tests {
		t.Run(jsonutil.MarshalCompactString(tt.paths), func(t *testing.T) {
			res := ParsePaths(tt.paths)
			require.Equal(t, tt.want, res)
		})
	}
}
