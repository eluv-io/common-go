package pool

import (
	"sync/atomic"

	"github.com/eluv-io/errors-go"
)

func newResource[T any](r T, pool *Pool[T]) *Resource[T] {
	return &Resource[T]{
		T:    r,
		pool: pool,
	}
}

// Resource is a wrapper around a generic resource that tracks the number of references to it and releases it back to
// the pool when the last reference is dropped.
//
// Reference counting is only safe when callers follow the ownership contract: increment the count
// (Reference/ReferenceN) only while already holding a live reference, and call Release/ReleaseN exactly once per held
// reference. Referencing or releasing a resource whose count has already reached zero (i.e. one already returned to the
// pool) corrupts the pool — it may resurrect or free a resource another goroutine has since borrowed.
type Resource[T any] struct {
	T    T // The actual user-provided resource
	refs atomic.Int32
	pool *Pool[T]
}

// Reference increments the reference count by one. Call it before handing the resource to another consumer; that
// consumer must call Release when done. See Resource for the ownership contract.
func (p *Resource[T]) Reference() {
	p.refs.Add(1)
}

// ReferenceN increments the reference count by n (n must be >= 0). It is equivalent to calling Reference n times, e.g.
// to hand the resource to n additional consumers at once. Panics if n is negative. See Resource for the ownership
// contract.
func (p *Resource[T]) ReferenceN(n int) {
	if n < 0 {
		panic(errors.E("Pool.ReferenceN", errors.K.Invalid, "reason", "negative count", "count", n))
	}
	p.refs.Add(int32(n))
}

// Release decrements the reference count by one and returns the resource to the pool once the count reaches zero.
// Panics if the count drops below zero (which indicates a duplicate release). See Resource for the ownership contract.
func (p *Resource[T]) Release() {
	p.release(1)
}

// ReleaseN decrements the reference count by n (n must be >= 0) and returns the resource to the pool once the count
// reaches zero. Panics if n is negative or if the count drops below zero. See Resource for the ownership contract.
func (p *Resource[T]) ReleaseN(n int) {
	if n < 0 {
		panic(errors.E("Pool.ReleaseN", errors.K.Invalid, "reason", "negative count", "count", n))
	}
	p.release(int32(n))
}

func (p *Resource[T]) release(n int32) {
	refs := p.refs.Add(-n)
	if refs == 0 {
		p.pool.factory.Reset(p.T)
		p.pool.returned.Add(1)
		p.pool.pool.Put(p)
	} else if refs < 0 {
		// This is not thread-safe (another go-routine might already be retrieving the same resource from the pool and
		// be modifying the ref count). However, it's mainly used to detect programming errors (duplicate releases), so
		// this is acceptable.
		panic(errors.E("Pool.Release", errors.K.Invalid, "reason", "negative reference count!", "count", refs))
	}
}

func (p *Resource[T]) init() {
	p.refs.Store(1) // initialize ref count
	p.pool.factory.Init(p.T)
}
