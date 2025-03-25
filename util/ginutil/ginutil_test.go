package ginutil

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/apexlog-go/handlers/memory"
	"github.com/eluv-io/common-go/util/httputil"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/log-go"
)

func init() {
	gin.SetMode(gin.ReleaseMode)
}

func TestAbort(t *testing.T) {
	tests := []struct {
		err      error
		wantCode int
	}{
		{nil, 500},
		{errors.E("op"), 500},
		{errors.E("op", errors.K.Invalid), 400},
		{errors.E("op", errors.K.Cancelled), 400},
		{errors.E("op", errors.K.Permission), 403},
		{errors.E("op", errors.K.NotExist), 404},
		{errors.E("op", errors.K.NoMediaMatch), 406},
		{errors.E("op", errors.K.Exist), 409},
		{errors.E("op", errors.K.Finalized), 409},
		{errors.E("op", errors.K.NotFinalized), 409},
		{errors.E("op", errors.K.NotFound), 500},
		{errors.E("op", errors.K.IO), 500},
		{errors.E("op", errors.K.AVInput), 500},
		{errors.E("op", errors.K.AVProcessing), 500},
		{errors.E("op", errors.K.NotImplemented), 501},
		{errors.E("op", errors.K.Unavailable), 503},
		{errors.E("op", httputil.KindRangeNotSatisfiable), 416},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprint(errors.Field(tt.err, "kind")), func(t *testing.T) {
			w, c := testCtx(t)

			Abort(c, tt.err)
			require.Equal(t, tt.wantCode, w.Code)
			require.Equal(
				t,
				jsonutil.MarshalCompactString(map[string]interface{}{"errors": []interface{}{tt.err}}),
				w.Body.String(),
			)
		})
	}
}

func TestAbortWithStatus(t *testing.T) {
	fnJson := func(err error) string {
		return jsonutil.MarshalCompactString(map[string]interface{}{"errors": []interface{}{err}})
	}

	tests := []struct {
		err      error
		code     int
		accept   string
		wantBody func(err error) string
	}{
		{nil, 500, "", fnJson},
		{nil, 500, "application/json", fnJson},
		{nil, 500, "application/custom", fnJson},
		{nil, 500, "application/xml", func(err error) string {
			return `<?xml version="1.0" encoding="UTF-8"?>
<root>
  <errors>
    <error/>
  </errors>
</root>
`
		}},
		{nil, 404, "application/json", fnJson},
		{errors.E("op"), 409, "application/json", fnJson},
		{errors.E("op", errors.K.Unavailable), 404, "application/json", fnJson},
		{errors.NoTrace("op"), 404, "application/json", fnJson},
		{errors.NoTrace("op"), 404, "application/xml", func(err error) string {
			return `<?xml version="1.0" encoding="UTF-8"?>
<root>
  <errors>
    <error>
      <kind>unclassified error</kind>
      <op>op</op>
    </error>
  </errors>
</root>
`
		}},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprint(tt.err), func(t *testing.T) {
			w, c := testCtx(t)
			if tt.accept != "" {
				c.Request.Header.Set("Accept", tt.accept)
			}

			AbortWithStatus(c, tt.code, tt.err)
			require.Equal(t, tt.code, w.Code)
			require.Equal(
				t,
				tt.wantBody(tt.err),
				w.Body.String(),
			)
		})
	}
}

func TestAbort_WithLog(t *testing.T) {
	lg := log.New(&log.Config{
		Level:   "debug",
		Handler: "memory",
	})
	require.Len(t, lg.Handler().(*memory.Handler).Entries, 0)

	_, c := testCtx(t)
	SetLogger(c, lg)
	Abort(c, io.EOF)

	require.Len(t, lg.Handler().(*memory.Handler).Entries, 2)
}

func TestSendError_Xml(t *testing.T) {
	w, c := testCtx(t)
	c.Request.Header.Set("Accept", "application/xml")

	err := errors.E("op", errors.K.NotExist)
	SendError(c, 404, err)

	require.Equal(t, 404, w.Code)
	require.Contains(t, w.Body.String(), "<kind>item does not exist</kind>")
	require.Contains(t, w.Body.String(), "<op>op</op>")
}

