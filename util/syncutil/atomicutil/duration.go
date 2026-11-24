package atomicutil

import (
	"sync/atomic"
	"time"
)

// Duration is a time.Duration that is safe for concurrent use. The zero value is a zero duration.
type Duration struct {
	v atomic.Int64
}

// Load returns the duration.
func (d *Duration) Load() time.Duration { return time.Duration(d.v.Load()) }

// Store sets the duration.
func (d *Duration) Store(t time.Duration) { d.v.Store(int64(t)) }
