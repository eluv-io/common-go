package multiqueue

import (
	"sync"

	"github.com/gammazero/deque"
)

func newInput(cap int, notify *sync.Cond) *input {
	if cap < 1 {
		cap = 1
	}
	return &input{
		cond:   sync.NewCond(&sync.Mutex{}),
		notify: notify,
		deque:  deque.Deque{},
		cap:    cap,
	}
}

var _ Input = (*input)(nil)

type input struct {
	cond   *sync.Cond  // the condition for internal locking/signalling
	notify *sync.Cond  // the condition for signalling MultiQueue on Push and Close
	deque  deque.Deque // the input queue
	cap    int         // the input queue capacity
	closed bool        // true when input queue is closed
}

func (p *input) Close() {
	p.cond.L.Lock()
	{
		p.closed = true
		p.cond.Broadcast()
	}
	p.cond.L.Unlock()

	p.signalMultiQueue()
}

func (p *input) Push(val interface{}) {
	p.cond.L.Lock()
	{
		if p.closed {
			panic("input closed")
		}
		for p.deque.Len() == p.cap {
			p.cond.Wait()
		}
		p.deque.PushFront(val)
	}
	p.cond.L.Unlock()

	p.signalMultiQueue()
}

func (p *input) Pop() (val interface{}, ok bool, closed bool) {
	p.cond.L.Lock()
	defer p.cond.L.Unlock()

	if p.deque.Len() == 0 {
		if p.closed {
			return nil, false, true
		}
		return nil, false, false
	}

	val = p.deque.PopBack()
	p.cond.Signal()

	return val, true, false
}

func (p *input) signalMultiQueue() {
	p.notify.L.Lock()
	p.notify.Signal()
	p.notify.L.Unlock()
}
