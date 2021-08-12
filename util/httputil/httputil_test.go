package httputil_test

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/qluvio/content-fabric/constants"
	"github.com/qluvio/content-fabric/errors"
	"github.com/qluvio/content-fabric/format/id"
	"github.com/qluvio/content-fabric/util/httputil"
	"github.com/qluvio/content-fabric/util/jsonutil"

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
		hdr        http.Header
		query      url.Values
		expect     string
		expectFail bool
	}
	for _, tcase := range []*testCase{
		{
			hdr: http.Header{
				constants.SetContentDispositionHeader: {"attachment; filename=genomex.jpeg;"},
			},
			query: url.Values{
				"header-x_set_content_disposition": []string{"attachment; filename=genomey.jpeg;"},
			},
			expectFail: true,
		},
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
		{
			hdr: http.Header{},
			query: url.Values{
				"header-x_set_content_disposition": []string{"attachment;filename=Wolf+of+Snow+Hollow,+The_MGM_Final_CCSL.pdf"},
			},
			expect: "attachment;filename=Wolf+of+Snow+Hollow,+The_MGM_Final_CCSL.pdf",
		},
	} {
		ct, err := httputil.GetSetContentDisposition(tcase.hdr, tcase.query, "")
		if tcase.expectFail {
			require.Error(t, err)
			continue
		}
		require.NoError(t, err)
		require.Equal(t, tcase.expect, ct)
	}
}

func TestSetReqNodes(t *testing.T) {
	inods := map[string]bool{
		"inod42f2YMiWdwmPB8Ts34vKm24Su9LJ": true,
	}
	h := make(http.Header)
	httputil.SetReqNodes(h, inods)
	m, err := httputil.GetReqNodes(h)
	require.NoError(t, err)
	require.True(t, m["inod42f2YMiWdwmPB8Ts34vKm24Su9LJ"])
}

func TestClientIP(t *testing.T) {
	req := func(remoteAddr string, headers ...string) *http.Request {
		header := http.Header{}
		for i := 0; i < len(headers); i += 2 {
			header.Add(headers[i], headers[i+1])
		}
		return &http.Request{
			RemoteAddr: remoteAddr,
			Header:     header,
		}
	}
	var tests = []struct {
		r      *http.Request
		accept string
		want   string
	}{
		// RemoteAddr is "" for reverse proxy using unix domain socket file
		{r: req("", "X-Real-IP", "2.2.2.2"), accept: "y", want: "2.2.2.2"},
		{r: req(""), want: ""},
		{r: req("", "X-Forwarded-For", "2.2.2.2"), accept: "", want: "2.2.2.2"},
		{r: req("", "X-Forwarded-For", "2.2.2.2"), accept: "nil", want: "2.2.2.2"},
		{r: req("", "X-Forwarded-For", "2.2.2.2"), accept: "n", want: ""},
		{r: req("", "X-Forwarded-For", "2.2.2.2"), accept: "y", want: "2.2.2.2"},
		{r: req("", "X-Real-IP", "2.2.2.2"), accept: "y", want: "2.2.2.2"},
		{r: req("", "X-Forwarded-For", "2.2.2.2", "X-Real-IP", "3.3.3.3"), accept: "y", want: "2.2.2.2"},
		{r: req("1.1.1.1:80"), want: "1.1.1.1"},
		{r: req("1.1.1.1:80", "X-Forwarded-For", "2.2.2.2"), accept: "n", want: "1.1.1.1"},
		{r: req("1.1.1.1:80", "X-Forwarded-For", "2.2.2.2"), accept: "y", want: "2.2.2.2"},
		{r: req("1.1.1.1:80", "X-Forwarded-For", "  3.3.3.3, 2.2.2.2"), accept: "y", want: "3.3.3.3"},
	}
	for _, test := range tests {
		t.Run(fmt.Sprintf("[%v] %v %v", test.r.RemoteAddr, test.accept, test.r.Header), func(t *testing.T) {
			switch test.accept {
			case "":
				require.Equal(t, test.want, httputil.ClientIP(test.r))
			case "nil":
				require.Equal(t, test.want, httputil.ClientIP(test.r, nil))
			default:
				require.Equal(t, test.want, httputil.ClientIP(test.r, func(string) bool { return test.accept == "y" }))
			}
		})
	}
}

func TestParseServerError(t *testing.T) {
	r := func(s string) io.Reader {
		return strings.NewReader(s)
	}

	match := func(e1, e2 error) {
		require.True(t, errors.Match(e1, e2), "e1=%s\ne2=%s", e1, e2)
	}

	assertKind := func(status int, kind interface{}) {
		err := httputil.ParseServerError(r("no json!"), status)
		match(errors.E(kind, "status", status, "body", "no json!"), err)
	}

	assertKind(http.StatusNotFound, errors.K.NotFound)
	assertKind(http.StatusUnauthorized, errors.K.Permission)
	assertKind(http.StatusForbidden, errors.K.Permission)
	assertKind(http.StatusInternalServerError, errors.K.Internal)
	assertKind(http.StatusConflict, errors.K.Internal)
	assertKind(http.StatusGone, errors.K.Internal)

	match(errors.E(errors.K.NotFound, "body", ""), httputil.ParseServerError(r(""), http.StatusNotFound))
	match(errors.E(errors.K.NotFound, "body", "abc"), httputil.ParseServerError(r("abc"), http.StatusNotFound))

	err := errors.NoTrace("get meta", errors.K.NotExist, "id", id.Generate(id.Q).String())
	body := jsonutil.MarshalString(map[string]interface{}{
		"errors": []interface{}{
			err,
		},
	})
	match(errors.NoTrace(errors.K.NotFound, err), httputil.ParseServerError(r(body), http.StatusNotFound))
}
