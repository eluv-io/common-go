package ctxutil

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/traceutil/trace"
)

func TestCleanup(t *testing.T) {
	// make sure cleanup works with wrapped spans
	root := newLogSpan(Current().InitTracing("root"))

	wg := sync.WaitGroup{}
	for i := 0; i < 100; i++ {
		wg.Add(1)
		ctx := Current().Ctx()
		go func() {
			defer Current().Push(ctx)()
			span := Current().StartSpan("s1")
			span.Attribute("attr", "blub")

			span2 := Current().StartSpan("s2")
			span2.End()

			span.End()
			wg.Done()
		}()
	}

	wg.Wait()

	root.End()
	require.Empty(t, Current().(*contextStack).stacks)

	root.Log()
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
