package trace

import "context"

type contextKey struct{}
type slowContextKey struct{}

var activeSpanKey = contextKey{}
var slowSpanKey = slowContextKey{}

// StartRootSpan starts a new top-level span and registers it with the given context.
func StartRootSpan(ctx context.Context, name string) (context.Context, Span) {
	sub := newSpan(name)
	return ContextWithSpan(ctx, sub), sub
}

// StartSlowSpan starts a new top-level slow span and registers it with the given context.
func StartSlowSpan(ctx context.Context, name string) (context.Context, Span) {
	sub := newSpan(name)
	return ContextWithSlowSpan(ctx, sub), sub
}

// ContextWithSpan returns a new `context.Context` that holds a reference to the provided `span`.
func ContextWithSpan(ctx context.Context, span Span) context.Context {
	return context.WithValue(ctx, activeSpanKey, span)
}

// ContextWithSlowSpan returns a new `context.Context` that holds a reference to the provided `span`
// as the designated 'slow span'.
func ContextWithSlowSpan(ctx context.Context, span Span) context.Context {
	return context.WithValue(ctx, slowSpanKey, span)
}

// SpanFromContext returns the `Span` previously associated with `ctx`, or a no-op span if none is found.
func SpanFromContext(ctx context.Context) Span {
	val := ctx.Value(activeSpanKey)
	if sp, ok := val.(Span); ok {
		return sp
	}
	return NoopSpan{}
}

// SpanFromContext returns the `Span` that should be use for tracking slow requests, and if the
// returned span should be used only for slow requests.
//
// If full tracing is enabled, that will be the span returned. Otherwise, it will return the span
// specifically for slow requests. If neither is found, it will return a no-op span. The no-op span
// should generally never actually be returned, however.
func SlowSpanFromContext(ctx context.Context) (Span, bool) {
	val := ctx.Value(activeSpanKey)
	if sp, ok := val.(Span); ok && sp.IsRecording() {
		return sp, false
	}
	valSlow := ctx.Value(slowSpanKey)
	if sp, ok := valSlow.(Span); ok {
		return sp, true
	}
	return NoopSpan{}, false
}
