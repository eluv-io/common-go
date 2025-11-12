// Package callbacks provides functionality for managing callback registrations and dispatching
// in a concurrent-safe manner.
package callback

import (
	"context"
	"sync"

	"github.com/eluv-io/common-go/util/ifutil"
)

// Handle is a unique identifier for a registered callback and is used for unregistering.
type Handle int

// Function is the generic callback function.
type Function[T any] = func(T)

// Manager manages a collection of callback functions of type T, providing thread-safe registration,
// unregistration, and dispatching of callbacks. Callbacks are dispatched in a separate goroutine, using a buffered
// channel to avoid blocking the caller.
type Manager[T any] struct {
	rwmu      sync.RWMutex
	callbacks map[Handle]Function[T]
	handleSeq Handle
	valChan   chan T
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewCallbackRegistry creates a new Manager instance with the given context and optional channel size and
// starts the dispatcher goroutine. The default channel size is 100. The dispatcher goroutine may be stopped by
// canceling the provided context by calling the Stop() method.
func NewCallbackRegistry[T any](ctx context.Context, channelSize ...int) *Manager[T] {
	childCtx, cancel := context.WithCancel(ctx)
	c := &Manager[T]{
		callbacks: make(map[Handle]Function[T]),
		valChan:   make(chan T, ifutil.FirstOrDefault(channelSize, 100)),
		ctx:       childCtx,
		cancel:    cancel,
	}
	c.wg.Add(1)
	go c.dispatcher()
	return c
}

// Register adds a new callback to the registry and returns a unique handle that can be used to unregister the callback
// later.
func (c *Manager[T]) Register(callback Function[T]) (handle Handle) {
	c.rwmu.Lock()
	handle = c.handleSeq
	c.callbacks[handle] = callback
	c.handleSeq++
	c.rwmu.Unlock()
	return handle
}

// Unregister removes a callback from the registry using its handle.
func (c *Manager[T]) Unregister(handle Handle) {
	c.rwmu.Lock()
	delete(c.callbacks, handle)
	c.rwmu.Unlock()
}

// Notify sends a value to all registered callbacks through the dispatcher. Silently ignores values after the registry
// is stopped.
func (c *Manager[T]) Notify(val T) {
	select {
	case c.valChan <- val:
	case <-c.ctx.Done():
		// registry is stopped, ignore value
	}
}

// dispatcher runs in a separate goroutine, reads values from the channel, and calls all registered callbacks.
func (c *Manager[T]) dispatcher() {
	defer c.wg.Done()
	for {
		select {
		case val := <-c.valChan:
			c.rwmu.RLock()
			for _, cb := range c.callbacks {
				cb(val)
			}
			c.rwmu.RUnlock()
		case <-c.ctx.Done():
			return
		}
	}
}

// Stop stops the Manager and waits for the dispatcher to finish. Alternatively, the registry may be stopped by
// canceling the context provided upon creation.
func (c *Manager[T]) Stop() {
	c.cancel()
	c.wg.Wait()
}
