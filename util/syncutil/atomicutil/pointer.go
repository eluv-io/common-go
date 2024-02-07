package atomicutil

import "sync/atomic"

func Pointer[T any](v *T) *atomic.Pointer[T] {
	ret := &atomic.Pointer[T]{}
	ret.Store(v)
	return ret
}
