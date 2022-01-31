package traceutil

import (
	"context"

	elog "github.com/eluv-io/log-go"
	"go.opentelemetry.io/otel/api/trace"

	"github.com/eluv-io/common-go/util/ctxutil"
)

var log = elog.Get("/eluvio/util/traceutil")

// The currently used ContextStack
var current = ctxutil.Noop()

// EnableTracing enables/disables tracing globally with a ContextStack. Returns
// a reset function that can be used to reset the tracing state to what it was
// before the call (mainly useful for testing).
//
// Note: this function is not thread-safe and should only be called before any
// 	     of the tracing functions are used.
func EnableTracing(enable bool) func() {
	if current == ctxutil.Noop() {
		if enable {
			log.Info("enabling performance tracing support")
			current = ctxutil.NewStack()
			return func() {
				current = ctxutil.Noop()
			}
		}
	} else {
		if !enable {
			log.Info("disabling performance tracing support")
			current = ctxutil.Noop()
			return func() {
				current = ctxutil.NewStack()
			}
		}
	}
	return func() {}
}

// InitTracing initializes performance tracing for the current goroutine.
func InitTracing(t trace.Tracer, spanName string, opts ...trace.StartOption) trace.Span {
	return current.InitTracing(t, spanName, opts...)
}

// InitTestTracing initializes performance tracing for the purpose of testing.
// The returned tracer exposes the collected trace through its Trace member
// after the returned span's End() function has been called.
// In order to cleanup after the test, call tracer.Cleanup()
//
//		tracer, span := ctxutil.InitTestTracing("my-test")
//		defer tracer.Cleanup()
//		...
//		span.End()
//		require.Contains(t, tracer.Trace.String(), `"name":"config"`)
func InitTestTracing(spanName string) (*TestTracer, trace.Span) {
	reset := EnableTracing(true)
	t := NewTestTracer(reset)
	return t, current.InitTracing(t, spanName)
}

// StartSpan creates new sub-span of the goroutine's current span or a noop
// span if there is no current span.
func StartSpan(spanName string, opts ...trace.StartOption) trace.Span {
	return current.StartSpan(spanName, opts...)
}

// WithSpan creates a new sub-span of the goroutine's current span and executes
// the given function within the sub-span.
func WithSpan(
	spanName string,
	fn func() error,
	opts ...trace.StartOption) error {

	span := current.StartSpan(spanName, opts...)
	defer span.End()
	return fn()
}

// Span retrieves the current span of this goroutine.
func Span() trace.Span {
	return current.Span()
}

// Ctx returns the current tracing context. Should only be used for backwards
// compatibility until old code is converted to use StartSpan directly.
func Ctx() context.Context {
	return current.Ctx()
}

// StartSubSpan creates new sub-span of the context's current span or a noop
// span if there is no current span.
func StartSubSpan(
	ctx context.Context,
	spanName string,
	opts ...trace.StartOption) (context.Context, trace.Span) {

	if ctx == nil {
		return nil, trace.NoopSpan{}
	}
	return trace.SpanFromContext(ctx).Tracer().Start(ctx, spanName, opts...)
}

// WithSubSpan executes the given function in a new sub-span of the context's
// current span or a noop span if there is no current span.
func WithSubSpan(
	ctx context.Context,
	spanName string,
	fn func(ctx context.Context) error,
	opts ...trace.StartOption) error {

	if ctx == nil {
		return fn(context.Background())
	}
	return trace.SpanFromContext(ctx).Tracer().WithSpan(ctx, spanName, fn, opts...)
}
