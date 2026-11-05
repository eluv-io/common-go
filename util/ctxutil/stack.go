package ctxutil

import (
	"context"
	"encoding/json"
	"runtime/debug"
	"sync"
	"unsafe"

	"github.com/eluv-io/common-go/util/goutil"
	"github.com/eluv-io/common-go/util/traceutil/trace"
)

// NewStack creates a new ContextStack.
func NewStack() ContextStack {
	return &contextStack{}
}

// ContextStack provides access to a stack of context.Context values individually managed per goroutine. It is
// essentially a thead-local implementation that offers an alternative to passing context object in each call along a
// call chain.
//
// The "standard" way of using contexts in go is passing it to each function in a call chain:
//
//	type anObject struct {}
//
//	func (r *anObject) A(ctx context.Context) {
//		ctx = context.WithValue(context.Background(), "key", "val")
//		r.B(ctx)
//		...
//	}
//
//	func (r *anObject) B(ctx context.Context) {
//		...
//		r.C(ctx)
//		...
//	}
//
//	func (r *anObject) C(ctx context.Context) string {
//		val := ctx.Value("key")
//		...
//	}
//
// ContextStack achieves the same without adding a context to each method call:
//
//	type anObject struct {
//		cs *ctxutil.ContextStack
//	}
//
//	func (r *anObject) A() string {
//		release := r.cs.WithValue("key", "val")
//		defer release()
//		r.B()
//		...
//	}
//
//	func (r *anObject) B() string {
//		...
//		r.C()
//		...
//	}
//
//	func (r *anObject) C() string {
//		val := r.Ctx().Value("key")
//		...
//	}
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
	// If slowOnly is true, the root span will only record operations that are explicitly started as slow spans.
	InitTracing(spanName string, slowOnly bool) trace.Span

	// StartSpan starts a new span and pushes its context onto the stack of the current goroutine. The span pops the
	// context upon calling span.End().
	//
	// Usage:
	// 	span := r.cs.StartSpan("my span")
	//	defer span.End()
	StartSpan(spanName string) trace.Span

	// SubSpan creates a new sub span of the given parent span and pushes its context onto the
	// stack. Defer-call the returned function to end the span and pop the context.
	SubSpan(parent trace.Span, spanName string) (release func())

	// Span retrieves the goroutine's current span.
	Span() trace.Span
}

////////////////////////////////////////////////////////////////////////////////

// stackShards is the number of shards of a contextStack, a power of two.
const stackShards = 64

// stackShardSize is the size of a shard, the cache line size of most CPUs.
const stackShardSize = 64

// contextStack keeps the stacks of its goroutines in shards, picked by goroutine ID. Each shard has its own mutex and
// map, so goroutines rarely contend on the same mutex. The shards take 4 KiB. Their maps are created on first use and
// add up to about 12 KiB once all are in use.
type contextStack struct {
	shards [stackShards]stackShard
}

// stackShard holds the stacks of the goroutines whose ID maps to it. It is padded to 64 bytes, so that the mutexes of
// neighbouring shards never share a 64-byte line.
type stackShard struct {
	stackShardFields
	// _ pads the shard to 64 bytes. The build fails if the fields outgrow that.
	_ [stackShardSize - unsafe.Sizeof(stackShardFields{})]byte
}

// stackShardFields are the fields of a stackShard, embedded so that its padding follows from their size.
type stackShardFields struct {
	// mu guards stacks and seq. The entries of the shard are modified under it, too.
	mu sync.Mutex
	// stacks maps a goroutine ID to the entry holding its stack. It is created on first use. An entry is removed when
	// its stack becomes empty.
	stacks map[int64]*entry
	// seq is the sequence number of the last frame pushed in this shard. The monotonic sequence is shared by all
	// goroutines of the shard, so a goroutine will likely not get contiguous numbers for its frames. A goroutine always
	// uses the same shard, so the numbers of its frames are unique, even across the entries it gets from the pool.
	//
	// A release finds its frame by this number. That detects a release of a frame that was already popped, such as a
	// second release, and releases that were skipped, whose frames lie above it. A release from another goroutine is
	// detected by the goroutine ID the release keeps along with the number.
	seq uint64
}

// shard returns the shard of the given goroutine. Goroutine IDs are sequential, so their low bits spread evenly over
// the shards without a hash.
func (c *contextStack) shard(gid int64) *stackShard {
	return &c.shards[uint64(gid)&(stackShards-1)]
}

// len returns the number of goroutines with a non-empty stack.
func (c *contextStack) len() int {
	var ret int
	for i := range c.shards {
		s := &c.shards[i]
		s.mu.Lock()
		ret += len(s.stacks)
		s.mu.Unlock()
	}
	return ret
}

func (c *contextStack) Ctx() context.Context {
	e := c.entry(goutil.GoID())
	if e == nil {
		return context.Background()
	}
	return e.ctx
}

func (c *contextStack) Push(ctx context.Context) func() {
	gid := goutil.GoID()
	sh := c.shard(gid)

	sh.mu.Lock()
	e, ok := sh.stacks[gid]
	if !ok {
		if sh.stacks == nil {
			sh.stacks = map[int64]*entry{}
		}
		e = entryPool.Get().(*entry)
		sh.stacks[gid] = e
	}
	sh.seq++
	seq := sh.seq
	e.frames = append(e.frames, frame{ctx: ctx, seq: seq})
	e.ctx = ctx
	sh.mu.Unlock()

	// The closure is the only allocation of a Push. It identifies its frame by goroutine and sequence number.
	return func() {
		c.pop(gid, seq)
	}
}

