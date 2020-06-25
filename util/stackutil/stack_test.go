package stackutil_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/qluvio/content-fabric/util/stackutil"
)

// TestLargeStack creates many goroutines with a deep stack and then generates
// a full stack trace that is expected to be larger than the default limit of
// 1 MB. It then asserts that FullStack created a larger buffer in order to
// accommodate the full stack trace.
func TestLargeStack(t *testing.T) {
	goroutineCount := 100
	sig := make(chan bool, goroutineCount)
	stop := make(chan bool)
	for i := 0; i < goroutineCount; i++ {
		go functionA(100, sig, stop, 0)
	}

	// wait for goroutines to build up their stack traces...
	for i := 0; i < goroutineCount; i++ {
		_ = <-sig
	}

	// create the stacktrace
	stack := stackutil.FullStack()
	require.Less(t, 1024*1024, len(stack))

	// stop goroutines
	close(stop)

	// wait for all goroutines to stop
	for i := 0; i < goroutineCount; i++ {
		_ = <-sig
	}
}

func functionA(stackDepth int, sig chan<- bool, stop <-chan bool, count int) {
	if count < stackDepth {
		functionB(stackDepth, sig, stop, count+1)
		return
	}
	sig <- true
	_ = <-stop
	sig <- true
}

func functionB(stackDepth int, sig chan<- bool, stop <-chan bool, count int) {
	functionC(stackDepth, sig, stop, count)
}

func functionC(stackDepth int, sig chan<- bool, stop <-chan bool, count int) {
	functionA(stackDepth, sig, stop, count)
}

func doSomething() string {
	return stackutil.Caller(1)
}

func TestCaller(t *testing.T) {
	s := doSomething()
	//fmt.Println(s)
	//stackutil_test.TestCaller (stack_test.go:66)
	require.True(t, strings.HasPrefix(s, "stackutil_test.TestCaller"))
}

// BenchmarkCaller-8   	 1093274	      1053 ns/op
func BenchmarkCaller(b *testing.B) {
	for i := 0; i < b.N; i++ {
		stackutil.Caller(1)
	}
}
