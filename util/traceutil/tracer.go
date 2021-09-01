package traceutil

import (
	"context"

	"go.opentelemetry.io/otel/api/kv"
	"go.opentelemetry.io/otel/api/trace"
	export "go.opentelemetry.io/otel/sdk/export/trace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

// NewTracer creates a new tracer that collects spans with a TraceCollector and
// exports them as a single trace with the given export function.
//
//	* name:   name of the tracer
//	* export: the function called for every completed trace with the trace
//	          marshalled to JSON
func NewTracer(name string, export func(trc *TraceInfo)) trace.Tracer {
	exporter := &Tracer{export: export}

	var tp trace.Provider
	var err error
	tp, err = sdktrace.NewProvider(sdktrace.WithConfig(sdktrace.Config{DefaultSampler: sdktrace.AlwaysSample()}),
		sdktrace.WithSyncer(exporter))
	if err != nil {
		log.Warn("failed to create trace provider", err)
		tp = &trace.NoopProvider{}
	}

	exporter.tracer = tp.Tracer(name)
	return exporter
}

type Tracer struct {
	tracer    trace.Tracer
	collector TraceCollector
	export    func(trc *TraceInfo)
}

func (t *Tracer) Start(
	ctx context.Context,
	spanName string,
	opts ...trace.StartOption) (context.Context, trace.Span) {

	start, span := t.tracer.Start(ctx, spanName, opts...)
	t.collector.AddSpan(span, nil)
	return start, span
}

func (t *Tracer) WithSpan(
	ctx context.Context,
	spanName string,
	fn func(ctx context.Context) error,
	opts ...trace.StartOption) error {

	ctx, span := t.Start(ctx, spanName, opts...)
	defer span.End()

	if err := fn(ctx); err != nil {
		span.SetAttributes(kv.Bool("error", true))
		return err
	}
	return nil
}

func (t *Tracer) ExportSpan(ctx context.Context, data *export.SpanData) {
	trc, completed := t.collector.SpanEnded(data)
	if !completed {
		return
	}

	t.export(trc)
}
