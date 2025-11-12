package callback

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCallbackRegistry_BasicCallback(t *testing.T) {
	ctx := context.Background()
	registry := NewCallbackRegistry[int](ctx)
	defer registry.Stop()

	var received int
	var mu sync.Mutex

	handle := registry.Register(func(val int) {
		mu.Lock()
		received = val
		mu.Unlock()
	})

	registry.Notify(42)

	// give dispatcher time to process
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	require.Equal(t, 42, received)
	mu.Unlock()

	registry.Unregister(handle)
}

func TestCallbackRegistry_MultipleCallbacks(t *testing.T) {
	ctx := context.Background()
	registry := NewCallbackRegistry[string](ctx)
	defer registry.Stop()

	var received1, received2, received3 string
	var mu sync.Mutex

	registry.Register(func(val string) {
		mu.Lock()
		received1 = val
		mu.Unlock()
	})

	registry.Register(func(val string) {
		mu.Lock()
		received2 = val
		mu.Unlock()
	})

	registry.Register(func(val string) {
		mu.Lock()
		received3 = val
		mu.Unlock()
	})

	registry.Notify("test")

	// give dispatcher time to process
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	require.Equal(t, "test", received1)
	require.Equal(t, "test", received2)
	require.Equal(t, "test", received3)
}

func TestCallbackRegistry_CallbackOrder(t *testing.T) {
	ctx := context.Background()
	registry := NewCallbackRegistry[int](ctx)
	defer registry.Stop()

	var received []int
	var mu sync.Mutex

	for i := 0; i < 5; i++ {
		registry.Register(func(val int) {
			mu.Lock()
			received = append(received, val)
			mu.Unlock()
		})
	}

	// send multiple values
	for i := 1; i <= 3; i++ {
		registry.Notify(i)
	}

	// give dispatcher time to process
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()

	// should receive 3 values * 5 callbacks = 15 total
	require.Len(t, received, 15)

	// verify order: all callbacks for value 1, then all for 2, then all for 3
	for i := 0; i < 3; i++ {
		expectedVal := i + 1
		for j := 0; j < 5; j++ {
			idx := i*5 + j
			require.Equal(t, expectedVal, received[idx], "mismatch at index %d", idx)
		}
	}
}

func TestCallbackRegistry_StopViaMethod(t *testing.T) {
	t.Run("stop via method", func(t *testing.T) {
		testStop(t, context.Background(), func(r *Manager[int]) {
			r.Stop()
		})
	})

	t.Run("stop via context", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		testStop(t, ctx, func(r *Manager[int]) {
			cancel()
		})
	})
}

func testStop(t *testing.T, ctx context.Context, stop func(r *Manager[int])) {
	registry := NewCallbackRegistry[int](ctx)
	defer stop(registry)

	var count atomic.Int32

	registry.Register(func(val int) {
		count.Add(1)
	})

	// send some callbacks
	for i := 0; i < 5; i++ {
		registry.Notify(i)
	}

	// give dispatcher time to process
	time.Sleep(20 * time.Millisecond)

	// stop the registry with the provided stop function
	stop(registry)

	// give dispatcher time to shut down
	time.Sleep(20 * time.Millisecond)

	// try to send more callbacks after cancel - these should be dropped
	for i := 0; i < 5; i++ {
		registry.Notify(i)
	}

	// give some time for any potential processing
	time.Sleep(20 * time.Millisecond)

	// should only have processed the first 5
	require.Equal(t, int32(5), count.Load())
}

func TestCallbackRegistry_LongRunningCallbacks(t *testing.T) {
	ctx := context.Background()
	registry := NewCallbackRegistry[int](ctx)
	defer registry.Stop()

	var callbacksStarted, callbacksCompleted atomic.Int32
	processingTime := 50 * time.Millisecond

	// register a long-running callback
	registry.Register(func(val int) {
		callbacksStarted.Add(1)
		time.Sleep(processingTime)
		callbacksCompleted.Add(1)
	})

	// send multiple values quickly
	numValues := 5
	for i := 0; i < numValues; i++ {
		registry.Notify(i)
	}

	// wait a short time - should have started processing first callback
	time.Sleep(10 * time.Millisecond)

	require.GreaterOrEqual(t, callbacksStarted.Load(), int32(1))

	// all callbacks should be processed sequentially, not concurrently
	// total time should be at least numValues * processingTime
	minExpectedTime := time.Duration(numValues) * processingTime
	time.Sleep(minExpectedTime + 50*time.Millisecond)

	require.Equal(t, int32(numValues), callbacksCompleted.Load())
}

