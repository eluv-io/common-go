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
// sync.Pool, without having to cast interface{} as *[]byte.
// Note: Pool methods act on *[]byte mainly to avoid unnecessary allocations;
// see https://github.com/golang/go/blob/9c8bf0e7/src/sync/example_pool_test.go#L17-L19
type Pool struct {
	BufSize   int         // Size of buffers
	p         *sync.Pool  // Backing pool
	created   Counter     // Metric for created buffers
	retrieved Counter     // Metric for retrieved buffers
	released  Counter     // Metric for released buffers
	locker    sync.Locker // usually a noop locker (a real locker is only used in tests)
}

// NewPool creates a new buffer pool to service buffers of size bufSize
func NewPool(bufSize int) *Pool {
	p := &Pool{}
	p.BufSize = bufSize
	p.p = &sync.Pool{New: p.new}
	p.locker = &sync.Mutex{}
	return p
}

// WithLocker sets the locker used for guarding critical sections. May be set to a no-op implementation if external
// locking is used.
func (p *Pool) WithLocker(l sync.Locker) *Pool {
	p.locker = l
	return p
}

// New force creates a new buffer. The reference counter to be set for the
// buffer defaults to 1.
func (p *Pool) New() *[]byte {
	buf := p.new().(*[]byte)
	p.setCounter(*buf, 1)
	if p.retrieved != nil {
		p.retrieved.Add(1)
	}
	return buf
}

// NewN force creates a new buffer. count specifies the reference counter to be
// set for the buffer.
func (p *Pool) NewN(count byte) *[]byte {
	buf := p.new().(*[]byte)
	p.setCounter(*buf, count)
	if p.retrieved != nil {
		p.retrieved.Add(1)
	}
	return buf
}

// Get retrieves a buffer from the pool; if no previous buffers are available,
// a new buffer is automatically created. The reference counter to be set for
// the buffer defaults to 1.
func (p *Pool) Get() *[]byte {
	buf := p.p.Get().(*[]byte)
	p.setCounter(*buf, 1)
	if p.retrieved != nil {
		p.retrieved.Add(1)
	}
	return buf
}

// GetN retrieves a buffer from the pool; if no previous buffers are available,
// a new buffer is automatically created. count specifies the reference counter
// to be set for the buffer.
func (p *Pool) GetN(count byte) *[]byte {
	buf := p.p.Get().(*[]byte)
	p.setCounter(*buf, count)
	if p.retrieved != nil {
		p.retrieved.Add(1)
	}
	return buf
}

// Put releases a reference to the given buffer, by decrementing the buffer's
// reference counter. If the counter reaches 0, the buffer is released back
// into the pool. The caller should no longer use the buffer after calling.
// Buffers that have been re-sliced will be ignored.
func (p *Pool) Put(buf *[]byte) {
	if buf == nil || *buf == nil {
		log.Warn("buffer not released back into pool", "reason", "nil buffer")
		return
	} else if cap(*buf) != p.BufSize+1 {
		log.Warn("buffer not released back into pool", "expected_size", p.BufSize+1, "actual_size", cap(*buf))
		return
	} else if len(*buf) != p.BufSize {
		log.Warn("buffer resized and released back into pool", "expected_size", p.BufSize, "actual_size", len(*buf))
		*buf = (*buf)[:p.BufSize]
	}
	// Decrement buffer's reference counter
	if p.decrCounter(*buf) {
		// Release buffer back into pool
		p.p.Put(buf)
		if p.released != nil {
			p.released.Add(1)
		}
	}
}

func (p *Pool) SetMetrics(created, retrieved, released Counter) {
	p.created = created
	p.retrieved = retrieved
	p.released = released
}

// Creates a byte buffer of configured size.
func (p *Pool) new() interface{} {
	buf := make([]byte, p.BufSize+1)[:p.BufSize]
	if p.created != nil {
		p.created.Add(1)
	}
	return &buf
}

// Sets the buffer's reference counter. Only the first count is used, if
// specified. Count is by default 1.
func (p *Pool) setCounter(buf []byte, count byte) {
	buf[:p.BufSize+1][p.BufSize] = count
}

// decrCounter decrements the buffer's reference counter by 1; returns true if
// the buffer should be released back into the pool.
//
// note: tests are reading the ref count from buf while this function is writing
// it to buf. This is why tests set a locker - alternatively we could use a directive
// 'go:norace' to disable the race detector in unit-tests where we are reading
// the ref count.
func (p *Pool) decrCounter(buf []byte) bool {
	buf = buf[:p.BufSize+1]
	p.locker.Lock()
	defer p.locker.Unlock()
	n := buf[p.BufSize]
	if n > 0 {
		buf[p.BufSize] = n - 1
		if n == 1 {
			return true
		}
	}
	return false
}
