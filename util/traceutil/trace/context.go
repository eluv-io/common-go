package trace

import "context"

type contextKey struct{}

var activeSpanKey = contextKey{}

// StartRootSpan starts a new top-level span and registers it with the given context.
func StartRootSpan(ctx context.Context, name string, tags ...string) (context.Context, Span) {
	sub := newSpan(name, tags)
	return ContextWithSpan(ctx, sub), sub
}

// ContextWithSpan returns a new `context.Context` that holds a reference to the provided `span`.
func ContextWithSpan(ctx context.Context, span Span) context.Context {
	return context.WithValue(ctx, activeSpanKey, span)
}

// SpanFromContext returns the `Span` previously associated with `ctx`, or a no-op span if none is found.
func SpanFromContext(ctx context.Context) Span {
	val := ctx.Value(activeSpanKey)
	if sp, ok := val.(Span); ok {
		return sp
	}
	return NoopSpan{}
}
