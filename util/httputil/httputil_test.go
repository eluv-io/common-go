package httputil_test

import (
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/errors-go"

	"github.com/eluv-io/common-go/format/hash"
	"github.com/eluv-io/common-go/format/id"
	"github.com/eluv-io/common-go/util/httputil"
	"github.com/eluv-io/common-go/util/jsonutil"
)

func TestGetBytesRange(t *testing.T) {
	tests := []struct {
		query     string
		header    string
		want      string
		wantError bool
	}{
		{"", "", "", false},
		{"bytes=3-8", "", "3-8", false},
		{"", "bytes=3-8", "3-8", false},
		{"bytes=-4", "bytes=3-8", "-4", false},
		{"", "not a byte range", "", true},
	}

	for _, test := range tests {
		url := "http://localhost/path"
		if test.query != "" {
			url = url + "?" + test.query
		}
		req, err := http.NewRequest("GET", url, nil)
		require.NoError(t, err)

		if test.header != "" {
			req.Header.Set("Range", test.header)
		}

		got, err := httputil.GetBytesRange(req)

		if test.wantError {
			require.Error(t, err)
		} else {
			require.NoError(t, err)
			require.Equal(t, test.want, got)
		}
	}

}

func TestCustomHeaders(t *testing.T) {
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
		require.EqualValues(t, ts.ID, m["struct"].(map[string]interface{})["ID"])
		require.EqualValues(t, ts.Name, m["struct"].(map[string]interface{})["Name"])
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
		require.EqualValues(t, ts.ID, m["struct"].(map[string]interface{})["ID"])
		require.EqualValues(t, ts.Name, m["struct"].(map[string]interface{})["Name"])
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
				httputil.SetContentDispositionHeader: {"attachment; filename=genomex.jpeg;"},
			},
			query: url.Values{
				"header-x_set_content_disposition": []string{"attachment; filename=genomey.jpeg;"},
			},
			expectFail: true,
		},
		{
			hdr: http.Header{
				httputil.SetContentDispositionHeader: {"attachment; filename=genome.jpeg;"},
			},
			query: url.Values{
				"header-x_set_content_disposition": []string{"attachment; filename=genome.jpeg;"},
			},
			expect: "attachment; filename=genome.jpeg;",
		},
		{
			hdr: http.Header{
				httputil.SetContentDispositionHeader: {"attachment; filename=genome.jpeg;"},
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

func TestGetSetReqNodes(t *testing.T) {
	inods := map[string]bool{
		"inod42f2YMiWdwmPB8Ts34vKm24Su9LJ": true,
		"inod4NCwnvo88W4KygHfBE652YfkjTeX": true,
	}
	headers := make(http.Header)
	httputil.SetReqNodes(headers, inods)
	require.Len(t, headers["X-Content-Fabric-Requested-Nodes"], 1)
	m, err := httputil.GetReqNodes(headers)
	require.NoError(t, err)
	for inod := range inods {
		require.True(t, m[inod])
	}
}

func TestGetSetPubNodes(t *testing.T) {
	inods := map[string]bool{
		"inod42f2YMiWdwmPB8Ts34vKm24Su9LJ": true,
		"inod4NCwnvo88W4KygHfBE652YfkjTeX": true,
	}
	headers := make(http.Header)
	httputil.SetPubNodes(headers, inods)
	require.Len(t, headers["X-Content-Fabric-Published-Nodes"], 1)
	m, err := httputil.GetPubNodes(headers)
	require.NoError(t, err)
	for inod := range inods {
		require.True(t, m[inod])
	}
}

func TestGetSetOriginNode(t *testing.T) {
	inod, err := id.QNode.FromString("inod42f2YMiWdwmPB8Ts34vKm24Su9LJ")
	require.NoError(t, err)
	headers := make(http.Header)
	httputil.SetOriginNode(headers, inod)
	require.Len(t, headers["X-Content-Fabric-Origin-Node"], 1)
	i, err := httputil.GetOriginNode(headers)
	require.NoError(t, err)
	require.Equal(t, inod, i)
}

func TestGetSetLiveHash(t *testing.T) {
	hql, err := hash.QPartLive.FromString("hql_8r4UFFpJSY3KJwnbLCN29Cuj2D9FqweWC1b388Hy")
	require.NoError(t, err)
	headers := make(http.Header)
	httputil.SetLiveHash(headers, hql)
	require.Len(t, headers["X-Content-Fabric-Live-Hash"], 1)
	h, err := httputil.GetLiveHash(headers)
	require.NoError(t, err)
	require.Equal(t, hql, h)
}

func TestGetSetMultipath(t *testing.T) {
	multipath := "inod42f2YMiWdwmPB8Ts34vKm24Su9LJ"
	headers := make(http.Header)
	httputil.SetMultipath(headers, multipath)
	require.Len(t, headers["X-Content-Fabric-Multipath"], 1)
	mp, err := httputil.GetMultipath(headers)
	require.NoError(t, err)
	require.Equal(t, multipath, mp)
}

func TestGetSetPubConfirms(t *testing.T) {
	confirms := []string{"hello", "world"}
	headers := make(http.Header)
	httputil.SetPubConfirms(headers, confirms)
	require.Len(t, headers["X-Content-Fabric-Publish-Confirmations"], 1)
	c, err := httputil.GetPubConfirms(headers)
	require.NoError(t, err)
	require.Equal(t, confirms, c)
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
	r := func(s string) io.ReadCloser {
		return ioutil.NopCloser(strings.NewReader(s))
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

func TestSetContentDispositionAttachment(t *testing.T) {
	tests := []struct {
		filename string
		headers  []string
	}{
		{"abc.txt", []string{"attachment; filename=\"abc.txt\""}},
		{"file with spaces.txt", []string{"attachment; filename=\"file with spaces.txt\""}},
		{"£ / € / 🚀", []string{"attachment; filename*=UTF-8''%C2%A3%20%2F%20%E2%82%AC%20%2F%20%F0%9F%9A%80"}},
	}

	for _, test := range tests {
		t.Run(test.filename, func(t *testing.T) {
			h := http.Header{}
			httputil.SetContentDisposition(h, test.filename)
			require.Equal(t, test.headers, h["Content-Disposition"])
		})
	}
}
