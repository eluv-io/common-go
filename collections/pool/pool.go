package pool

import (
	"sync"
	"sync/atomic"
)

// ResourceFactory creates and recycles the resources managed by a Pool. Init is called on every Borrow (including
// immediately after New for a freshly created resource), and Reset is called whenever a resource is returned to the
// pool. A factory must therefore ensure that both New+Init and Reset+Init leave the resource in a pristine state.
type ResourceFactory[T any] interface {
	// New creates a new resource. It is called when the pool has no resource available to satisfy a Borrow.
	New() T
	// Init is called after a resource is retrieved from the pool (on every Borrow) and must bring it to a pristine,
	// ready-to-use state.
	Init(T)
	// Reset is called before a resource is returned to the pool. Use it to clean up and release any references the
	// resource holds, so the idle resource does not pin memory while pooled.
	Reset(T)
}

// New creates a new resource pool.
func New[T any](factory ResourceFactory[T]) *Pool[T] {
	pool := &Pool[T]{
		factory: factory,
	}
	pool.pool.New = func() interface{} {
		pool.created.Add(1)
		return newResource[T](factory.New(), pool)
	}
	return pool
}

// Pool is a pool for generic resources that can be shared between multiple goroutines. To simplify usage, a resource is
// wrapped in a Resource struct that tracks the number of references to it. When the last reference is released, the
// resource is automatically returned to the pool.
//
// A resource's ref count is 1 when it is retrieved from the pool with Borrow. To share it with another goroutine,
// increment the count with Reference (or ReferenceN) before handing it over; the other goroutine then calls Release when
// finished. The resource returns to the pool once every reference has been released.
type Pool[T any] struct {
	factory ResourceFactory[T]
	pool    sync.Pool

	// usage counters (see Stats)
	created  atomic.Int64 // resources allocated via factory.New (i.e. pool misses)
	borrowed atomic.Int64 // calls to Borrow
	returned atomic.Int64 // resources returned to the pool when their last reference was released
}

// Borrow returns a Resource from the pool with its reference count set to 1. The wrapped resource has been brought to a
// pristine state by the factory's Init (a recycled resource is also cleaned up by Reset before reuse), but the factory
// determines whether any underlying storage is zeroed. Release the returned Resource when done.
func (p *Pool[T]) Borrow() *Resource[T] {
	res := p.pool.Get().(*Resource[T])
	p.borrowed.Add(1)
	res.init()
	return res
}

// Stats returns a snapshot of the pool's usage counters. The counters are read independently and are not captured
// atomically with respect to each other, so the snapshot may be slightly inconsistent under concurrent use.
func (p *Pool[T]) Stats() Stats {
	return Stats{
		Created:  p.created.Load(),
		Borrowed: p.borrowed.Load(),
		Returned: p.returned.Load(),
	}
}

// ---------------------------------------------------------------------------------------------------------------------

// Stats is a snapshot of a Pool's lifetime usage counters.
type Stats struct {
	Created  int64 `json:"created"`  // resources allocated via the factory (pool misses)
	Borrowed int64 `json:"borrowed"` // total Borrow calls
	Returned int64 `json:"returned"` // resources returned to the pool on final Release
}
