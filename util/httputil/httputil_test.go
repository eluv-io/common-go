package httputil_test

import (
	"fmt"
	"github.com/qluvio/content-fabric/constants"
	"net/http"
	"net/url"
	"testing"

	"github.com/qluvio/content-fabric/util/httputil"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseBytesRange(t *testing.T) {
	tests := []struct {
		bytes      string
		wantOffset int64
		wantSize   int64
		wantErr    bool
	}{
		{bytes: "0-99", wantOffset: 0, wantSize: 100, wantErr: false},
		{bytes: "100-199", wantOffset: 100, wantSize: 100, wantErr: false},
		{bytes: "100-", wantOffset: 100, wantSize: -1, wantErr: false},
		{bytes: "-100", wantOffset: -1, wantSize: 100, wantErr: false},
		{bytes: "-", wantOffset: 0, wantSize: -1, wantErr: false},
		{bytes: "", wantOffset: 0, wantSize: -1, wantErr: false},
		{bytes: "0", wantOffset: 0, wantSize: 0, wantErr: true},
		{bytes: "+400", wantOffset: 0, wantSize: 0, wantErr: true},
		{bytes: "abc-def", wantOffset: 0, wantSize: 0, wantErr: true},
		{bytes: "-0", wantOffset: -1, wantSize: 0, wantErr: false},
	}
	for _, tt := range tests {
		t.Run(tt.bytes, func(t *testing.T) {
			gotOffset, gotSize, err := httputil.ParseByteRange(tt.bytes)
			assert.Equal(t, tt.wantOffset, gotOffset)
			assert.Equal(t, tt.wantSize, gotSize)
			if tt.wantErr {
				assert.Error(t, err)
				fmt.Println(err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCustomHeaders(t *testing.T) {
	gin.SetMode(gin.ReleaseMode)
	var err error

	{ // no headers
		h := http.Header{}
		m, err := httputil.ExtractCustomHeaders(h)
		require.NoError(t, err)
		require.Equal(t, 0, len(m))
	}

	{ // string headers
		h := http.Header{}
		httputil.SetCustomHeader(h, "ID", "123456")
		httputil.SetCustomHeader(h, "Name", "Joe")

		m, err := httputil.ExtractCustomHeaders(h)
		require.NoError(t, err)
		require.Equal(t, 2, len(m))
		require.EqualValues(t, "123456", m["id"].(string))
		require.EqualValues(t, "Joe", m["name"].(string))
	}

	{ // CBOR headers, individual values
		h := http.Header{}
		err = httputil.SetCustomCBORHeader(h, "ID", 123456)
		require.NoError(t, err)
		err = httputil.SetCustomCBORHeader(h, "Name", "Joe")
		require.NoError(t, err)

		m, err := httputil.ExtractCustomHeaders(h)
		require.NoError(t, err)
		require.Equal(t, 2, len(m))
		require.EqualValues(t, 123456, m["id"])
		require.EqualValues(t, "Joe", m["name"])
	}

	{ // CBOR struct
		type testStruct struct {
			ID   int
			Name string
		}

		ts := &testStruct{
			ID:   998877,
			Name: "joe doe",
		}

		h := http.Header{}
		err = httputil.SetCustomCBORHeader(h, "struct", ts)
		require.NoError(t, err)

		m, err := httputil.ExtractCustomHeaders(h)
		require.NoError(t, err)
		require.Equal(t, 1, len(m))
		require.EqualValues(t, ts.ID, m["struct"].(map[interface{}]interface{})["ID"])
		require.EqualValues(t, ts.Name, m["struct"].(map[interface{}]interface{})["Name"])
	}

	{ // custom headers from a map
		type testStruct struct {
			ID   int
			Name string
		}

		ts := &testStruct{
			ID:   998877,
			Name: "joe doe",
		}

		hm := make(map[string]interface{})
		hm["struct"] = ts
		hm["string"] = "a string!"
		hm["int"] = 5566
		hm["float"] = 1.23

		h := http.Header{}
		httputil.SetCustomHeaders(h, hm)

		require.NotNil(t, h.Get("X-Content-Fabric-Mc-Struct"))
		require.NotNil(t, h.Get("X-Content-Fabric-String"))
		require.NotNil(t, h.Get("X-Content-Fabric-Mc-Int"))
		require.NotNil(t, h.Get("X-Content-Fabric-Mc-Float"))

		m, err := httputil.ExtractCustomHeaders(h)
		require.NoError(t, err)
		require.Equal(t, 4, len(m))
		require.EqualValues(t, ts.ID, m["struct"].(map[interface{}]interface{})["ID"])
		require.EqualValues(t, ts.Name, m["struct"].(map[interface{}]interface{})["Name"])
		require.EqualValues(t, "a string!", m["string"])
		require.EqualValues(t, 5566, m["int"])
		require.EqualValues(t, 1.23, m["float"])
	}

}

func TestNormalizeURL(t *testing.T) {
	defaultScheme := "http"
	defaultPort := "8008"

	u, err := httputil.NormalizeURL("hello", defaultScheme, defaultPort)
	require.NoError(t, err)
	require.EqualValues(t, "http://hello:8008", u)

	u, err = httputil.NormalizeURL("hello:1234", defaultScheme, defaultPort)
	require.NoError(t, err)
	require.EqualValues(t, "http://hello:1234", u)

	u, err = httputil.NormalizeURL("test://hello", defaultScheme, defaultPort)
	require.NoError(t, err)
	require.EqualValues(t, "test://hello:8008", u)

	u, err = httputil.NormalizeURL("hello/path/to/pizza", defaultScheme, defaultPort)
	require.NoError(t, err)
	require.EqualValues(t, "http://hello:8008/path/to/pizza", u)
}

func TestGetSetContentDisposition(t *testing.T) {
	type testCase struct {
		hdr    http.Header
		query  url.Values
		expect string
	}
	for _, tcase := range []*testCase{
		{
			hdr: http.Header{
				constants.SetContentDispositionHeader: {"attachment; filename=genome.jpeg;"},
			},
			query: url.Values{
				"header-x_set_content_disposition": []string{"attachment; filename=genome.jpeg;"},
			},
			expect: "attachment; filename=genome.jpeg;",
		},
		{
			hdr: http.Header{
				constants.SetContentDispositionHeader: {"attachment; filename=genome.jpeg;"},
			},
			query:  url.Values{},
			expect: "attachment; filename=genome.jpeg;",
		},
		{
			hdr: http.Header{},
			query: url.Values{
				"header-x_set_content_disposition": []string{"attachment; filename=genome.jpeg;"},
			},
			expect: "attachment; filename=genome.jpeg;",
		},
	} {
		ct, err := httputil.GetSetContentDisposition(tcase.hdr, tcase.query, "")
		require.NoError(t, err)
		require.Equal(t, tcase.expect, ct)
	}
}
