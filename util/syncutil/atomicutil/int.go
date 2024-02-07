package atomicutil

import "sync/atomic"

func Uint64(v uint64) *atomic.Uint64 {
	ret := &atomic.Uint64{}
	ret.Add(v)
	return ret
}

func Int64(v int64) *atomic.Int64 {
	ret := &atomic.Int64{}
	ret.Add(v)
	return ret
}
