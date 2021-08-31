package traceutil

import (
	"go.opentelemetry.io/otel/api/trace"
)

type TestTracer struct {
	trace.Tracer
	Trace   *TraceInfo
	cleanup func()
}

func (t *TestTracer) Cleanup() {
	t.cleanup()
}

// NewTestTracer creates a tracer for testing that collects the trace in its
// Trace member variable upon calling End() on the root span.
func NewTestTracer(reset func()) *TestTracer {
	tt := &TestTracer{
		cleanup: reset,
	}
	tt.Tracer = NewTracer("test-tracer", func(trcInfo *TraceInfo) {
		tt.Trace = trcInfo
	})
	return tt
}
