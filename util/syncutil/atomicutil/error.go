package atomicutil

import "sync/atomic"

type atomError struct {
	err error
}

type Error struct {
	p atomic.Pointer[atomError]
}

func (x *Error) Set(err error) {
	x.p.Store(&atomError{err: err})
}

func (x *Error) Get() error {
	ret := x.p.Load()
	if ret == nil {
		return nil
	}
	return ret.err
}
