package ctxutil_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/apex/log/handlers/memory"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/api/trace"

	"github.com/qluvio/content-fabric/log"
	"github.com/qluvio/content-fabric/util/ctxutil"
	"github.com/qluvio/content-fabric/util/jsonutil"
	"github.com/qluvio/content-fabric/util/traceutil"
)

func TestContextStack(t *testing.T) {
	obj := anObject{cs: ctxutil.NewStack()}
	require.Equal(t, "ABBDBBA", obj.A())
}

func TestContextStackTracing(t *testing.T) {
	var trace *traceutil.TraceInfo
	stack := ctxutil.NewStack()

	span := stack.InitTracing(
		traceutil.NewTracer("test-tracer", func(trcInfo *traceutil.TraceInfo) {
			fmt.Println(jsonutil.MustPretty(trcInfo.MinimalString()))
			trace = trcInfo
		}),
		"test-span")

	obj := anObjectWithTracing{cs: stack}
	obj.A()

	obj.SpawnAndWait()
	obj.SpawnAndWait2()

	span.End()

	require.Len(t, trace.Spans, 13)
	require.Contains(t, trace.MinimalString(), `"current-func":"C"`)
}

func TestContextStackTracingDisabled(t *testing.T) {
	stack := ctxutil.NewStack()

	span := stack.StartSpan("test-span")
	obj := anObjectWithTracing{cs: stack}
	obj.A()

	obj.SpawnAndWait()
	obj.SpawnAndWait2()

	span.End()

	require.Equal(t, trace.NoopSpan{}, span)
}

func TestContextStackReleaseReordering(t *testing.T) {
	log.SetDefault(&log.Config{
		Level:   "warn",
		Handler: "memory",
	})
	logger := log.Get("/util/ctxutil")
	handler := logger.Handler().(*memory.Handler)

	cs := ctxutil.NewStack()

	release1 := cs.Push(context.Background())
	release2 := cs.Push(context.Background())
	release3 := cs.Push(context.Background())

	handler.Entries = nil // clear previous entries
	release2()
	require.Equal(t, "ContextStack: missing release calls detected!", handler.Entries[0].Message)
	require.Equal(t, 1, handler.Entries[0].Fields["missing"])

	handler.Entries = nil // clear previous entries
	release3()
	require.Equal(t, "ContextStack: released stack not found!", handler.Entries[0].Message)
	require.Equal(t, 1, handler.Entries[0].Fields["remaining"])

	handler.Entries = nil // clear previous entries
	release1()
	require.Empty(t, handler.Entries)

	handler.Entries = nil // clear previous entries
	release1()
	require.Equal(t, "ContextStack: release called on empty stack!", handler.Entries[0].Message)

}

////////////////////////////////////////////////////////////////////////////////

type anObject struct {
	cs ctxutil.ContextStack
}

func (r *anObject) A() string {
	defer r.cs.WithValue("func", "A")()
	// defer r.cs.Push(context.WithValue(r.cs.Ctx(), "func", "A"))()
	return r.get() + r.B() + r.get()
}

func (r *anObject) B() string {
	release := r.cs.WithValue("func", "B")
	defer release()
	return r.get() + r.C() + r.get()
}

func (r *anObject) C() string {
	return r.get() + r.D() + r.get()
}

func (r *anObject) D() string {
	defer r.cs.WithValue("func", "D")()
	return r.get()
}

func (r *anObject) get() string {
	return r.cs.Ctx().Value("func").(string)
}

////////////////////////////////////////////////////////////////////////////////

type anObjectWithTracing struct {
	cs ctxutil.ContextStack
}

func (r *anObjectWithTracing) A() {
	span := r.cs.StartSpan("A")
	defer span.End()
	r.B()
}

func (r *anObjectWithTracing) B() {
	span := r.cs.StartSpan("B")
	defer span.End()
	r.C()
}

func (r *anObjectWithTracing) C() {
	r.cs.Span().SetAttribute("current-func", "C")
	r.D()
}

func (r *anObjectWithTracing) D() {
	span := r.cs.StartSpan("D")
	defer span.End()
}

func (r *anObjectWithTracing) SpawnAndWait() {
	wg := sync.WaitGroup{}
	for i := 0; i < 3; i++ {
		wg.Add(1)
		parent := r.cs.Ctx()
		// r.cs.Go(func() {})
		go func() {
			// r.cs.Ctx()
			defer r.cs.Push(parent)()
			r.D()
			wg.Done()
		}()
	}
	wg.Wait()
}

func (r *anObjectWithTracing) SpawnAndWait2() {
	wg := sync.WaitGroup{}
	for i := 0; i < 3; i++ {
		wg.Add(1)
		r.cs.Go(func() {
			r.B()
			wg.Done()
		})
	}
	wg.Wait()
}
