package traceutil

import (
	"context"

	"go.opentelemetry.io/otel/api/trace"
)

// StartSubSpan creates new sub-span of the context's current span or a noop
// span if there is no current span.
func StartSubSpan(
	ctx context.Context,
	spanName string,
	opts ...trace.StartOption) (context.Context, trace.Span) {

	return trace.SpanFromContext(ctx).Tracer().Start(ctx, spanName, opts...)
}

// WithSubSpan executes the given function in a new sub-span of the context's
// current span or a noop span if there is no current span.
func WithSubSpan(
	ctx context.Context,
	spanName string,
	fn func(ctx context.Context) error,
	opts ...trace.StartOption) error {

	return trace.SpanFromContext(ctx).Tracer().WithSpan(ctx, spanName, fn, opts...)
}
