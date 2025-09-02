package traceutil

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/utc-go"
)

func TestTraceLocker(t *testing.T) {
	rootSpan := InitTracing("test", false)

	locker := NewTraceLocker("locktest")

	go func() {
		// trace here is not recorded, since this goroutine is not initialized for tracing
		defer locker.LockUnlock()()
		time.Sleep(100 * time.Millisecond)
	}()

	time.Sleep(50 * time.Millisecond)
	locker.Lock() // should block for ~50ms
	locker.Unlock()

	span := rootSpan.FindByName("locktest")
	require.NotNil(t, span)

	require.NotEmpty(t, span.Events())
	require.Equal(t, "locked", span.Events()[0].Name)
	require.InDelta(t, 50*time.Millisecond, span.Duration(), float64(20*time.Millisecond))

	// wg.Wait()
	rootSpan.End()
	fmt.Println(rootSpan.Json())
}

func ExampleTraceLocker() {
	now := utc.UnixMilli(0)
	defer utc.MockNowFn(func() utc.UTC { return now })()

	rootSpan := InitTracing("test", false)

	locker := NewTraceLocker("example-lock")

	locker.Lock()
	now = now.Add(5 * time.Second)
	locker.Unlock()

	now = now.Add(1 * time.Second)
	rootSpan.End()

	fmt.Println(jsonutil.MustPretty(rootSpan.Json()))

	// Output:
	//
	// {
	//   "name": "test",
	//   "time": "6s",
	//   "subs": [
	//     {
	//       "name": "example-lock",
	//       "time": "5s",
	//       "evnt": [
	//         {
	//           "name": "locked",
	//           "at": "0s"
	//         }
	//       ]
	//     }
	//   ]
	// }
}
