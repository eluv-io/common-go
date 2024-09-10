package atomicutil

import "sync/atomic"

type Bytes struct {
	atomic.Pointer[[]byte]
}

func (x *Bytes) Get() []byte {
	val := x.Load()
	if val == nil {
		return nil
	}
	return *val
}

func (x *Bytes) Set(buf []byte) {
	var val *[]byte
	if buf != nil {
		val = &buf
	}
	x.Store(val)
}

// SetNX sets the given value if no value already set
func (x *Bytes) SetNX(buf []byte) {
	var val *[]byte
	if buf != nil {
		val = &buf
	}
	_ = x.CompareAndSwap(nil, val)
}
