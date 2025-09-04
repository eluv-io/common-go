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

	locker := BuildTraceLocker("locktest").Build()

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
	locker := BuildTraceLocker("example-lock").Build()
	useLocker(locker)

	// Output:
	//
	// {
	//   "name": "root",
	//   "time": "6s",
	//   "subs": [
	//     {
	//       "name": "example-lock",
	//       "time": "5s",
	//       "attr": {
	//         "caller": "traceutil.useLocker (tracelocker_test.go:98)"
	//       },
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

func ExampleTraceLockerBuilder() {
	locker := BuildTraceLocker("example-lock").WithCaller(false).WithSpanUnlock(false).Build()
	useLocker(locker)

	// Output:
	//
	// {
	//   "name": "root",
	//   "time": "6s",
	//   "subs": [
	//     {
	//       "name": "example-lock",
	//       "time": "0s",
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

func useLocker(locker *TraceLocker) {
	now := utc.UnixMilli(0)
	defer utc.MockNowFn(func() utc.UTC { return now })()

	rootSpan := InitTracing("root", false)

	locker.Lock()
	now = now.Add(5 * time.Second)
	locker.Unlock()

	now = now.Add(1 * time.Second)
	rootSpan.End()

	fmt.Println(jsonutil.MustPretty(rootSpan.Json()))
}
