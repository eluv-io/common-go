package traceutil_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/traceutil"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/log-go/handlers/text"
	"github.com/eluv-io/utc-go"
)

func TestEnableTracing(t *testing.T) {
	defer utc.MockNow(utc.UnixMilli(0))()

	elog.SetDefault(&elog.Config{
		Handler: "text",
		Named: map[string]*elog.Config{
			"/enabled": {
				Level: "trace",
			},
			"/disabled": {
				Level: "debug",
			},
		},
	})

	handler, ok := elog.Root().Handler().(*text.Handler)
	require.True(t, ok, "handler type %T", elog.Root().Handler())

	buf := &bytes.Buffer{}
	handler.Writer = buf

	enabled := elog.Get("/enabled")
	disabled := elog.Get("/disabled")

	require.True(t, enabled.IsTrace())
	require.False(t, disabled.IsTrace())

	doIt := func(log *elog.Log, shouldTrace bool) {
		defer traceutil.InitLogTracing(log, "test-span")()
		span := traceutil.StartSpan("test-span-child")
		defer span.End()

		require.Equal(t, shouldTrace, span.IsRecording())

		log.Info("doing it!", "expect_trace", shouldTrace)
	}

	doIt(enabled, true)
	doIt(disabled, false)
	disabled.SetLevel("trace")
	doIt(disabled, true)

	expected := `
1970-01-01T00:00:00.000Z INFO  doing it!                 logger=/enabled expect_trace=true
1970-01-01T00:00:00.000Z TRACE trace                     logger=/enabled span={"name":"test-span","time":"0s","subs":[{"name":"test-span-child","time":"0s","start":"1970-01-01T00:00:00.000Z","end":"1970-01-01T00:00:00.000Z"}],"start":"1970-01-01T00:00:00.000Z","end":"1970-01-01T00:00:00.000Z"}
1970-01-01T00:00:00.000Z INFO  doing it!                 logger=/disabled expect_trace=false
1970-01-01T00:00:00.000Z INFO  doing it!                 logger=/disabled expect_trace=true
1970-01-01T00:00:00.000Z TRACE trace                     logger=/disabled span={"name":"test-span","time":"0s","subs":[{"name":"test-span-child","time":"0s","start":"1970-01-01T00:00:00.000Z","end":"1970-01-01T00:00:00.000Z"}],"start":"1970-01-01T00:00:00.000Z","end":"1970-01-01T00:00:00.000Z"}
`

	require.Equal(t, strings.TrimLeft(expected, "\n"), buf.String(), "unexpected log output")
}
