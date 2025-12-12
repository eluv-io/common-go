package syncutil

import (
	"encoding/json"
	"sync"

	"github.com/gammazero/deque"

	"github.com/eluv-io/errors-go"
)

// Future is a placeholder for a [value, error] pair that is potentially only
// available at a later point in time. Since Futures are mostly used to
// represent the result of an asynchronous function call, it actually wraps both
// a generic value and an associated error.
type Future interface {
	// Await waits for the future value to be available.
	Await()
	// Get blocks until the future [value, error] pair is available and returns it.
	Get() (interface{}, error)
	// Try tries to get the future [value, error] pair without blocking. Returns [true, value, erro] if successful,
	// [false, nil, nil] otherwise .
	Try() (bool, interface{}, error)
}

// Promise is a synchronisation facility that allows to decouple the execution
// of a computational process and the collection of its result.
//
//	promise := NewPromise()
//
//	go func() {
//		res, err := doSomething()
//		promise.Resolve(res, err)
//	}()
//
//	...
//
//	res, err := promise.Get()
type Promise interface {
	Future
	Resolve(data interface{}, err error)
}

func NewPromise() Promise {
	return &promise{
		c: make(chan *result, 1),
	}
}

func NewFutures() *Futures {
	return &Futures{}
}

func NewMarshaledFuture(f Future) Future {
	return &MarshaledFuture{f}
}

// -----------------------------------------------------------------------------

type promise struct {
	c chan *result
}

func (p *promise) Await() {
	_, _ = p.Get()
}

func (p *promise) Get() (interface{}, error) {
	res := <-p.c // wait for result
	p.c <- res   // send back into channel for additional Get() calls...
	return res.data, res.err
}

func (p *promise) Try() (bool, interface{}, error) {
	select {
	case res := <-p.c:
		p.c <- res // send back into channel for additional Get() calls...
		return true, res.data, res.err
	default:
		return false, nil, nil
	}
}

func (p *promise) Resolve(data interface{}, err error) {
	p.c <- &result{data, err}
}

type result struct {
	data interface{}
	err  error
}

// -----------------------------------------------------------------------------

// MarshaledFuture is a wrapper for a Future that retrieves the future value
// when being marshaled to JSON.
type MarshaledFuture struct {
	Future
}

func (m *MarshaledFuture) MarshalJSON() ([]byte, error) {
	data, err := m.Future.Get()
	if err != nil {
		return nil, errors.E("marshal.future", err)
	}
	return json.Marshal(data)
}

// -----------------------------------------------------------------------------

// Futures is a dynamic collection of Futures. Its main purpose is to accumulate
// instances of Future and provide methods for waiting on all futures or
// collecting all their values. This also works in a "hierarchical" setup, where
// a given promise spawns additional promises during their execution. So while
// waiting for the parent promise/future, the collection allows adding more
// futures that will be waited on as well.
type Futures struct {
	mutex sync.Mutex
	queue deque.Deque
}

func (s *Futures) Add(f Future) {
	s.mutex.Lock()
	defer s.mutex.Unlock()
	s.queue.PushBack(f)
}

// Await waits for all futures that have been added to this Futures collection
// or that will be added while waiting for other futures. If any of the futures
// represents an error, the error is returned immediately (without waiting for
// additional futures).
func (s *Futures) Await() error {
	done := false
	var f Future
	for i := 0; ; i++ {
		s.mutex.Lock()
		{
			if i >= s.queue.Len() {
				done = true
			} else {
				f = s.queue.At(i).(Future)
			}
		}
		s.mutex.Unlock()

		if done {
			return nil
		}

		_, err := f.Get()
		if err != nil {
			return err
		}
	}
}

// Futures waits for and returns all futures that have been added to this Futures
// collection or that will be added while waiting for other futures.
func (s *Futures) Futures() []Future {
	done := false
	var f Future
	var res []Future
	for i := 0; ; i++ {
		s.mutex.Lock()
		{
			if i >= s.queue.Len() {
				done = true
			} else {
				f = s.queue.At(i).(Future)
			}
		}
		s.mutex.Unlock()

		if done {
			return res
		}

		f.Await()
		res = append(res, f)
	}
}
