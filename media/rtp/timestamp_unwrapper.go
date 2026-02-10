package rtp

// TimestampUnwrapper computes a 64-bit sequence from a 32-bit wrap-around timestamp.
type TimestampUnwrapper struct {
	hasLast  bool   // true if last has been set
	last     uint32 // the last wrapped timestamp
	current  int64  // the most recent unwrapped timestamp
	previous int64  // the previous unwrapped timestamp
}

// Unwrap returns the previous and current 64-bit timestamp corresponding to the given 32-bit timestamp. On first call,
// the previous timestamp is fabricated as (current - 1).
func (u *TimestampUnwrapper) Unwrap(seq uint32) (previous, current int64) {
	if !u.hasLast {
		u.last = seq
		u.hasLast = true
		u.current = int64(seq)
		u.previous = u.current - 1
		return u.previous, u.current
	}

	diff := int32(seq - u.last)
	u.previous = u.current
	u.current = u.current + int64(diff)
	u.last = seq
	return u.previous, u.current
}

// Previous returns the previous unwrapped timestamp.
func (u *TimestampUnwrapper) Previous() int64 {
	return u.previous
}

// Current returns the most recent unwrapped timestamp.
func (u *TimestampUnwrapper) Current() int64 {
	return u.current
}
