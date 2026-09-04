package netutil

import (
	"encoding/json"
	"fmt"
	"net/url"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestJoin(t *testing.T) {
	tests := []struct {
		name     string
		elems    []string
		expected string
	}{
		// empty / degenerate cases
		{name: "no args", elems: []string{}, expected: ""},
		{name: "single empty", elems: []string{""}, expected: ""},
		{name: "all empty", elems: []string{"", "", ""}, expected: ""},

		// single element
		{name: "single element", elems: []string{"a"}, expected: "a"},
		{name: "single slash", elems: []string{"/"}, expected: "/"},
		{name: "single with leading slash", elems: []string{"/a"}, expected: "/a"},
		{name: "single with trailing slash", elems: []string{"a/"}, expected: "a/"},

		// two plain segments - slash is inserted between them
		{name: "two plain", elems: []string{"a", "b"}, expected: "a/b"},
		{name: "three plain", elems: []string{"a", "b", "c"}, expected: "a/b/c"},

		// second element already has leading slash - no double slash
		{name: "second has leading slash", elems: []string{"a", "/b"}, expected: "a/b"},
		{name: "first has trailing slash", elems: []string{"a/", "b"}, expected: "a/b"},

		// both overlap: first ends '/' AND second starts '/' - slashes are collapsed
		{name: "both slashes overlap", elems: []string{"a/", "/b"}, expected: "a/b"},

		// empty elements are ignored
		{name: "empty between segments", elems: []string{"a", "", "b"}, expected: "a/b"},
		{name: "leading empty", elems: []string{"", "a", "b"}, expected: "a/b"},
		{name: "trailing empty", elems: []string{"a", "b", ""}, expected: "a/b"},

		// URL-like usage - preserves double slashes inside path elements in contrast to path.Join()
		{name: "base url plain path", elems: []string{"http://host", "path"}, expected: "http://host/path"},
		{name: "base url trailing slash", elems: []string{"http://host/", "path"}, expected: "http://host/path"},
		{name: "base url path with leading slash", elems: []string{"http://host", "/path"}, expected: "http://host/path"},
		{name: "base url with subpath", elems: []string{"http://host/base", "sub", "resource"}, expected: "http://host/base/sub/resource"},

		// path segments with trailing slash preserved
		{name: "trailing slash preserved", elems: []string{"a", "b/"}, expected: "a/b/"},
		{name: "middle segment trailing slash", elems: []string{"a", "b/", "c"}, expected: "a/b/c"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.expected, Join(tc.elems...))
		})
	}
}

func TestUrlJSON(t *testing.T) {
	type A struct {
		U *URL `json:"url"`
	}

	mustParse := func(s string) *url.URL {
		ret, err := url.Parse(s)
		if err != nil {
			panic(err)
		}
		return ret
	}

	type testCase struct {
		name string
		a    *A
	}
	for _, tc := range []*testCase{
		{name: "no url", a: &A{U: &URL{}}},
		{name: "wrong url", a: &A{U: &URL{URL: mustParse("http:127.0.0.1:8008")}}},
		{name: "with url", a: &A{U: &URL{URL: mustParse("http://127.0.0.1:8008")}}},
		{name: "with query", a: &A{U: &URL{URL: mustParse("http://127.0.0.1:8008?authorization=eyjbAA")}}},
	} {
		bb, err := json.Marshal(tc.a)
		require.NoError(t, err, tc.name)
		fmt.Println(tc.name, string(bb))
		a := &A{}
		err = json.Unmarshal(bb, a)
		require.NoError(t, err, tc.name)
		require.Equal(t, tc.a.U, a.U, tc.name)
	}

}
