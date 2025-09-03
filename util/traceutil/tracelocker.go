package traceutil

import (
	"sync"
)

// TraceLocker is a sync.Locker that tracks lock/unlock events in a trace span. It is useful to trace lock contention
// and duration of critical sections. Tracing must be enabled on the current goroutine for it to have any effect (see
// traceutil.InitTracing).
//
// The generated span starts when Lock() is called and ends when Unlock() is called. In addition, it adds an event
// "locked" right after the lock is successfully acquired.
//
//	{
//	  "name": "example-lock",
//	  "time": "5ms",
//	  "evnt": [
//	    {
//	      "name": "locked",
//	      "at": "3ms"
//	    }
//	  ]
//	}
type TraceLocker struct {
	mu   sync.Mutex
	name string
}

// NewTraceLocker creates a new TraceLocker with the given name.
func NewTraceLocker(name string) *TraceLocker {
	return &TraceLocker{
		name: name,
	}
}

// Lock locks the mutex and starts a new trace span. Ensure that the mutex is unlocked by calling Unlock on the same
// goroutine.
func (t *TraceLocker) Lock() {
	span := StartSpan(t.name)
	t.mu.Lock()
	span.Event("locked", nil)
}

// Unlock unlocks the mutex and ends the trace span. Ensure that the mutex was locked by calling Lock on the same
// goroutine.
func (t *TraceLocker) Unlock() {
	t.mu.Unlock()
	Span().End()
}

// LockUnlock is a convenience method that locks the mutex and returns a function that unlocks it. This is useful for
// deferring the unlock right after locking:
//
//	defer NewTraceLocker("update").LockUnlock()()
func (t *TraceLocker) LockUnlock() (unlock func()) {
	span := StartSpan(t.name)
	t.mu.Lock()
	span.Event("locked", nil)
	return func() {
		t.mu.Unlock()
		span.End()
	}
}
