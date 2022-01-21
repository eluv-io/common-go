package timeutil

import (
	"time"

	"github.com/eluv-io/utc-go"
)

// Periodic is a helper that calls a provided function at most once every
// time-period it was configured with.
type Periodic interface {
	Do(f func()) bool
}

// NewPeriodic creates a new Periodic call helper with the given period. The
// returned instance is NOT thread-safe and should be called from a single
// go-routine.
func NewPeriodic(period time.Duration) Periodic {
	return &periodic{
		period: period,
		next:   utc.Now(),
	}
}

type periodic struct {
	period time.Duration // the function is called at most once every period
	next   utc.UTC       // the next possible time the function may be called (again)
}

func (p *periodic) Do(f func()) bool {
	now := utc.Now()
	if now.After(p.next) || now.Equal(p.next) {
		p.next = now.Add(p.period)
		f()
		return true
	}
	return false
}