func TestSendError_JSON(t *testing.T) {
	tests := []struct {
		err  error
		want string
	}{
		{
			err:  nil,
			want: `{"errors":[null]}`,
		},
		{
			err:  io.EOF,
			want: `{"errors":["EOF"]}`,
		},
		{
			err:  fmt.Errorf("std error"),
			want: `{"errors":["std error"]}`,
		},
		{
			err:  errors.NoTrace("test", errors.K.Invalid),
			want: `{"errors":[{"op":"test","kind":"invalid"}]}`,
		},
		{ // errors.Append creates an *errors.ErrorList...
			err:  errors.Append(io.EOF, fmt.Errorf("std error"), errors.NoTrace("test", errors.K.Invalid)),
			want: `{"errors":["EOF","std error",{"op":"test","kind":"invalid"}]}`,
		},
	}

	for _, test := range tests {
		t.Run(test.want, func(t *testing.T) {
			w, c := testCtx(t)
			c.Request.Header.Set("Accept", "application/json")
			SendError(c, 404, test.err)
			require.Equal(t, 404, w.Code)
			require.Equal(t, w.Body.String(), test.want)
		})
	}
}

func TestSend(t *testing.T) {
	tests := []struct {
		res      interface{}
		code     int
		accept   string
		wantBody string
		wantCode int
	}{
		{
			res:      "This is the result",
			code:     200,
			accept:   "application/json",
			wantBody: "\"This is the result\"",
		},
		{
			res:      "This is the result",
			code:     200,
			accept:   "application/xml",
			wantBody: "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<root>This is the result</root>\n",
		},
		{
			res:      "This is the result",
			code:     406,
			accept:   "application/custom",
			wantBody: "",
		},
		{
			res:      "This is the result",
			code:     -1,
			accept:   "application/custom",
			wantBody: "",
			wantCode: 200,
		},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprint("code", tt.code, "accept", tt.accept), func(t *testing.T) {

			w, c := testCtx(t)
			c.Request.Header.Set("Accept", tt.accept)

			Send(c, tt.code, tt.res)

			wantCode := tt.wantCode
			if wantCode == 0 {
				wantCode = tt.code
			}
			require.Equal(t, wantCode, w.Code)
			require.Equal(
				t,
				tt.wantBody,
				w.Body.String(),
			)
		})
	}
}

func testCtx(t *testing.T) (*httptest.ResponseRecorder, *gin.Context) {
	var err error
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request, err = http.NewRequest("GET", "http://127.0.0.1", nil)
	require.NoError(t, err)
	return w, c
}

func TestSendErrorAfterWrite(t *testing.T) {
	tests := []struct {
		description string
		err         error
	}{
		{description: `eof`, err: io.EOF},
		{description: `with-stack`, err: errors.E("omg", errors.K.Invalid)},
		{description: `no-error`, err: nil},
	}
	//log.Get("/").SetLevel("debug")

	for _, test := range tests {
		t.Run(test.description, func(t *testing.T) {

			var panicErr interface{}

			handler := func(c *gin.Context) {
				defer func() {
					panicErr = recover()
				}()
				c.Writer.WriteHeader(http.StatusOK)
				c.Writer.Header().Set("Content-Length", "10")
				c.Writer.Header().Add("a-key", "a-value")
				_, err := c.Writer.Write([]byte("0123456789"))
				require.NoError(t, err)
				var sendErr error
				if test.err != nil {
					sendErr = errors.E("xx", test.err)
				}
				SendError(c, http.StatusInternalServerError, sendErr)
			}
			srv, _ := testHttpCtx(handler)
			srv.Start()
			defer srv.Close()

			request, err := http.NewRequest("GET", srv.URL, nil)
			request.Header.Set("Accept", "application/json")
			require.NoError(t, err)

			resp, err := srv.Client().Do(request)
			require.Nil(t, panicErr)
			require.NoError(t, err)

			require.Equal(t, http.StatusOK, resp.StatusCode)
			rbytes, err := io.ReadAll(resp.Body)
			require.NoError(t, err)
			require.Equal(t, string(rbytes), "0123456789")
		})
	}
}

func testHttpCtx(hf gin.HandlerFunc) (*httptest.Server, *gin.Engine) {
	c := gin.New()
	c.Handle("GET", "", hf)
	srv := httptest.NewUnstartedServer(c)
	return srv, c
}
