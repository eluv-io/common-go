package timeutil_test

import (
	"time"

	"github.com/eluv-io/common-go/util/timeutil"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

func ExampleWarnSlowOp() {
	timeutil.WarnSlowOp(
		func() {
			time.Sleep(20 * time.Millisecond)
		},
		log.Warn,
		10*time.Millisecond,
		"test operation", "id", "0003",
	)

	// Output:
	//
	// 1970-01-01T00:00:00.000Z WARN  slow operation            logger=/ duration=20ms limit=10ms op=test operation id=0003
}

func ExampleStopWatch_WarnSlowOp() {
	watch := timeutil.StartWatch()
	time.Sleep(20 * time.Millisecond)
	watch.WarnSlowOp(log.Warn, 10*time.Millisecond, "test operation", "id", "0003")

	// Output:
	//
	// 1970-01-01T00:00:00.000Z WARN  slow operation            logger=/ duration=20ms limit=10ms op=test operation id=0003
}

// test logger that truncates all durations found in log fields to 10ms
var log = logger{}

type logger struct{}

func (logger) Warn(msg string, fields ...any) {
	for i, field := range fields {
		if d, ok := field.(time.Duration); ok {
			fields[i] = d.Truncate(10 * time.Millisecond)
		}
	}
	defer utc.MockNow(utc.UnixMilli(0))()
	l := elog.New(&elog.Config{Handler: "text", Level: "debug"})
	l.Warn(msg, fields...)
}
