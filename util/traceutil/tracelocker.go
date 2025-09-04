package traceutil

import (
	"sync"

	"github.com/eluv-io/common-go/util/stackutil"
	"github.com/eluv-io/errors-go"
)

// TraceLocker is a sync.Locker that tracks lock/unlock events in a trace span. It is useful to trace lock contention
// and duration of critical sections. Tracing must be enabled on the current goroutine for it to have any effect (see
// traceutil.InitTracing).
//
// The generated span starts when Lock() is called and ends - by default - when Unlock() is called. In addition, it adds
// an event "locked" right after the lock is successfully acquired.
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
//
// In this default mode, Lock() and Unlock() must be called from the same goroutine - otherwise the trace spans may get
// corrupted.
//
// If WithSpanUnlock(false) is called on construction, the span will end right after acquiring the lock instead. In this
// mode, the duration of the critical section is not tracked, but Unlock() may be called safely from a different
// goroutine.
type TraceLocker struct {
	mu                 sync.Mutex
	name               string
	logCaller          bool
	logCallerFullstack bool
	spanUnlock         bool
}

// TraceLockerBuilder is a builder for TraceLocker.
type TraceLockerBuilder struct {
	tl *TraceLocker
}

// WithCaller controls whether the locker will log the caller's call location.
func (t *TraceLockerBuilder) WithCaller(b bool) *TraceLockerBuilder {
	t.tl.logCaller = b
	return t
}

// WithFullstack controls whether the locker will log the caller's full call stack.
func (t *TraceLockerBuilder) WithFullstack(b bool) *TraceLockerBuilder {
	t.tl.logCallerFullstack = b
	return t
}

// WithSpanUnlock controls whether the locker will end the span when unlocking (true) or right after acquiring the lock
// (false).
func (t *TraceLockerBuilder) WithSpanUnlock(b bool) *TraceLockerBuilder {
	t.tl.spanUnlock = b
	return t
}

func (t *TraceLockerBuilder) Build() *TraceLocker {
	return t.tl
}

// BuildTraceLocker returns a builder for a new TraceLocker with the given name. By default, it will log the caller's
// call location and end the span when unlocking. Call any of the With* methods to change this behavior on construction
// of the TraceLocker (and never after first use!).
func BuildTraceLocker(name string) *TraceLockerBuilder {
	return &TraceLockerBuilder{
		tl: &TraceLocker{
			name:               name,
			logCaller:          true,
			logCallerFullstack: false,
			spanUnlock:         true,
		},
	}
}

// Lock locks the mutex and starts a new trace span. The caller must ensure that it releases the lock (by calling
// Unlock) from the same goroutine - otherwise the trace spans may get corrupted.
func (t *TraceLocker) Lock() {
	span := StartSpan(t.name)
	if t.logCallerFullstack {
		span.Attribute("caller", errors.E("stack"))
	} else if t.logCaller {
		span.Attribute("caller", stackutil.Caller(1))
	}
	t.mu.Lock()
	span.Event("locked", nil)
	if !t.spanUnlock {
		span.End()
	}
}

// Unlock unlocks the mutex and ends the trace span. The caller must ensure that the mutex was locked (by calling Lock)
// from the same goroutine - otherwise the trace spans may get corrupted.
func (t *TraceLocker) Unlock() {
	t.mu.Unlock()
	if t.spanUnlock {
		Span().End()
	}
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
