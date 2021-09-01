package ctxutil

import (
	"context"

	"go.opentelemetry.io/otel/api/trace"
)

var noopInstance = noop{}

func Noop() ContextStack {
	return noopInstance
}

type noop struct{}

func (n noop) Ctx() context.Context {
	return context.Background()
}

func (n noop) Push(ctx context.Context) func() {
	return func() {}
}

func (n noop) WithValue(_ interface{}, _ interface{}) func() {
	return func() {}
}

func (n noop) Go(fn func()) {
	go fn()
}

func (n noop) InitTracing(_ trace.Tracer, _ string, _ ...trace.StartOption) trace.Span {
	return trace.NoopSpan{}
}

func (n noop) StartSpan(_ string, _ ...trace.StartOption) trace.Span {
	return trace.NoopSpan{}
}

func (n noop) Span() trace.Span {
	return trace.NoopSpan{}
}
