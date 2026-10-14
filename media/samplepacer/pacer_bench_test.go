package samplepacer

import (
	"context"
	"testing"
	"time"
)

// Benchmarks of the pacer's per-sample cost. Inline error checks rather than require: require runs inside the timed
// loop and its reflection would dominate a measurement of tens of nanoseconds.
//
// The pacer had no benchmarks before this file, so there is no measured baseline to compare against. What it cost
// per sample was one context.AfterFunc registration in Push (a closure and a context node), one boxing of the item
// into `any` on the way into container/heap and another on the way out, and one time.Timer per wait in Run.
//
//	goos: linux
//	goarch: amd64
//	pkg: github.com/eluv-io/common-go/media/samplepacer
//	cpu: VirtualApple @ 2.50GHz
//	BenchmarkItemHeap_PushPop-4   28384264    40.16 ns/op   0 B/op   0 allocs/op
//	BenchmarkPacer_Push-4         18145071    65.82 ns/op   0 B/op   0 allocs/op
//	BenchmarkPacer_Cycle-4         1798581   650.0 ns/op    0 B/op   0 allocs/op
func BenchmarkItemHeap_PushPop(b *testing.B) {
	// video and audio interleaved, so the heap actually reorders rather than staying sorted
	h := make(itemHeap[int], 0, 64)
	var dts time.Duration
	b.ReportAllocs()
	for b.Loop() {
		h.push(Item[int]{Track: Video, DTS: dts, Sync: true})
		h.push(Item[int]{Track: Audio, DTS: dts + 7*time.Millisecond})
		h.pop()
		h.pop()
		dts += 40 * time.Millisecond
	}
}

// BenchmarkPacer_Push measures the producer side alone: a queue that never fills, so Push never waits.
//
// Nothing consumes here, so the queue is emptied every benchRound iterations with the timer stopped. Left to grow it
// would reach b.N items, which makes the run allocate and page proportionally to an adaptive iteration count and
// measures the heap growing rather than what a push costs.
func BenchmarkPacer_Push(b *testing.B) {
	p := New(Config[int]{Latency: time.Hour, MaxBuffered: 100 * time.Hour})
	ctx := context.Background()
	var dts time.Duration
	queued := 0
	b.ReportAllocs()
	for b.Loop() {
		if err := p.Push(ctx, Item[int]{Track: Video, DTS: dts}); err != nil {
			b.Fatal(err)
		}
		dts += 40 * time.Millisecond
		if queued++; queued == benchRound {
			b.StopTimer()
			p.emptyQueue()
			queued = 0
			b.StartTimer()
		}
	}
}

// benchRound is how many items the push benchmark queues before emptying. Large enough that the backing array reaches
// its size in the first round and every later round reuses it, small enough to keep the queue out of the measurement.
const benchRound = 4096

// emptyQueue drops everything queued without going through the consumer, for the benchmark above.
func (p *Pacer[P]) emptyQueue() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for len(p.queue) > 0 {
		p.queue.pop()
	}
}

// BenchmarkPacer_Cycle measures a full push and release cycle: the queue, the schedule and the hand-off to the
// consumer. It runs on the tests' fake clock, which jumps to each due time instead of waiting, so the numbers cover
// the pacer's own work plus that clock rather than real sleeping.
func BenchmarkPacer_Cycle(b *testing.B) {
	clock := newFakeClock()
	p := New(Config[int]{
		Latency: 0,
		// larger than the span the sentinel audio item below spreads the queue over, so Push never waits
		MaxBuffered:  100000 * time.Hour,
		StallTimeout: time.Nanosecond,
		Now:          clock.Now,
		Sleep:        clock.Sleep,
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// One audio item far in the future, so the video track is never held back waiting for audio: with a frozen clock
	// the stall timeout that would otherwise release it never expires. It sorts last and is never delivered.
	if err := p.Push(ctx, Item[int]{Track: Audio, DTS: 1000 * time.Hour}); err != nil {
		b.Fatal(err)
	}

	delivered := make(chan struct{})
	go func() {
		_ = p.Run(ctx, func(Item[int]) error {
			delivered <- struct{}{}
			return nil
		})
	}()

	var dts time.Duration
	b.ReportAllocs()
	for b.Loop() {
		if err := p.Push(ctx, Item[int]{Track: Video, DTS: dts, Sync: true}); err != nil {
			b.Fatal(err)
		}
		<-delivered
		dts += 40 * time.Millisecond
	}
	b.StopTimer()
	cancel()
}
