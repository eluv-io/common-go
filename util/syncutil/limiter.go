package syncutil

import (
	"sync"
	"sync/atomic"
)

type ConcurrencyLimiter interface {
	// TryAcquire tries to acquire a permit without blocking. Returns true if successful, false otherwise. If true is
	// returned, Release has to be called subsequently to release the acquired permit.
	TryAcquire() bool
	// Acquire acquires a permit, potentially blocking until one becomes available.
	Acquire()
	// Release releases a previously acquired permit.
	Release()
	// Count returns the current number of acquired permits.
	Count() int
}

func NewConcurrencyLimiter(limit int) ConcurrencyLimiter {
	if limit <= 0 {
		return &noopLimiter{}
	}
	return &concurrencyLimiter{
		limit:   limit,
		permits: make(chan bool, limit),
	}
}

type concurrencyLimiter struct {
	limit   int
	mutex   sync.Mutex
	count   int
	permits chan bool
}

func (c *concurrencyLimiter) TryAcquire() bool {
	c.mutex.Lock()
	if c.count >= c.limit {
		c.mutex.Unlock()
		return false
	}

	c.count++
	c.mutex.Unlock()
	c.permits <- true

	return true
}

func (c *concurrencyLimiter) Acquire() {
	c.mutex.Lock()
	c.count++
	c.mutex.Unlock()
	c.permits <- true
}

func (c *concurrencyLimiter) Release() {
	<-c.permits
	c.mutex.Lock()
	c.count--
	c.mutex.Unlock()
}

func (c *concurrencyLimiter) Count() int {
	c.mutex.Lock()
	defer c.mutex.Unlock()
	return c.count
}

type noopLimiter struct {
	count atomic.Int32
}

func (n *noopLimiter) TryAcquire() bool {
	n.Acquire()
	return true
}

func (n *noopLimiter) Acquire() {
	_ = n.count.Add(1)
}

func (n *noopLimiter) Release() {
	_ = n.count.Add(-1)
}

func (n *noopLimiter) Count() int {
	return int(n.count.Load())
}
