package testutil

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

// NewBaseTest creates a base test object that can be embedded in other tests.
// It embeds itself *testing.T and *require.Assertions, so they can be used from
// the test object directly.
//
// In addition, it offers a Run function for subtests similar to
// testing.T.Run() that handles the embedded *testing.T instance accordingly.
// See Run() for details.
func NewBaseTest(t *testing.T) *BaseTest {
	return &BaseTest{
		T:          t,
		Assertions: require.New(t),
	}
}

type BaseTest struct {
	*testing.T
	*require.Assertions
	mutex sync.Mutex
}

// Run runs a subtest with the given name similar to testing.T.Run(). In
// addition, it replaces the embedded *testing.T instance for the
// duration of the subtest with the subtest's instance. As a consequence,
// though, parallel tests are disabled.
func (b *BaseTest) Run(name string, f func()) bool {
	return b.T.Run(name, func(t *testing.T) {
		b.mutex.Lock()
		parent := b.T
		b.T = t
		b.mutex.Unlock()

		defer func() {
			b.mutex.Lock()
			b.T = parent
			b.mutex.Unlock()
		}()

		f()
	})
}

func (b *BaseTest) Parallel() {
	panic("parallel test not supported - use testing.T.Run() for parallel subtests")
}
