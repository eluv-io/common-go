package stackutil

import (
	"fmt"
	"path"
	"runtime"
	"strings"
	"sync"

	"github.com/eluv-io/log-go"
)

var full = struct {
	buf   []byte
	mutex sync.Mutex
}{
	buf: make([]byte, 10*1024*1024),
}

// FullStack creates a full dump of all the stack traces of all current
// goroutines.
func FullStack() (stack string) {
	full.mutex.Lock()
	defer full.mutex.Unlock()

	buf := full.buf
	n := runtime.Stack(buf, true)

	defer func() {
		// catch any out-of-memory panics which could happen while allocating
		// large(r) buffers below...
		if r := recover(); r != nil {
			log.Error("recovered panic in stackutil.FullStack", r)
			if len(buf) >= n {
				stack = string(buf[:n])
			}
		}
	}()

	for n == len(buf) {
		// try to allocate a large buffer
		buf = make([]byte, 10*len(buf))
		n = runtime.Stack(buf, true)
	}
	return string(buf[:n])
}

// Caller reports information on the caller at the given index in calling
// goroutine's stack. The argument index is the number of stack frames to ascend,
// with 0 identifying the caller of Caller.
// This function uses internally runtime.Caller
// The returned string contains the 'simple' name of the package and function
// followed by (file-name:line-number) of the caller.
// Example:
// file:     Users/xx/eluv.io/ws/src/content-fabric/util/stackutil/stack_test.go
// function: github.com/eluv-io/common-go/util/stackutil_test.TestCaller
// results:
// stackutil_test.TestCaller (stack_test.go:66)
func Caller(index int) string {
	simpleName := func(name string) string {
		if n := strings.LastIndex(name, "/"); n > 0 {
			name = name[n+1:]
		}
		return name
	}

	fname := "unknown"
	pc, file, line, ok := runtime.Caller(index + 1) // account for this call
	if !ok {
		file = "??"
		line = 1
	} else {
		file = simpleName(file)
	}
	f := runtime.FuncForPC(pc)
	if f != nil {
		fname = simpleName(f.Name())
	}
	return fmt.Sprintf("%s (%s:%d)", fname, file, line)
}

// Fn returns the caller's function name, e.g. pkg.Foo
func Fn() (fname string) {
	fname = "unknown"
	if pc, _, _, ok := runtime.Caller(1); ok {
		if f := runtime.FuncForPC(pc); f != nil {
			fname = path.Base(f.Name())
		}
	}
	return
}
