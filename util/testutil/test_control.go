package testutil

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// WaitUserTouch waits for user input by watching the file $TMPDIR/test-continue for a change of the last modified time.
// This is useful to halt unit tests, e.g. to execute manual curl commands against a test node. Due to the fact that
// 'go test' spawns a separate process with test binary, stdin cannot be used for this purpose. It might be possible to
// compile the test with 'go test -c' and then run the binary and use its stdin, but just simply "touching a file" is
// more convenient in any case.
func WaitUserTouch() {
	file := filepath.Join(os.TempDir(), "test-continue")

	fmt.Printf("####### 'touch %s' to continue #######\n", file)

	last := time.Time{}
	current := time.Time{}

	for {
		fileInfo, err := os.Stat(file)
		if err == nil {
			current = fileInfo.ModTime()
			if last.IsZero() {
				last = current
				continue
			}
		}
		if !last.Equal(current) {
			return
		}

		time.Sleep(time.Second)
		last = current
	}
}
