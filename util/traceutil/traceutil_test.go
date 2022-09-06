package traceutil_test

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/traceutil"
	"github.com/eluv-io/common-go/util/traceutil/trace"
)

func TestStartSubSpan(t *testing.T) {
	ctx, span := trace.StartRootSpan(context.Background(), "root-span")
	require.NotNil(t, ctx)
	require.NotNil(t, span)

	ctx, sub := traceutil.StartSubSpan(ctx, "sub-span")
	require.NotNil(t, ctx)
	require.NotNil(t, sub)

	sub.End()
	span.End()

	root := span.(*trace.RecordingSpan)
	require.Equal(t, "root-span", root.Data.Name)
	require.Len(t, root.Data.Subs, 1)
	require.Equal(t, "sub-span", root.Data.Subs[0].(*trace.RecordingSpan).Data.Name)
}

func TestWithSubSpan(t *testing.T) {
	var err error

	ctx, span := trace.StartRootSpan(context.Background(), "root-span")
	root := span.(*trace.RecordingSpan)
	require.Equal(t, "root-span", root.Data.Name)

	err = traceutil.WithSubSpan(ctx, "sub", func(ctx context.Context) error {
		return io.EOF
	})
	require.Error(t, io.EOF, err)

	sub := root.Data.Subs[0].(*trace.RecordingSpan)
	require.Equal(t, "sub", sub.Data.Name)
	require.Equal(t, io.EOF, sub.Data.Attr["error"])

	err = traceutil.WithSubSpan(ctx, "sub-a", func(ctx context.Context) error {
		return traceutil.WithSubSpan(ctx, "sub-b", func(ctx context.Context) error {
			return nil
		})
	})
	require.NoError(t, err)

	subA := root.Data.Subs[1].(*trace.RecordingSpan)
	subB := subA.Data.Subs[0].(*trace.RecordingSpan)

	require.Equal(t, "sub-a", subA.Data.Name)
	require.Equal(t, "sub-b", subB.Data.Name)
}
