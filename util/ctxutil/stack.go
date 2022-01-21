package ctxutil

import (
	"context"
	"runtime/debug"
	"sync"

	elog "github.com/eluv-io/log-go"
	"go.opentelemetry.io/otel/api/trace"

	"github.com/qluvio/content-fabric/util/goutil"
)

var log = elog.Get("/eluvio/util/ctxutil")

// NewStack creates a new ContextStack.
func NewStack() ContextStack {
	return &contextStack{stacks: map[int64]*entry{}}
}

// ContextStack provides access to a stack of context.Context values individually managed per goroutine. It is
// essentially a thead-local implementation that offers an alternative to passing context object in each call along a
// call chain.
//
// The "standard" way of using contexts in go is passing it to each function in a call chain:
// 	type anObject struct {}
//
// 	func (r *anObject) A(ctx context.Context) {
// 		ctx = context.WithValue(context.Background(), "key", "val")
// 		r.B(ctx)
// 		...
// 	}
//
// 	func (r *anObject) B(ctx context.Context) {
// 		...
// 		r.C(ctx)
// 		...
// 	}
//
// 	func (r *anObject) C(ctx context.Context) string {
// 		val := ctx.Value("key")
// 		...
// 	}
//
// ContextStack achieves the same without adding a context to each method call:
//
// 	type anObject struct {
// 		cs *ctxutil.ContextStack
// 	}
//
// 	func (r *anObject) A() string {
// 		release := r.cs.WithValue("key", "val")
// 		defer release()
// 		r.B()
// 		...
// 	}
//
// 	func (r *anObject) B() string {
// 		...
// 		r.C()
// 		...
// 	}
//
// 	func (r *anObject) C() string {
// 		val := r.Ctx().Value("key")
// 		...
// 	}
type ContextStack interface {
	// Ctx retrieves the current context for the current goroutine.
	Ctx() context.Context

	// Push pushes the given context to the top of the stack for the current goroutine and makes it the current context.
	Push(ctx context.Context) func()

	// WithValue creates a new context with the provided key value pair and the current context of this goroutine as
	// parent.
	//
	// Usage:
	// 	release := r.cs.WithValue("key", "val")
	//	defer release()
	WithValue(key interface{}, val interface{}) func()

	// Go starts the given function in a new goroutine after pushing the calling goroutine's current context onto the
	// context stack of the new goroutine.
	Go(fn func())

	// InitTracing initializes tracing for the current goroutine with a root span created through the given tracer.
	InitTracing(tracer trace.Tracer, spanName string, opts ...trace.StartOption) trace.Span

	// StartSpan starts a new span and pushes its context onto the stack of the current goroutine. The span pops the
	// context upon calling span.End().
	//
	// Usage:
	// 	span := r.cs.StartSpan("my span")
	//	defer span.End()
	StartSpan(
		spanName string,
		opts ...trace.StartOption) trace.Span
	// Span retrieves the goroutine's current span.
	Span() trace.Span
}

////////////////////////////////////////////////////////////////////////////////

type contextStack struct {
	stacks map[int64]*entry
	mutex  sync.Mutex // guards access to stacks map
}

func (c *contextStack) Ctx() context.Context {
	e := c.entry(false)
	if e == nil {
		return context.Background()
	}
	return e.stack.ctx
}

func (c *contextStack) Push(ctx context.Context) func() {
	se := c.entry(true)
	// modification of stack entry requires no locking, since the entry is unique for each goroutine.
	s := &stack{
		ctx:    ctx,
		parent: se.stack,
	}
	se.stack = s
	return func() {
		c.pop(s)
	}
}

// pop removes the context at the top of the stack for the current goroutine and removes the stack altogether if it
// becomes empty.
func (c *contextStack) pop(expect *stack) {
	gid := goutil.GoID()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	e, ok := c.stacks[gid]
	if !ok {
		log.Warn("ContextStack: release called on empty stack!", "~at", "\n"+string(debug.Stack()))
		return
	}
	invalidCount := 0
	s := e.stack
	for expect != s {
		invalidCount++
		s = s.parent
		if s == nil {
			log.Warn("ContextStack: released stack not found!", "remaining", invalidCount, "~at", "\n"+string(debug.Stack()))
			return
		}
	}
	if invalidCount > 0 {
		log.Warn("ContextStack: missing release calls detected!", "missing", invalidCount, "~at", "\n"+string(debug.Stack()))
	}
	if s.parent == nil {
		delete(c.stacks, gid)
	}
	e.stack = s.parent
}

func (c *contextStack) Go(fn func()) {
	parent := c.Ctx()
	go func() {
		defer c.Push(parent)()
		fn()
	}()

}

func (c *contextStack) InitTracing(tracer trace.Tracer, spanName string, opts ...trace.StartOption) trace.Span {
	ctx := c.Ctx()
	ctx, sp := tracer.Start(ctx, spanName, opts...)
	release := c.Push(ctx)
	return &span{
		gid:     goutil.GoID(),
		Span:    sp,
		release: release,
	}
}

func (c *contextStack) StartSpan(
	spanName string,
	opts ...trace.StartOption) trace.Span {

	ctx := c.Ctx()
	parentSpan := trace.SpanFromContext(ctx)
	if !parentSpan.IsRecording() {
		// fast path if tracing is disabled: no need to start a (noop) span and push its dummy ctx onto the stack...
		return trace.NoopSpan{}
	}
	ctx, sp := parentSpan.Tracer().Start(ctx, spanName, opts...)
	release := c.Push(ctx)
	return &span{
		gid:     goutil.GoID(),
		Span:    sp,
		release: release,
	}
}

func (c *contextStack) Span() trace.Span {
	return trace.SpanFromContext(c.Ctx())
}

func (c *contextStack) WithValue(key interface{}, val interface{}) func() {
	return c.Push(context.WithValue(c.Ctx(), key, val))
}

// entry returns the entry for the current goroutine. Creates an new entry if necessary and "create" is true.
func (c *contextStack) entry(create bool) *entry {
	gid := goutil.GoID()

	c.mutex.Lock()
	defer c.mutex.Unlock()

	e, ok := c.stacks[gid]
	if !ok && create {
		e = &entry{}
		c.stacks[gid] = e
	}
	return e
}

////////////////////////////////////////////////////////////////////////////////

// entry is the struct stored in the stacks map - one per goroutine.
type entry struct {
	stack *stack
}

////////////////////////////////////////////////////////////////////////////////

// stack is the stack of contexts (for a given goroutine). It is not directly stored in the stacks map, so that it can
// be modified without modifying the map (which would require locking...)
type stack struct {
	ctx    context.Context
	parent *stack
}

////////////////////////////////////////////////////////////////////////////////

// span is a light wrapper around a trace.Span that pops the context in the End() call.
type span struct {
	trace.Span
	gid     int64
	release func()
	ended   bool
}

// End implements trace.Span.End()
func (s *span) End(options ...trace.EndOption) {
	gid := goutil.GoID()
	if s.gid != gid {
		log.Warn(
			"ContextStack: span.End() called from different goroutine! Ignoring call.",
			"creating_gid", s.gid,
			"ending_gid", gid,
			"~at",
			"\n"+string(debug.Stack()),
		)
		return
	}

	// End() is called from a single goroutine, so no need to protect s.ended...
	if s.ended {
		return
	}
	s.ended = true
	s.Span.End(options...)
	s.release()
}