// pop removes the frame with the given sequence number and all frames above it from the stack of the current goroutine,
// which must be the goroutine that pushed it. It removes the stack altogether if it becomes empty.
func (c *contextStack) pop(pushGid int64, seq uint64) {
	gid := goutil.GoID()
	if gid != pushGid {
		log.Warn(
			"ContextStack: release called from different goroutine! Ignoring call.",
			"pushing_gid", pushGid,
			"releasing_gid", gid,
			"~at",
			"\n"+string(debug.Stack()),
		)
		return
	}
	sh := c.shard(gid)

	sh.mu.Lock()
	defer sh.mu.Unlock()

	e, ok := sh.stacks[gid]
	if !ok {
		log.Warn("ContextStack: release called on empty stack!", "~at", "\n"+string(debug.Stack()))
		return
	}
	i := len(e.frames) - 1
	for i >= 0 && e.frames[i].seq != seq {
		i--
	}
	if i < 0 {
		log.Warn("ContextStack: released stack not found!", "remaining", len(e.frames), "~at", "\n"+string(debug.Stack()))
		return
	}
	if missing := len(e.frames) - 1 - i; missing > 0 {
		log.Warn("ContextStack: missing release calls detected!", "missing", missing, "~at", "\n"+string(debug.Stack()))
	}
	clear(e.frames[i:]) // do not keep the popped contexts alive
	e.frames = e.frames[:i]
	if i == 0 {
		e.ctx = nil
		delete(sh.stacks, gid)
		entryPool.Put(e)
		return
	}
	e.ctx = e.frames[i-1].ctx
}

func (c *contextStack) Go(fn func()) {
	parent := c.Ctx()
	go func() {
		defer c.Push(parent)()
		fn()
	}()

}

func (c *contextStack) InitTracing(spanName string, slowOnly bool) trace.Span {
	ctx := c.Ctx()

	var sp trace.Span
	if slowOnly {
		_, sp = trace.StartSlowSpan(ctx, spanName) // ignore the returned context, we will bind the wrapped span below
	} else {
		_, sp = trace.StartRootSpan(ctx, spanName) // ignore the returned context, we will bind the wrapped span below
	}

	wrappedSpan := &span{
		gid:  goutil.GoID(),
		Span: sp,
	}
	// Bind the wrapped span, so that contextStack.Span() returns it instead of the plain span we wrap. This ensures
	// that c.Span().End() actually pops the context stack.
	ctx = trace.ContextWithSpan(ctx, wrappedSpan)
	wrappedSpan.release = c.Push(ctx)
	return wrappedSpan
}

func (c *contextStack) StartSpan(spanName string) trace.Span {
	ctx := c.Ctx()
	parentSpan := trace.SpanFromContext(ctx)
	if !parentSpan.IsRecording() {
		// fast path if tracing is disabled: no need to start a (noop) span and push its dummy ctx onto the stack...
		return trace.NoopSpan{}
	}
	_, sp := parentSpan.Start(ctx, spanName) // ignore the returned context, we will bind the wrapped span below
	wrappedSpan := &span{
		gid:  goutil.GoID(),
		Span: sp,
	}
	// Bind the wrapped span, so that contextStack.Span() returns it instead of the plain span we wrap. This ensures
	// that c.Span().End() actually pops the context stack.
	ctx = trace.ContextWithSpan(ctx, wrappedSpan)
	wrappedSpan.release = c.Push(ctx)
	return wrappedSpan
}

func (c *contextStack) SubSpan(parent trace.Span, spanName string) (release func()) {
	spCtx, sp := parent.Start(c.Ctx(), spanName)
	pop := c.Push(spCtx)
	return func() {
		sp.End()
		pop()
	}
}

func (c *contextStack) Span() trace.Span {
	return trace.SpanFromContext(c.Ctx())
}

func (c *contextStack) WithValue(key interface{}, val interface{}) func() {
	return c.Push(context.WithValue(c.Ctx(), key, val))
}

// entry returns the entry for the given goroutine, or nil if its stack is empty.
func (c *contextStack) entry(gid int64) *entry {
	sh := c.shard(gid)

	sh.mu.Lock()
	e := sh.stacks[gid]
	sh.mu.Unlock()
	return e
}

////////////////////////////////////////////////////////////////////////////////

// entryPool recycles the entries of emptied stacks, together with the capacity of their frames.
var entryPool = sync.Pool{New: func() any { return &entry{} }}

// entry is the stack of contexts of one goroutine, stored in the stacks map. It is removed from the map when its stack
// becomes empty. It is modified under the shard's mutex. Only its goroutine reads it without the mutex.
type entry struct {
	ctx    context.Context // the current context, the one of the last frame
	frames []frame         // the stack, with the current context last
}

// frame is one context on a stack.
type frame struct {
	ctx context.Context
	seq uint64
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
func (s *span) End() {
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
	s.Span.End()
	s.release()
}

func (s *span) MarshalJSON() ([]byte, error) {
	return json.Marshal(s.Span)
}
