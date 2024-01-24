package atomicutil

import "sync/atomic"

type Bytes struct {
	atomic.Pointer[[]byte]
}

func (x *Bytes) Get() []byte {
	s := x.Load()
	if s == nil {
		return nil
	}
	return *s
}

func (x *Bytes) Set(val []byte) {
	x.Store(&val)
}
