package timeutil

import (
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

type TestListener struct {
	tickCount atomic.Int64
}

func (tl *TestListener) Tick() {
	tl.tickCount.Add(1)
}

func TestTicker(t *testing.T) {
	ticker := NewTicker(50 * time.Millisecond)
	l1 := &TestListener{}
	l2 := &TestListener{}

	numGoroutines := runtime.NumGoroutine()

	ticker.Register(l1)
	ticker.Register(l2)

	time.Sleep(275 * time.Millisecond)

	require.Equal(t, numGoroutines+1, runtime.NumGoroutine()) // one goroutine for the ticker

	require.Equal(t, int64(5), l1.tickCount.Load())
	require.Equal(t, int64(5), l2.tickCount.Load())

	ticker.Unregister(l1)
	ticker.Unregister(l2)

	time.Sleep(10 * time.Millisecond)

	require.Equal(t, numGoroutines, runtime.NumGoroutine()) // ticker goroutine should stop
}

func TestManualTicker(t *testing.T) {
	ticker := NewManualTicker()
	l1 := &TestListener{}
	l2 := &TestListener{}

	numGoroutines := runtime.NumGoroutine()

	ticker.Register(l1)

	for i := 0; i < 10; i++ {
		if i == 5 {
			ticker.Register(l2)
		}
		ticker.Tick()
	}

	require.Equal(t, int64(10), l1.tickCount.Load())
	require.Equal(t, int64(5), l2.tickCount.Load())

	require.Equal(t, numGoroutines, runtime.NumGoroutine()) // no new goroutines should be created
}
