package syncutil

import "sync/atomic"

// AtomicBool implements a boolean value that is safe to be used from
// multiple go routines. It uses an atomic int underneath and ensures that read
// operations do not return stale values from thread-local caches.
// Based on atomicBool in net/http/server.go.
type AtomicBool int32

func (b *AtomicBool) IsTrue() bool  { return atomic.LoadInt32((*int32)(b)) != 0 }
func (b *AtomicBool) IsFalse() bool { return !b.IsTrue() }
func (b *AtomicBool) SetTrue()      { atomic.StoreInt32((*int32)(b), 1) }
func (b *AtomicBool) SetFalse()     { atomic.StoreInt32((*int32)(b), 0) }
