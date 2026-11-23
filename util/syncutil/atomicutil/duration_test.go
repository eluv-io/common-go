package atomicutil

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestDuration(t *testing.T) {
	var d Duration
	require.Zero(t, d.Load())

	var wg sync.WaitGroup
	for i := 1; i <= 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.Store(time.Duration(i) * time.Second)
			_ = d.Load()
		}()
	}
	wg.Wait()
	require.Positive(t, d.Load())

	d.Store(time.Minute)
	require.Equal(t, time.Minute, d.Load())
}