func TestCallbackRegistry_UnregisterDuringProcessing(t *testing.T) {
	ctx := context.Background()
	registry := NewCallbackRegistry[int](ctx)
	defer registry.Stop()

	var count1, count2 atomic.Int32

	handle1 := registry.Register(func(val int) {
		count1.Add(1)
		time.Sleep(10 * time.Millisecond)
	})

	handle2 := registry.Register(func(val int) {
		count2.Add(1)
		time.Sleep(10 * time.Millisecond)
	})

	// send some callbacks
	for i := 0; i < 3; i++ {
		registry.Notify(i)
	}

	// give dispatcher time to start processing
	time.Sleep(5 * time.Millisecond)

	// unregister first callback
	registry.Unregister(handle1)

	// send more callbacks
	for i := 0; i < 3; i++ {
		registry.Notify(i)
	}

	// give dispatcher time to process all
	time.Sleep(200 * time.Millisecond)

	// first callback should have processed first batch only
	require.Equal(t, int32(1), count1.Load())

	// second callback should have processed both batches
	require.Equal(t, int32(6), count2.Load())

	// Unregister second callback
	registry.Unregister(handle2)

}

func TestCallbackRegistry_ChannelBuffering(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping test in short mode")
	}

	ctx := context.Background()
	registry := NewCallbackRegistry[int](ctx)
	defer registry.Stop()

	var processed atomic.Int32

	// register a slow callback
	registry.Register(func(val int) {
		time.Sleep(20 * time.Millisecond)
		processed.Add(1)
	})

	// send more than buffer size callbacks quickly
	numCallbacks := 150
	for i := 0; i < numCallbacks; i++ {
		registry.Notify(i)
		// small delay to avoid blocking on channel send
		if i%10 == 0 {
			time.Sleep(time.Millisecond)
		}
	}

	// wait for all to be processed
	maxWaitTime := time.Duration(numCallbacks*25) * time.Millisecond
	time.Sleep(maxWaitTime)

	require.Equal(t, int32(numCallbacks), processed.Load())
}

func TestCallbackRegistry_ConcurrentRegistration(t *testing.T) {
	ctx := context.Background()
	registry := NewCallbackRegistry[int](ctx)
	defer registry.Stop()

	var wg sync.WaitGroup
	numGoroutines := 10
	var totalCallbacks atomic.Int32

	// concurrently register callbacks
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			registry.Register(func(val int) {
				totalCallbacks.Add(1)
			})
		}()
	}

	wg.Wait()

	// send a callback
	registry.Notify(42)

	// give dispatcher time to process
	time.Sleep(50 * time.Millisecond)

	// should have called all registered callbacks
	require.Equal(t, int32(numGoroutines), totalCallbacks.Load())
}

func TestCallbackRegistry_EmptyRegistry(t *testing.T) {
	ctx := context.Background()
	registry := NewCallbackRegistry[int](ctx)
	defer registry.Stop()

	// send callbacks when no callbacks are registered - should not panic
	require.NotPanics(t, func() {
		for i := 0; i < 5; i++ {
			registry.Notify(i)
		}
	})

	time.Sleep(10 * time.Millisecond)
}

func TestCallbackRegistry_HandleReuse(t *testing.T) {
	ctx := context.Background()
	registry := NewCallbackRegistry[int](ctx)
	defer registry.Stop()

	var count1, count2 atomic.Int32

	handle1 := registry.Register(func(val int) {
		count1.Add(1)
	})

	registry.Notify(1)
	time.Sleep(10 * time.Millisecond)

	registry.Unregister(handle1)

	// register a new callback - should get a different handle
	handle2 := registry.Register(func(val int) {
		count2.Add(1)
	})

	require.NotEqual(t, handle1, handle2, "expected different handles for subsequent registrations")

	registry.Notify(2)
	time.Sleep(10 * time.Millisecond)

	require.Equal(t, int32(1), count1.Load())
	require.Equal(t, int32(1), count2.Load())
}
