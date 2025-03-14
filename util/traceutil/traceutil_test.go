package traceutil_test

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/jsonutil"
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

func TestSlowSpanInit(t *testing.T) {
	rootSp := traceutil.InitTracing("slow-span-test", true)
	require.True(t, rootSp.IsRecording())
	require.True(t, rootSp.SlowOnly())

	span := traceutil.StartSpan("should-not-appear")
	require.NotNil(t, span)
	require.False(t, span.IsRecording())

	slowSp := traceutil.StartSlowSpan("should-appear")
	require.NotNil(t, slowSp)
	require.True(t, slowSp.IsRecording())
	require.True(t, slowSp.SlowOnly())

	slowSp.SetSlowCutoff(500 * time.Millisecond)
	time.Sleep(1 * time.Second)

	slowSp.End()

	require.Greater(t, slowSp.Duration(), time.Duration(0))
	require.Equal(t, 500*time.Millisecond, slowSp.SlowCutoff())
	span.End()

	// Testing marshalling before ending
	s := rootSp.Json()
	require.Equal(t, 1, strings.Count(s, "should-appear"))
	require.Equal(t, 0, strings.Count(s, "should-not-appear"))
	require.Equal(t, 1, strings.Count(s, "slow-span-test"))

	rootSp.End()

	s = rootSp.Json()
	require.Equal(t, 1, strings.Count(s, "should-appear"))
	require.Equal(t, 0, strings.Count(s, "should-not-appear"))
	require.Equal(t, 1, strings.Count(s, "slow-span-test"))

	s2, err, foundSlow := rootSp.MarshalSlowOnly()
	require.NoError(t, err)
	require.True(t, foundSlow)
	require.Equal(t, 1, strings.Count(string(s2), "should-appear"))
}

func TestInitTracing(t *testing.T) {
	rootSp := traceutil.InitTracing("init-tracing-test", false)
	require.True(t, rootSp.IsRecording())
	require.False(t, rootSp.SlowOnly())

	span := traceutil.StartSpan("should-appear-regular")
	require.NotNil(t, span)
	require.True(t, span.IsRecording())

	slowSp := traceutil.StartSlowSpan("should-appear-slow")
	require.NotNil(t, slowSp)
	require.True(t, slowSp.IsRecording())
	slowSp.Attribute("attr-1", "arbitrary-unique-value")

	slowSp.End()
	span.End()
	rootSp.End()

	s := rootSp.Json()
	require.Equal(t, 1, strings.Count(s, "should-appear-regular"))
	require.Equal(t, 1, strings.Count(s, "should-appear-slow"))
	require.Equal(t, 1, strings.Count(s, "init-tracing-test"))
	require.Equal(t, 1, strings.Count(s, "attr-1"))
	require.Equal(t, 1, strings.Count(s, "arbitrary-unique-value"))
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

func TestSlowSpanEnd2End(t *testing.T) {

	t.Run("not slow", func(t *testing.T) {
		rootSp := traceutil.InitTracing("slow-span-test", true)
		rootSp.Attribute("label", "api-request")

		handleRequest(0)

		bts, err, ok := rootSp.MarshalSlowOnly()
		require.Empty(t, bts)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("slow", func(t *testing.T) {
		rootSp := traceutil.InitTracing("slow-span-test", true)
		rootSp.Attribute("label", "api-request")

		{
			span := traceutil.StartSpan("should-not-appear")
			require.NotNil(t, span)
			require.False(t, span.IsRecording())
			span.End()
		}

		handleRequest(time.Second)

		bts, err, ok := rootSp.MarshalSlowOnly()
		require.True(t, ok)
		require.NoError(t, err)
		fmt.Println(jsonutil.MustPretty(string(bts)))
		require.Contains(t, string(bts), "live-playout")
		require.Contains(t, string(bts), "api-request")
		require.NotContains(t, string(bts), "should-not-appear")
	})

	t.Run("slow, but no slow span initialized", func(t *testing.T) {
		handleRequest(time.Second)

		bts, err, ok := traceutil.Span().MarshalSlowOnly()
		require.Empty(t, bts)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("full tracing, not slow", func(t *testing.T) {
		rootSp := traceutil.InitTracing("slow-span-test", false)
		rootSp.Attribute("label", "api-request")

		{
			span := traceutil.StartSpan("should-not-appear")
			require.NotNil(t, span)
			require.True(t, span.IsRecording())
			span.End()
		}

		handleRequest(0)

		bts, err, ok := rootSp.MarshalSlowOnly()
		require.Empty(t, bts)
		require.NoError(t, err)
		require.False(t, ok)
	})

	t.Run("full tracing, slow", func(t *testing.T) {
		rootSp := traceutil.InitTracing("slow-span-test", false)
		rootSp.Attribute("label", "api-request")

		{
			span := traceutil.StartSpan("should-appear")
			require.NotNil(t, span)
			require.True(t, span.IsRecording())
			span.End()
		}

		handleRequest(time.Second)

		bts, err, ok := rootSp.MarshalSlowOnly()
		require.True(t, ok)
		require.NoError(t, err)
		fmt.Println(jsonutil.MustPretty(string(bts)))
		require.Contains(t, string(bts), "live-playout")
		require.Contains(t, string(bts), "api-request")
		require.Contains(t, string(bts), "should-appear")
	})

}

func handleRequest(sleep time.Duration) {
	span := traceutil.StartSlowSpan("handle-request")
	defer span.End()

	livePlayout(sleep)
}

func livePlayout(sleep time.Duration) {
	span := traceutil.StartSlowSpan("live-playout")
	defer span.End()

	traceutil.Span().FindAncestorByAttr("label", "api-request").SetSlowCutoff(500 * time.Millisecond)
	// alternatively, we could also use the local span instance since we have it:
	// span.FindAncestorByAttr("label", "api-request").SetSlowCutoff(500 * time.Millisecond)

	if sleep > 0 {
		time.Sleep(sleep)
	}
}
