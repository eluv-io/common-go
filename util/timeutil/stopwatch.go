package timeutil

import (
	"time"

	"github.com/qluvio/content-fabric/format/utc"
)

type StopWatch struct {
	startTime utc.UTC
	stopTime  utc.UTC
}

// StartWatch starts and returns a stopwatch.
//
// The StopWatch is super simple:
//		sw := timeutil.StartWatch()
//		...
//		sw.Stop()
//		sw.Duration() // get the elapsed time between start and stop time
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
