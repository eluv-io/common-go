package filterutil

import "sync"

type FilterFn func(item ...interface{}) bool

// Filter is the interface for a general-purpose filter. Its Filter function
// returns true if it accepts an invocation, false otherwise. The items passed
// to the Filter function are implementation dependent - some filters may not
// need any, e.g. count- or time-based filters.
type Filter interface {
	Filter(items ...interface{}) bool
	// Reset resets the filters state. Equivalent to creating a new filter with
	// the same parameters.
	Reset()
}

type BurstCount struct {
	Accept uint
	Deny   uint

	mutex sync.Mutex
	count uint
}

func (b *BurstCount) Reset() {
	b.mutex.Lock()
	b.count = 0
	b.mutex.Unlock()
}

func (b *BurstCount) Filter(_ ...interface{}) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	for {
		b.count++
		switch {
		case b.Accept == 0:
			return false
		case b.count <= b.Accept:
			return true
		case b.count-b.Accept <= b.Deny:
			return false
		}
		b.count = 0
	}
}

type Manual struct {
	accept bool
	mutex  sync.Mutex
}

func (b *Manual) Reset() {
	b.Set(false)
}

func (b *Manual) Set(accept bool) *Manual {
	b.mutex.Lock()
	b.accept = accept
	b.mutex.Unlock()
	return b
}

func (b *Manual) Filter(_ ...interface{}) bool {
	b.mutex.Lock()
	defer b.mutex.Unlock()

	return b.accept
}
