package link_test

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/qluvio/content-fabric/format/hash"
	"github.com/qluvio/content-fabric/format/link"
	"github.com/qluvio/content-fabric/format/structured"
	"github.com/qluvio/content-fabric/util/maputil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testCase struct {
	name string
	str  string
	lnk  *link.Link
}

func qHash() *hash.Hash {
	h, err := hash.FromString("hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7")
	if err != nil {
		panic(err)
	}
	return h
}

func qpHash() *hash.Hash {
	h, err := hash.FromString("hqp_4YWKwzD4cymG9DodGRLphDg8fi2euXRgyYq9euQkjZx4a39")
	if err != nil {
		panic(err)
	}
	return h
}

var linkTests = []testCase{
	{
		name: "rel",
		str:  "./meta/some/path",
		lnk:  create(nil, link.S.Meta, structured.ParsePath("/some/path")),
	},
	{
		name: "rel with range",
		str:  "./files/some/path#40-49",
		lnk:  create(nil, link.S.File, structured.ParsePath("/some/path"), 40, 10),
	},
	{
		name: "abs no path",
		str:  "/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/rep",
		lnk:  create(qHash(), link.S.Rep, nil),
	},
	{
		name: "abs with path",
		str:  "/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/files/some/path",
		lnk:  create(qHash(), link.S.File, structured.ParsePath("/some/path")),
	},
	{
		name: "abs with path and range",
		str:  "/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/files/some/path#300-",
		lnk:  create(qHash(), link.S.File, structured.ParsePath("/some/path"), 300, -1),
	},
	{
		name: "qpart",
		str:  "/qfab/hqp_4YWKwzD4cymG9DodGRLphDg8fi2euXRgyYq9euQkjZx4a39",
		lnk:  create(qpHash(), link.S.None, nil),
	},
	{
		name: "qpart with range",
		str:  "/qfab/hqp_4YWKwzD4cymG9DodGRLphDg8fi2euXRgyYq9euQkjZx4a39#300-",
		lnk:  create(qpHash(), "", nil, 300, -1),
	},
	{
		name: "rel rep with byte range: range must not be parsed",
		str:  "./rep/bla#10-19",
		lnk:  create(nil, link.S.Rep, structured.ParsePath("bla#10-19")),
	},
	{
		name: "abs rep with byte range: range must not be parsed",
		str:  "/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/rep/bla#10-19",
		lnk:  create(qHash(), link.S.Rep, structured.ParsePath("bla#10-19")),
	},
}

func TestLinks(t *testing.T) {
	for _, test := range linkTests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("string-conversions", func(t *testing.T) {
				testStringConversion(t, test)
			})
			t.Run("json", func(t *testing.T) {
				testJSON(t, test.lnk, fmt.Sprintf(`{"/":"%s"}`, test.str))
			})
			t.Run("wrapped-json", func(t *testing.T) {
				testWrappedJSON(t, test)
			})
		})
	}
}

func TestLinksInvalid(t *testing.T) {
	tests := []struct {
		link string
	}{
		{link: "not-absolute/not-relative"},
		{link: "./invalid-selector"},
		{link: "./meta/with-byterange#45-"},
		{link: "/qfab/invalid-hash/"},
		{link: "./files/invalid-byterange#10-5"},
		{link: "/qfab/hqp_4YWKwzD4cymG9DodGRLphDg8fi2euXRgyYq9euQkjZx4a39/invalid/path"},
		{link: "/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7"},
		{link: "/qfab/hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/meta#45-"},
		{link: "hq__2w1SR2eY9LChsaY5f3EE2G4RhroKnmL7dsyB7Wm2qvbRG5UF9GoPVgFvD1nFqe9Pt4hF7/some/path"},
	}
	for _, test := range tests {
		t.Run(test.link, func(t *testing.T) {
			l, err := link.FromString(test.link)
			assert.Error(t, err)
			assert.Nil(t, l)
			fmt.Println(err)
		})
	}
}

