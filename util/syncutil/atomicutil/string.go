package atomicutil

import (
	"sync/atomic"
)

type String struct {
	atomic.Pointer[string]
}

func (x *String) Get() string {
	s := x.Load()
	if s == nil {
		return ""
	}
	return *s
}

func (x *String) Set(val string) {
	x.Store(&val)
}
