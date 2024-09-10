package atomicutil

import "sync/atomic"

type Error struct {
	atomic.Pointer[error]
}

func (x *Error) Get() error {
	val := x.Load()
	if val == nil {
		return nil
	}
	return *val
}

func (x *Error) Set(err error) {
	var val *error
	if err != nil {
		val = &err
	}
	x.Store(val)
}

// SetNX sets the given value if no value already set
func (x *Error) SetNX(err error) {
	var val *error
	if err != nil {
		val = &err
	}
	_ = x.CompareAndSwap(nil, val)
}
