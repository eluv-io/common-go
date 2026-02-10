package rtp

// SequenceUnwrapper computes a 64-bit sequence from a 16-bit wrap-around sequence number.
type SequenceUnwrapper struct {
	hasLast  bool   // true if last has been set
	last     uint16 // the last wrapped sequence number
	current  int64  // the last unwrapped sequence number
	previous int64  // the previous unwrapped sequence number
}

// Unwrap returns the previous and current 64-bit sequence number corresponding to the given 16-bit sequence number. On
// the first call, the previous sequence number is fabricated as (current - 1).
func (u *SequenceUnwrapper) Unwrap(seq uint16) (previous, current int64) {
	if !u.hasLast {
		u.last = seq
		u.hasLast = true
		u.current = int64(seq)
		u.previous = u.current - 1
		return u.previous, u.current
	}

	diff := int16(seq - u.last)
	u.previous = u.current
	u.current = u.current + int64(diff)
	u.last = seq
	return u.previous, u.current
}

// Previous returns the previous unwrapped sequence number.
func (u *SequenceUnwrapper) Previous() int64 {
	return u.previous
}

// Current returns the most recent unwrapped sequence number.
func (u *SequenceUnwrapper) Current() int64 {
	return u.current
}
