package ginutil

import (
	"encoding"
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/eluv-io/common-go/util/httputil"
	"github.com/eluv-io/common-go/util/stackutil"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
)

const loggerKey = "ginutil.LOGGER"

// Handle "extends" a regular gin.HandlerFunc with an error return value. If fn() returns an error, it calls Abort.
// Otherwise, it does nothing and expects fn() to have sent an HTTP response itself.
//
// Handle allows handler functions to be coded with idiomatic error handling, instead of invoking ginutil.Abort(c, err)
// in each error handling block.
func Handle(fn func(c *gin.Context) error) gin.HandlerFunc {
	return func(c *gin.Context) {
		err := fn(c)
		if err != nil {
			Abort(c, err)
		}
	}
}

// Abort aborts the current HTTP request with the given error. The HTTP status code is set according to the error type.
// If the error is an eluv-io/errors-go with kind "Other" or a non-eluv-io/errors-go, the call also logs the stacktrace
// of all goroutines. The logger (an instance of eluv-io/log-go) can be set in the gin context under the "LOGGER" key.
// If not set, the root logger will be used.
func Abort(c *gin.Context, err error) {
	AbortWithStatus(c, abortCode(c, err), err)
}

// AbortHead aborts the current HTTP HEAD request with the HTTP status code set according to the
// given error type.
func AbortHead(c *gin.Context, err error) {
	AbortHeadWithStatus(c, abortCode(c, err))
}

func abortCode(c *gin.Context, err error) int {
	code := http.StatusInternalServerError
	if e, ok := err.(*errors.Error); ok {
		switch e.Kind() {
		case errors.K.Invalid, errors.K.Timeout, errors.K.Cancelled:
			code = http.StatusBadRequest
		case errors.K.Exist:
			code = http.StatusConflict
		case errors.K.NotExist:
			code = http.StatusNotFound
		case errors.K.Finalized:
			code = http.StatusConflict
		case errors.K.NotFinalized:
			code = http.StatusConflict
		case errors.K.Permission:
			code = http.StatusForbidden
		case errors.K.NoMediaMatch:
			code = http.StatusNotAcceptable
		case errors.K.Unavailable:
			code = http.StatusServiceUnavailable
		case errors.K.Other:
			dumpGoRoutines(c)
		}
	} else {
		dumpGoRoutines(c)
	}
	return code
}

// AbortWithStatus aborts the current HTTP request with the given status code and error.
func AbortWithStatus(c *gin.Context, code int, err error) {
	c.Abort()
	SendError(c, code, err)
}

// AbortHeadWithStatus aborts the current HTTP HEAD request with the given status code.
func AbortHeadWithStatus(c *gin.Context, code int) {
	c.Abort()
	c.Writer.Header().Del("Content-Type")
	c.Writer.Header().Del("Cache-Control")
	c.Writer.WriteHeader(code)
}

// SendError sends back an HTTP response with the given HTTP status code and the data marshaled to JSON or XML depending
// on the "accept" headers of the request. The data is marshaled to JSON if no accept headers are specified. No data is
// marshaled if an accept headers other than 'application/json' or 'application/xml' is specified.
func SendError(c *gin.Context, code int, err error) {
	if err != nil {
		getLog(c).Debug("api error", "code", code, "error", err)
	}

	c.Writer.Header().Del("Content-Type")
	c.Writer.Header().Del("Cache-Control")

	switch c.NegotiateFormat(gin.MIMEJSON, gin.MIMEXML) {
	case binding.MIMEXML:
		c.Render(code, httputil.NewCustomXMLRenderer(gin.H{"errors": []interface{}{err}}))
	default:
		switch t := err.(type) {
		case *errors.ErrorList:
			// error list marshals exactly as we want it: {"errors": [ e1, e2, ... ]}
			c.JSON(code, t)
		case json.Marshaler,
			encoding.TextMarshaler:
			// this includes *errors.Error: the error marshals correctly
			c.JSON(code, gin.H{"errors": []interface{}{t}})
		default:
			if err != nil {
				// convert error to string before marshalling
				c.JSON(code, gin.H{"errors": []interface{}{err.Error()}})
			} else {
				// we could not send a body at all, but for backwards compatibility let's keep this...
				c.JSON(code, gin.H{"errors": []interface{}{err}})
			}
		}
	}
}

// Send send back an HTTP response with the given HTTP status code and the data marshaled to JSON or XML depending on
// the "accept" headers of the request. The data is marshaled to JSON if no accept headers are specified. If an
// incompatible accept header is specified an error is returned with code '406 - Not Acceptable'
func Send(c *gin.Context, code int, data interface{}) {
	c.Writer.Header().Del("Content-Type")
	switch c.NegotiateFormat(gin.MIMEJSON, gin.MIMEXML) {
	case binding.MIMEJSON:
		c.JSON(code, data)
	case binding.MIMEXML:
		c.Render(code, httputil.NewCustomXMLRenderer(data))
	default:
		if code <= 0 {
			// this is also called from middleware.ErrorHandler with an explicit
			// code -1 to not modify the code in the context (which is _logged_
			// afterward)
			return
		}
		_ = c.AbortWithError(http.StatusNotAcceptable, errors.Str("the accepted formats are not offered by the server"))
	}
}

// SetLogger sets the logger for all logging performed in this package on the given gin context.
func SetLogger(c *gin.Context, logger *elog.Log) {
	c.Set(loggerKey, logger)
}

// dumpGoRoutines prints the stack of all goroutines to the log.
func dumpGoRoutines(c *gin.Context) {
	log := getLog(c)
	if !log.IsDebug() {
		return
	}
	log.Error("dumping go-routines", "dump", stackutil.FullStack())
}

// getLog returns the logger from the gin context or the root logger.
func getLog(c *gin.Context) (log *elog.Log) {
	if clg, ok := c.Get(loggerKey); ok {
		log, _ = clg.(*elog.Log)
	}
	if log == nil {
		log = elog.Get("/")
	}
	return log
}
