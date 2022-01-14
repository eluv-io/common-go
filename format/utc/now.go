package utc

import "time"

// nowFn is the function used to get the current time and can be mocked with MockNow/MockNowFn
var nowFn = now

// New creates a new UTC instance from the given time. Use utc.Now() to get the
// current time.
func New(t time.Time) UTC {
	return UTC{Time: t.UTC(), mono: t}
}

// Now returns the current time as UTC instance. Now can be mocked for tests: see MockNow() function.
func Now() UTC {
	return nowFn()
}

// now is the default, non-mocked value of Now.
func now() UTC {
	return New(time.Now())
}

// MockNowFn allows to replace the Now func variable with a mock function and returns a function to restore the default
// Now() implementation.
//
// Usage:
//	defer MockNow(func() UTC { ... })()
// or
//	reset := MockNow(func() UTC { ... })
//	defer reset()
func MockNowFn(fn func() UTC) (restore func()) {
	nowFn = fn
	return ResetNow
}

// MockNow allows to replace the Now func variable with a function that returns
// the given constant time and returns itself a function to restore the default
// Now() implementation.
//
// Usage:
//	defer MockNow(utc.MustParse("2020-01-01"))()
// or
//	reset := MockNow(utc.MustParse("2020-01-01"))
//	defer reset()
func MockNow(time UTC) (restore func()) {
	return MockNowFn(func() UTC {
		return time
	})
}

// ResetNow resets the Now func to the default implementation.
func ResetNow() {
	nowFn = now
}