func TestLinkProperties(t *testing.T) {
	for _, test := range linkTests {
		// create a copy of the link
		lnk := *(test.lnk)
		lnk.Props = maputil.From("k1", "v1", "k2", "v2")
		// create a copy of the test case
		tc := test
		tc.lnk = &lnk
		t.Run(test.name, func(t *testing.T) {
			// string conversion tests do not apply, because link properties are
			// not retained in string format!
			//
			// t.Run("string-conversions", func(t *testing.T) {
			// 	testStringConversion(t, test)
			// })

			t.Run("json", func(t *testing.T) {
				testJSON(t, tc.lnk, fmt.Sprintf(`{"/":"%s","k1":"v1","k2":"v2"}`, test.str))
			})
			t.Run("wrapped-json", func(t *testing.T) {
				testWrappedJSON(t, tc)
			})
		})
	}
}

func TestUnmarshalMap(t *testing.T) {
	for _, test := range linkTests {
		t.Run(test.name, func(t *testing.T) {
			t.Run("UnmarshalMap", func(t *testing.T) {
				m := map[string]interface{}{
					"/": test.str,
				}
				var lnk link.Link
				err := lnk.UnmarshalMap(m)
				require.NoError(t, err)
				require.Equal(t, test.lnk, &lnk)
			})
		})
	}
	t.Run("not a link", func(t *testing.T) {
		var lnk link.Link
		err := lnk.UnmarshalMap(map[string]interface{}{"key": "value"})
		require.Error(t, err)
	})
}

func TestIsLink(t *testing.T) {
	aLink := link.NewBuilder().P("other").Selector(link.S.Meta).MustBuild()
	tests := []struct {
		name     string
		val      interface{}
		wantLink *link.Link
	}{
		{
			name:     "*link",
			val:      aLink,
			wantLink: aLink,
		},
		{
			name:     "link",
			val:      *aLink,
			wantLink: aLink,
		},
		{
			name:     "no link",
			val:      "a string",
			wantLink: nil,
		},
		{
			name:     "nil",
			val:      nil,
			wantLink: nil,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.wantLink != nil, link.IsLink(test.val))
			require.Equal(t, test.wantLink, link.AsLink(test.val))
		})
	}

}

func testStringConversion(t *testing.T, tc testCase) {
	linkString := tc.lnk.String()
	assert.Equal(t, tc.str, linkString)

	linkFromString, err := link.FromString(linkString)
	require.NoError(t, err)

	assert.Equal(t, tc.lnk, linkFromString)
	assert.Equal(t, linkString, fmt.Sprint(tc.lnk))
	assert.Equal(t, linkString, fmt.Sprint(*tc.lnk))
	assert.Equal(t, linkString, fmt.Sprintf("%v", tc.lnk))
	assert.Equal(t, "blub"+linkString, fmt.Sprintf("blub%s", tc.lnk))
}

func testJSON(t *testing.T, lnk *link.Link, expJson string) {
	b, err := json.Marshal(lnk)
	assert.NoError(t, err)

	if expJson != "" {
		assert.Equal(t, expJson, string(b))
	}

	var unmarshalled link.Link
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, lnk, &unmarshalled)
	assert.Equal(t, *lnk, unmarshalled)
}

type Wrapper struct {
	Link link.Link
}

func testWrappedJSON(t *testing.T, tc testCase) {
	s := Wrapper{
		Link: *tc.lnk,
	}
	b, err := json.Marshal(s)
	assert.NoError(t, err)
	assert.Contains(t, string(b), tc.str)

	fmt.Println(string(b))

	var unmarshalled Wrapper
	err = json.Unmarshal(b, &unmarshalled)
	assert.NoError(t, err)
	assert.Equal(t, s, unmarshalled)
}

func create(target *hash.Hash, sel link.Selector, path structured.Path, offAndLen ...int64) *link.Link {
	l, err := link.NewLink(target, sel, path, offAndLen...)
	if err != nil {
		panic(err)
	}
	return l
}
