package byteutil

import (
	"sync"

	"github.com/eluv-io/log-go"
)

type Counter interface {
	Add(delta float64)
}

// Pool is a buffer pool that allows for re-using previously allocated buffers
// of a set size. Buffers that have been completely released from use are cycled
// back into the pool. If no previous buffers are available for re-use, new
// buffers are created as necessary. The pool may automatically expand or shrink
// according to demand; see documentation for sync.Pool, which backs this
// implementation. When retrieving a buffer from the pool, a reference counter
// is set for the buffer, such that when the buffer is "put back" into the pool,
// the counter is simply decreased. Only when the counter reaches 0 will the
// buffer be released back into the pool. The reference counter is stored as an
// additional, last element of each buffer. Callers of Pool are expected not to
// alter this reference counter in any way or to attempt to release a buffer
// that has been re-sliced. Pool is designed to be a drop-in replacement for
// sync.Pool, without having to cast interface{} as []byte.
type Pool struct {
	BufSize  int         // Size of buffers
	p        *sync.Pool  // Backing pool
	q        chan []byte // Queue of released buffers
	created  Counter     // Metric for created buffers
	released Counter     // Metrics for released buffers
}

// NewPool creates a new buffer pool to service buffers of size bufSize
func NewPool(bufSize int) *Pool {
	p := &Pool{}
	p.BufSize = bufSize
	p.p = &sync.Pool{New: p.new}
	p.q = make(chan []byte)

	// Process buffers to be released sequentially in background
	go func() {
		for buf := range p.q {
			// Decrement buffer's reference counter
			if p.decrCounter(buf) {
				// Release buffer back into pool
				p.p.Put(buf)
				if p.released != nil {
					p.released.Add(1)
				}
			}
		}
	}()

	return p
}

// New force creates a new buffer. Optionally, count specifies the reference
// counter to be set for the buffer; if ommitted, count defaults to 1. Only the
// first specified count is used.
func (p *Pool) New(count ...byte) []byte {
	buf := p.new().([]byte)
	p.setCounter(buf, count)
	return buf
}

// Get retrieves a buffer from the pool; if no previous buffers are available,
// a new buffer is automatically created. Optionally, count specifies the
// reference counter to be set for the buffer; if ommitted, count defaults to 1.
// Only the first specified count is used.
func (p *Pool) Get(count ...byte) []byte {
	buf := p.p.Get().([]byte)
	p.setCounter(buf, count)
	return buf
}

// Put releases a reference to the given buffer, by decrementing the buffer's
// reference counter. If the counter reaches 0, the buffer is released back
// into the pool. The caller should no longer use the buffer after calling.
// Buffers that have been re-sliced will be ignored.
func (p *Pool) Put(buf []byte) {
	if p.q != nil && cap(buf) == p.BufSize+1 {
		// Add buffer to queue, to be released sequentially in background
		p.q <- buf[:p.BufSize]
	} else if buf != nil {
		log.Debug("buffer not released back into pool", "expected_size", p.BufSize+1, "actual_size", cap(buf))
	}
}

// Close closes the pool. A closed pool will still service buffers, but buffers
// will not be re-added to the pool for re-use.
// Caveat: Close currently must not be executed concurrently with any Put calls
func (p *Pool) Close() error {
	close(p.q)
	p.q = nil
	return nil
}

func (p *Pool) SetMetrics(created, released Counter) {
	p.created = created
	p.released = released
}

// Creates a byte buffer of configured size.
func (p *Pool) new() interface{} {
	buf := make([]byte, p.BufSize+1)[:p.BufSize]
	if p.created != nil {
		p.created.Add(1)
	}
	return buf
}

// Sets the buffer's reference counter. Only the first count is used, if
// specified. Count is by default 1.
func (p *Pool) setCounter(buf []byte, count []byte) {
	n := byte(1)
	if len(count) > 0 {
		n = count[0]
	}
	buf[:p.BufSize+1][p.BufSize] = n
}

// Decrements the buffer's reference counter by 1; returns true if the buffer
// should be released back into the pool.
func (p *Pool) decrCounter(buf []byte) bool {
	buf = buf[:p.BufSize+1]
	n := buf[p.BufSize]
	if n > 0 {
		buf[p.BufSize] = n - 1
		if n == 1 {
			return true
		}
	}
	return false
}
