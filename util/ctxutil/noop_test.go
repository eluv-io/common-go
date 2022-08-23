package ctxutil_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/util/ctxutil"
	"github.com/eluv-io/common-go/util/traceutil/trace"
)

func TestNoop(t *testing.T) {
	cs := ctxutil.Noop()
	defer cs.Push(context.TODO())()
	defer cs.WithValue("k1", "v1")()

	span := cs.StartSpan("blub")
	defer span.End()
	span.Attribute("k2", "v2")

	wg := &sync.WaitGroup{}
	wg.Add(1)
	cs.Go(func() {
		wg.Done()
	})
	wg.Wait()

	require.Equal(t, context.Background(), cs.Ctx())
	require.Equal(t, trace.NoopSpan{}, cs.Span())
}
