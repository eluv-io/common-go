package multiqueue

import (
	"sync"

	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
)

var log = elog.Get("/eluvio/util/multiqueue")

type Input interface {
	Push(interface{})
	Close()
}

type MultiQueue interface {
	// NewInput adds a new input queue. Cap is the buffer capacity.
	NewInput(cap int) Input
	// The current number of current queues.
	InputCount() int
	// Pop returns the next element from any of the input queues. The call
	// blocks until an element is available or the MultiQueue is closed. The res
	// value may be nil if nil is submitted into an input queue. If closed is
	// true, the res value must be ignored.
	Pop() (res interface{}, closed bool)
	// Close closes this multiqueue. Returns an error if the queue still has
	// active inputs.
	Close() error
}

func New() MultiQueue {
	mc := &multiQueue{
		cond: sync.NewCond(&sync.Mutex{}),
	}
	return mc
}

type multiQueue struct {
	in     set
	start  *entry // the round-robin iteration start entry
	cond   *sync.Cond
	closed bool
}

func (m *multiQueue) NewInput(cap int) Input {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()

	p := newInput(cap, m.cond)
	m.in.Add(p)
	m.cond.Signal()

	return p
}

func (m *multiQueue) InputCount() int {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()

	return m.in.size
}

// Pop returns the next element from any of the input queues. Blocks until an
// element is available or the MultiQueue is closed. The res value may be nil if
// nil is submitted into an input queue. If closed is true, the res value must
// be ignored.
func (m *multiQueue) Pop() (res interface{}, closed bool) {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()

	for {
		for m.in.size == 0 {
			if m.closed {
				return nil, true
			}
			m.cond.Wait()
		}

		hasRes := false
		m.in.Iterate(m.start, func(e *entry) (cont bool) {
			val, ok, inClosed := e.val.Pop()
			if inClosed {
				m.in.remove(e)
				m.start = nil
				return true
			}
			if ok {
				m.start = e.next
				res = val
				hasRes = true
				return false
			}
			return true
		})
		if hasRes {
			return res, false
		}

		m.cond.Wait()
	}
}

func (m *multiQueue) Close() error {
	m.cond.L.Lock()
	defer m.cond.L.Unlock()

	if m.in.size > 0 {
		return errors.E("MultiQueue.Close", errors.K.Invalid,
			"reason", "still has active inputs",
			"num", m.in.size)
	}

	m.closed = true

	// unblock Pop()
	m.cond.Signal()

	return nil
}
