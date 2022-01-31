package traceutil_test

import (
	"context"
	"fmt"
	"io"
	"testing"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/api/kv"
	"go.opentelemetry.io/otel/api/kv/value"

	"github.com/eluv-io/common-go/util/traceutil"
)

func TestStartSubSpan(t *testing.T) {
	var trc *traceutil.TraceInfo

	tracer := traceutil.NewTracer("test-tracer", func(trcInfo *traceutil.TraceInfo) {
		trc = trcInfo
		fmt.Println(trcInfo.MinimalString())
		// fmt.Println(jsonutil.MustPretty(trc))
	})

	ctx, span := tracer.Start(context.Background(), "root-span")
	require.NotNil(t, ctx)
	require.NotNil(t, span)

	ctx, sub := traceutil.StartSubSpan(ctx, "sub-span")
	require.NotNil(t, ctx)
	require.NotNil(t, sub)

	sub.End()
	span.End()

	require.Equal(t, "root-span", trc.RootSpan().Name)
	require.Len(t, trc.RootSpan().Children, 1)
	require.Equal(t, "sub-span", trc.RootSpan().Children[0].Name)
}

func TestWithSubSpan(t *testing.T) {
	var trc *traceutil.TraceInfo
	var err error

	tracer := traceutil.NewTracer("test-tracer", func(trcInfo *traceutil.TraceInfo) {
		trc = trcInfo
		fmt.Println(trcInfo.MinimalString())
		// fmt.Println(jsonutil.MustPretty(trc))
	})

	err = tracer.WithSpan(context.Background(), "root-span", func(ctx context.Context) error {
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, "root-span", trc.RootSpan().Name)

	err = tracer.WithSpan(context.Background(), "root-span", func(ctx context.Context) error {
		return io.EOF
	})
	require.Error(t, io.EOF, err)
	require.Equal(t, "root-span", trc.RootSpan().Name)
	require.Equal(t, kv.Key("error"), trc.RootSpan().Attributes[0].Key)
	require.Equal(t, value.BOOL, trc.RootSpan().Attributes[0].Value.Type())
	require.Equal(t, true, trc.RootSpan().Attributes[0].Value.AsInterface())

	err = tracer.WithSpan(context.Background(), "root-span", func(ctx context.Context) error {
		return traceutil.WithSubSpan(ctx, "sub-span", func(ctx context.Context) error {
			return nil
		})
	})
	require.NoError(t, err)
	require.Equal(t, "root-span", trc.RootSpan().Name)
	require.Len(t, trc.RootSpan().Children, 1)
	require.Equal(t, "sub-span", trc.RootSpan().Children[0].Name)
}
