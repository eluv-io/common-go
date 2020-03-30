package stackutil

import (
	"runtime"
	"sync"

	"github.com/qluvio/content-fabric/log"
)

var full = struct {
	buf   []byte
	mutex sync.Mutex
}{
	buf: make([]byte, 1024*1024),
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
