package traceutil

import (
	"context"

	"github.com/eluv-io/common-go/util/ctxutil"
	"github.com/eluv-io/common-go/util/traceutil/trace"
)

// current returns the current ContextStack
func current() ctxutil.ContextStack {
	return ctxutil.Current()
}

// InitTracing initializes performance tracing for the current goroutine.
//
// If slowOnly is true, only spans that are explicitly created with StartSlowSpan wiill be recorded,
// and even then only persisted if they end up being slower than expected.
func InitTracing(spanName string, slowOnly bool) trace.Span {
	return current().InitTracing(spanName, slowOnly)
}

// StartSpan creates new sub-span of the goroutine's current span or a noop
// span if there is no current span.
func StartSpan(spanName string) trace.Span {
	sp := current().Span()
	if sp.SlowOnly() {
		return trace.NoopSpan{}
	}
	return current().StartSpan(spanName)
}

// StartSlowSpan creates a new sub-span of the goroutine's current slow span or a noop span if there
// is no current span. If tracing is fully enabled, this has the exact same behavior as StartSpan.
func StartSlowSpan(spanName string) trace.Span {
	return current().StartSpan(spanName)
}

// WithSpan creates a new sub-span of the goroutine's current span and executes
// the given function within the sub-span.
func WithSpan(spanName string, fn func() error) error {
	span := current().StartSpan(spanName)
	defer span.End()

	err := fn()
	if err != nil {
		span.Attribute("error", err)
	}
	return err
}

// Span retrieves the current span of this goroutine. If this function is used to add attributes or
// events to the span, span.SlowOnly should be checked to ensure that detailed tracing is not added
// when unnecessary.
func Span() trace.Span {
	return current().Span()
}

// Ctx returns the current tracing context. Should only be used for backwards
// compatibility until old code is converted to use StartSpan directly.
func Ctx() context.Context {
	return current().Ctx()
}

// StartSubSpan creates new sub-span of the context's current span or a noop
// span if there is no current span.
func StartSubSpan(ctx context.Context, spanName string) (context.Context, trace.Span) {
	if ctx == nil {
		return nil, trace.NoopSpan{}
	}
	return trace.SpanFromContext(ctx).Start(ctx, spanName)
}

// WithSubSpan executes the given function in a new sub-span of the context's
// current span or a noop span if there is no current span.
func WithSubSpan(ctx context.Context, spanName string, fn func(ctx context.Context) error) error {
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, span := trace.SpanFromContext(ctx).Start(ctx, spanName)
	defer span.End()
	err := fn(ctx)
	if err != nil {
		span.Attribute("error", err)
	}
	return err
}
