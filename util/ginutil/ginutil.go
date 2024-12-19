package ginutil

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/gin-gonic/gin/binding"

	"github.com/eluv-io/common-go/util/httputil"
	"github.com/eluv-io/common-go/util/stackutil"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
)

// CustomAbortHandler is an optional handler that can be registered on the gin.Context with SetCustomAbortHandler() that
// allows intercepting calls to Abort() or AbortWithStatus(). The custom handler must return true if it has handled the
// abort completely. Otherwise, the default abort logic is executed.
type CustomAbortHandler func(c *gin.Context, code int, err error) (handled bool)

const customAbortHandlerKey = "custom-abort-handler"
const loggerKey = "ginutil.LOGGER"

// Abort aborts the current HTTP request with the given error. The HTTP status code is set according to the error type.
// If the error is an eluv-io/errors-go with kind "Other" or a non-eluv-io/errors-go, the call also logs the stacktrace
// of all goroutines. The logger (an instance of eluv-io/log-go) can be set in the gin context under the "LOGGER" key.
// If not set, the root logger will be used.
func Abort(c *gin.Context, err error) {
	code := HttpStatus(err)
	if code == http.StatusInternalServerError {
		dumpGoRoutines(c)
	}
	AbortWithStatus(c, code, err)
}

// AbortHead aborts the current HTTP HEAD request with the HTTP status code set according to the
// given error type.
func AbortHead(c *gin.Context, err error) {
	AbortHeadWithStatus(c, HttpStatus(err))
}

// HttpStatus returns the HTTP status code for the given error. The status code is determined based on the error kind.
func HttpStatus(err error) int {
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
		case errors.K.NotImplemented:
			code = http.StatusNotImplemented
		case httputil.KindRangeNotSatisfiable:
			code = http.StatusRequestedRangeNotSatisfiable
		}
	}
	return code
}

// AbortWithStatus aborts the current HTTP request with the given status code and error.
func AbortWithStatus(c *gin.Context, code int, err error) {
	if handlerFunc, ok := GetCustomAbortHandler(c); ok {
		log := getLog(c)
		log.Trace("calling custom abort handler", "code", code, "err", err)
		if handlerFunc(c, code, err) {
			log.Debug("abort handled by custom handler")
			return
		}
		log.Trace("abort not handled by custom handler")
	}
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
	case binding.MIMEJSON:
		c.JSON(code, gin.H{"errors": []interface{}{err}})
	case binding.MIMEXML:
		c.Render(code, httputil.NewCustomXMLRenderer(gin.H{"errors": []interface{}{err}}))
	default:
		c.JSON(code, gin.H{"errors": []interface{}{err}})
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

// SetCustomAbortHandler sets a custom handler that is called if an API call is aborted.
func SetCustomAbortHandler(c *gin.Context, handler CustomAbortHandler) {
	c.Set(customAbortHandlerKey, handler)
}

// GetCustomAbortHandler gets the custom abort handler if previously set. The bool return indicates whether a handler
// was set or not.
func GetCustomAbortHandler(c *gin.Context) (CustomAbortHandler, bool) {
	if handler, ok := c.Get(customAbortHandlerKey); ok {
		if handlerFunc, ok := handler.(CustomAbortHandler); ok {
			return handlerFunc, true
		}
	}
	return nil, false
}
