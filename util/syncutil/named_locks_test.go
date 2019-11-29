package syncutil

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	log2 "github.com/qluvio/content-fabric/log"
)

// log with relative timestamps for easier visualization
var lg = func() *log2.Log {
	config := log2.NewConfig()
	config.Handler = "console"
	return log2.New(config)
}()

func TestNamedLocksBasic(t *testing.T) {
	nl := NamedLocks{}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	go func() {
		l1 := nl.Lock("l1")
		l2 := nl.Lock("l2")
		l3 := nl.Lock("l3")
		l3.Unlock()
		l1.Unlock()
		l2.Unlock()
		wg.Done()
	}()

	require.False(t, WaitTimeout(wg, time.Second))
	require.Empty(t, nl.named)
}

func TestNamedLocksSingle(t *testing.T) {
	nl := NamedLocks{}
	counter := atomic.NewInt64(0)
	wg := &sync.WaitGroup{}

	l := nl.Lock("l1")

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			pl := nl.Lock("l1")
			require.True(t, l == pl)
			counter.Add(1)
			pl.Unlock()
			wg.Done()
		}()
	}

	require.Equal(t, int64(0), counter.Load())
	l.Unlock()

	require.False(t, WaitTimeout(wg, time.Second))
	require.Equal(t, int64(10), counter.Load())
	require.Empty(t, nl.named)
}

// Creates 10 lock names and corresponding atomic counters and 4 workers. Each
// worker starts with a start offset 0-3, acquires the corresponding lock,
// sleeps 10 millis, increments the counter, releases the lock. Then continues
// with the next lock until all 10 locks have been acquired and released.
//
//   0.011       worker               count=1 lock=l00 start=0
//   0.013       worker               count=1 lock=l01 start=1
//   0.016       worker               count=1 lock=l02 start=2
//   0.017       worker               count=1 lock=l03 start=3
//   0.024       worker               count=2 lock=l01 start=0
//   0.028       worker               count=2 lock=l03 start=2
//   0.028       worker               count=1 lock=l04 start=3
//   0.028       worker               count=2 lock=l02 start=1
//   ...
//   0.220       worker               count=3 lock=l00 start=2
//   0.220       worker               count=3 lock=l01 start=3
//   0.226       worker               count=4 lock=l18 start=0
//   0.226       worker               count=3 lock=l19 start=1
//   0.232       worker               count=4 lock=l02 start=3
//   0.232       worker               count=4 lock=l01 start=2
//   0.237       worker               count=4 lock=l19 start=0
//   0.237       worker               count=4 lock=l00 start=1
func TestNamedLocksConcurrent(t *testing.T) {
	nl := NamedLocks{}
	wg := &sync.WaitGroup{}

	numWorkers := 4
	numAcquire := 20

	names := make([]string, numAcquire)
	counters := make([]*atomic.Int64, numAcquire)
	for i := 0; i < numAcquire; i++ {
		names[i] = fmt.Sprintf("l%.2d", i)
		counters[i] = atomic.NewInt64(0)
	}

	work := func(start int) {
		for i := start; i < numAcquire+start; i++ {
			idx := i % numAcquire
			l := nl.Lock(names[idx])
			c := counters[idx].Add(1)
			lg.Info("worker", "start", start, "lock", names[idx], "count", c)
			time.Sleep(10 * time.Millisecond)
			l.Unlock()
		}
		wg.Done()
	}

	wg.Add(numWorkers)
	for i := 0; i < numWorkers; i++ {
		go work(i)
		time.Sleep(2 * time.Millisecond)
	}

	require.False(t, WaitTimeout(wg, time.Second))
	for i := 0; i < numAcquire; i++ {
		require.Equal(t, int64(4), counters[i].Load())
	}
	require.Empty(t, nl.named)
}
