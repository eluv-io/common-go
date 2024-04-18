package atomicutil

import "sync/atomic"

type Error struct {
	p atomic.Pointer[error]
}

func (x *Error) Set(err error) {
	x.p.Store(&err)
}

func (x *Error) Get() error {
	ret := x.p.Load()
	if ret == nil {
		return nil
	}
	return *ret
}
