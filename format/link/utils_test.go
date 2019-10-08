package link_test

import (
	"fmt"
	"testing"

	"eluvio/format/link"
	"eluvio/format/structured"
	"eluvio/util/jsonutil"
	"eluvio/util/maputil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type jm = map[string]interface{}
type ja = []interface{}

func TestConvertLinks(t *testing.T) {

	testLink, err := link.NewLink(qHash(), link.S.File, structured.ParsePath("/some/path"))
	require.NoError(t, err)
	testLinkWithProps, err := link.NewLink(qHash(), link.S.File, structured.ParsePath("/some/path"))
	require.NoError(t, err)
	testLinkWithProps.Props = maputil.From("prop1", "value1")

	tests := []struct {
		name        string
		data        interface{}
		expectError bool
	}{
		{
			name: "no links",
			data: jm{"a": "one"},
		},
		{
			name: "single link",
			data: testLink,
		},
		{
			name: "link in map",
			data: jm{
				"a link":          testLink,
				"no link 1":       jm{"/": jm{}},
				"no link 2":       jm{"/": ja{}},
				"link with props": testLinkWithProps,
				"no link 4":       jm{".": "something"},
			},
		},
		{
			name: "link in array",
			data: ja{"no link", testLink, "no link"},
		},
		{
			name: "multiple links in arrays and maps",
			data: jm{
				"this is a link": testLink,
				"an array":       ja{"no link", testLink, "no link"},
			},
		},
		{
			name: "blob link",
			data: link.NewBlobBuilder().Data([]byte("clear data")).MustBuild(),
		},
		{
			name: "invalid link generates error",
			data: jm{
				"invalid link": jm{"/": "/qfab/invalid-hash"},
			},
			expectError: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ser := jsonutil.Marshal(&test.data)
			fmt.Println(string(ser))
			var des interface{}
			jsonutil.Unmarshal(ser, &des)
			data, err := link.ConvertLinks(des)
			if test.expectError {
				assert.Error(t, err)
				assert.Nil(t, data)
			} else {
				assert.NoError(t, err)
				assert.EqualValues(t, test.data, data)
			}
		})
	}
}
