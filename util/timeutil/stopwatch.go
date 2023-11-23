package timeutil

import (
	"time"

	"github.com/eluv-io/utc-go"
)

type StopWatch struct {
	startTime utc.UTC
	stopTime  utc.UTC
}

// StartWatch starts and returns a stopwatch.
//
// The StopWatch is super simple:
//
//	sw := timeutil.StartWatch()
//	...
//	sw.Stop()
//	sw.Duration() // get the elapsed time between start and stop time
func StartWatch() *StopWatch {
	return &StopWatch{startTime: utc.Now()}
}

// Reset resets the stopwatch by re-recording the start time.
func (w *StopWatch) Reset() {
	w.startTime = utc.Now()
}

// Stop stops the stopwatch by recording the stop time. The stopwatch may be
// stopped multiple times, but only the last stop time is retained.
func (w *StopWatch) Stop() {
	w.stopTime = utc.Now()
}

// StartTime returns the time when the stopwatch was started.
func (w *StopWatch) StartTime() utc.UTC {
	return w.startTime
}

// StopTime returns the time when the stopwatch was stopped or the zero value of
// utc.UTC if the stopwatch hasn't been stopped yet.
func (w *StopWatch) StopTime() utc.UTC {
	return w.stopTime
}

// Duration returns the elapsed duration between start and stop time. If the
// stopwatch has not been stopped yet, returns the duration between now and the
// start time.
func (w *StopWatch) Duration() time.Duration {
	if w.stopTime.IsZero() {
		return utc.Now().Sub(w.startTime)
	}
	return w.stopTime.Sub(w.startTime)
}

// String returns the duration as a string.
func (w *StopWatch) String() string {
	return w.Duration().String()
}

// WarnSlowOp logs a warning with the provided log function if the execution of an operation - measured by
// StopWatch.Duration() - is larger than the given limit. Returns true if a warning was logged, false otherwise.
func (w *StopWatch) WarnSlowOp(
	logFn func(msg string, fields ...interface{}),
	limit time.Duration,
	op string,
	fields ...any,
) bool {
	d := w.Duration()
	if d > limit {
		logFn("slow operation", append([]any{"duration", d, "limit", limit, "op", op}, fields...)...)
		return true
	}
	return false
}

// WarnSlowOp executes the given operation and logs a warning with the provided log function if the execution takes
// longer than the given limit. Returns true if a warning was logged, false otherwise.
func WarnSlowOp(
	op func(),
	logFn func(msg string, fields ...interface{}),
	limit time.Duration,
	opName string,
	fields ...any,
) bool {
	w := StartWatch()
	op()
	return w.WarnSlowOp(logFn, limit, opName, fields...)
}
