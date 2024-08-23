package atomicutil

import "sync/atomic"

type String struct {
	atomic.Pointer[string]
}

func (x *String) Get() string {
	val := x.Load()
	if val == nil {
		return ""
	}
	return *val
}

func (x *String) Set(str string) {
	var val *string
	if str != "" {
		val = &str
	}
	x.Store(val)
}

// SetNX sets the given value if no value already set
func (x *String) SetNX(str string) {
	var val *string
	if str != "" {
		val = &str
	}
	_ = x.CompareAndSwap(nil, val)
}
