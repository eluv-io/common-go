package traceutil

import (
	"github.com/eluv-io/common-go/util/jsonutil"
	elog "github.com/eluv-io/log-go"
)

// InitLogTracing enables tracing for the current goroutine if and only if the given log instance is at the TRACE
// logging level. Otherwise, does nothing. If enabled, it initializes a root span with the provided name and sets it on
// the goroutine's context stack using traceutil.InitTracing.
//
// The returned function should be called in a defer statement: it will end the root span and log it at TRACE level in
// the provided log. If tracing is disabled, the returned function is a no-op.
func InitLogTracing(log *elog.Log, rootSpan string) func() {
	if !log.IsTrace() {
		return func() {}
	}
	span := InitTracing(rootSpan, false)
	span.SetMarshalExtended() // enable start/end timestamps in addition to duration
	return func() {
		span.End()
		log.Trace("trace", "span", jsonutil.Stringer(span))
	}
}
