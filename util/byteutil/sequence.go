package byteutil

import (
	"go.uber.org/atomic"
)

// NewSequence creates a new sequence starting at the given integer.
func NewSequence(startWith uint64) *Sequence {
	res := &Sequence{}
	res.seq.Store(startWith)
	return res
}

// Sequence implements a strictly monotonically increasing sequence of integer numbers expressed as byte slices of
// minimal length in big endian format. Sequence is safe for concurrent use.
type Sequence struct {
	seq atomic.Uint64
}

// Next returns the next sequence number as byte slice.
func (s *Sequence) Next() []byte {
	seq := s.seq.Inc() - 1
	if seq == 0 {
		return []byte{0}
	}

	i := 0
	for s := seq; s > 0; s >>= 8 {
		i++
	}

	res := make([]byte, i)
	for seq > 0 {
		i--
		res[i] = byte(seq)
		seq >>= 8
	}

	return res
}
