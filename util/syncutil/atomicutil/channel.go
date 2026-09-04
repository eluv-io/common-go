package atomicutil

import "sync/atomic"

type Channel[T interface{}] struct {
	atomic.Pointer[chan T]
}

func (x *Channel[T]) Get() chan T {
	val := x.Load()
	if val == nil {
		return nil
	}
	return *val
}

func (x *Channel[T]) Set(buf chan T) {
	var val *chan T
	if buf != nil {
		val = &buf
	}
	x.Store(val)
}

// SetNX sets the given value if no value already set
func (x *Channel[T]) SetNX(buf chan T) {
	var val *chan T
	if buf != nil {
		val = &buf
	}
	_ = x.CompareAndSwap(nil, val)
}
