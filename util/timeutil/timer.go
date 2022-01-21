package timeutil

import (
	"time"

	"github.com/eluv-io/utc-go"
)

// Timer is a wrapper around the standard time.Timer that provides additional state information.
type Timer struct {
	*time.Timer
	doneTime  utc.UTC
	resetTime utc.UTC
	stopTime  utc.UTC
}

// NewTimer creates a new Timer that will send the current time on its channel after at least duration d.
func NewTimer(d time.Duration) *Timer {
	t := Timer{}
	t.Timer = time.AfterFunc(d, func() {
		t.doneTime = utc.Now()
		t.stopTime = t.doneTime
	})
	t.resetTime = utc.Now()
	return &t
}

// AfterFunc waits for the duration to elapse and then calls f in its own goroutine.
func AfterFunc(d time.Duration, f func()) *Timer {
	t := Timer{}
	t.Timer = time.AfterFunc(d, func() {
		t.doneTime = utc.Now()
		t.stopTime = t.doneTime
		f()
	})
	t.resetTime = utc.Now()
	return &t
}

// Reset changes the timer to expire after duration d.
// It returns true if the timer had been active, false if the timer had expired or been stopped.
func (t *Timer) Reset(d time.Duration) bool {
	defer func() {
		t.resetTime = utc.Now()
	}()
	t.doneTime = utc.UTC{}
	t.stopTime = utc.UTC{}
	return t.Timer.Reset(d)
}

// Stop prevents the timer from firing.
// It returns true if the call stops the timer, false if the timer has already expired or been stopped.
func (t *Timer) Stop() bool {
	stopped := t.Timer.Stop()
	if stopped {
		t.stopTime = utc.Now()
	}
	return stopped
}

// DoneTime returns the time at which the timer elapsed the full duration.
func (t *Timer) DoneTime() utc.UTC {
	return t.doneTime
}

// ResetTime returns the time at which the timer was started or last reset.
func (t *Timer) ResetTime() utc.UTC {
	return t.resetTime
}

// StopTime returns the time at which the timer was stopped or done.
func (t *Timer) StopTime() utc.UTC {
	return t.stopTime
}
