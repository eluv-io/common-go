package ctxutil

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/traceutil/trace"
)

func TestCleanup(t *testing.T) {
	// make sure cleanup works with wrapped spans
	root := newLogSpan(Current().InitTracing("root", false))

	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		ctx := Current().Ctx()
		go func() {
			defer wg.Done()
			defer Current().Push(ctx)()
			span := Current().StartSpan("s1")
			span.Attribute("attr", "blub")

			span2 := Current().StartSpan("s2")
			span2.End()

			span.End()
		}()
	}

	wg.Wait()

	root.End()
	require.Zero(t, Current().(*contextStack).len())

	root.Log()
}

func TestConcurrentStacks(t *testing.T) {
	type key struct{}
	cs := NewStack().(*contextStack)

	wg := sync.WaitGroup{}
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				release := cs.Push(context.WithValue(cs.Ctx(), key{}, j))
				inner := cs.WithValue(key{}, -j)
				// assert, since require must not fail the test from another goroutine
				assert.Equal(t, -j, cs.Ctx().Value(key{}))
				inner()
				assert.Equal(t, j, cs.Ctx().Value(key{}))
				release()
				assert.Nil(t, cs.Ctx().Value(key{}))
			}
		}()
	}
	wg.Wait()

	require.Zero(t, cs.len())
}

func newLogSpan(span trace.Span) *logSpan {
	return &logSpan{
		Span: span,
	}
}

type logSpan struct {
	trace.Span
	once sync.Once
}

func (s *logSpan) Log() {
	s.once.Do(func() {
		log.Info("collected trace", "trace", s.Span.Json())
		// would be more efficient if logging disabled:
		// log.Info("collected trace", "trace", jsonutil.Stringer(s.Span))
	})
}
