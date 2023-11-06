package traceutil_test

import (
	"context"
	"io"
	"regexp"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/utc-go"

	"github.com/eluv-io/common-go/util/traceutil"
	"github.com/eluv-io/common-go/util/traceutil/trace"
)

const spanJson = `{"name":"root-span","time":"1s","subs":[{"name":"sub-span","time":"1s"}]}`

const spanExtendedJson = `{"name":"root-span","time":"1s","subs":[{"name":"sub-span","time":"1s","start":"2020-02-02T00:00:00","end":"2020-02-02T00:00:01"}],"start":"2020-02-02T00:00:00","end":"2020-02-02T00:00:01"}`

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

func TestExtendedSpan(t *testing.T) {
	var now time.Time
	defer utc.MockNowFn(func() utc.UTC {
		return utc.MustParse("2020-02-02").Add(time.Since(now))
	})()

	now = time.Now()
	ctx, span := trace.StartRootSpan(context.Background(), "root-span")
	require.NotNil(t, ctx)
	require.NotNil(t, span)

	ctx, sub := traceutil.StartSubSpan(ctx, "sub-span")
	require.NotNil(t, ctx)
	require.NotNil(t, sub)

	time.Sleep(time.Second)

	sub.End()
	span.End()

	require.False(t, span.MarshalExtended())
	require.False(t, sub.MarshalExtended())
	require.Equal(t, spanJson, span.Json())

	span.SetMarshalExtended()
	require.True(t, span.MarshalExtended())
	require.True(t, sub.MarshalExtended())
	require.Equal(t, spanExtendedJson, removeMs(span.Json()))
}

func removeMs(s string) string {
	return regexp.MustCompile(`\.00\dZ`).ReplaceAllString(s, "")
}
