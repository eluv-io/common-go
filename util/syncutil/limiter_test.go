package syncutil_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/util/syncutil"
)

func TestConcurrencyLimiter(t *testing.T) {
	want := require.New(t)

	l := syncutil.NewConcurrencyLimiter(3)

	shouldAcquire := func(succeed bool) {
		wg := &sync.WaitGroup{}
		wg.Add(1)
		go func() {
			l.Acquire()
			wg.Done()
		}()
		timedOut := syncutil.WaitTimeout(wg, 100*time.Millisecond)
		want.Equal(!succeed, timedOut)
		if !succeed {
			// release the go routine that was expected to block
			l.Release()
		}
	}

	shouldAcquire(true)
	shouldAcquire(true)
	shouldAcquire(true)
	shouldAcquire(false)
	shouldAcquire(false)

	want.False(l.TryAcquire())
	want.False(l.TryAcquire())

	for i := 0; i < 10; i++ {
		l.Release()
		want.True(l.TryAcquire())
		want.False(l.TryAcquire())

		l.Release()
		shouldAcquire(true)
		want.False(l.TryAcquire())
	}
}
