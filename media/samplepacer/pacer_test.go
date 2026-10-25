package samplepacer

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/errors-go"
)

// fakeClock drives the pacer deterministically: Now returns the fake time and Sleep jumps it to the requested instant,
// as long as that instant lies within the clock's horizon. A sleep beyond the horizon blocks until the pacer is woken
// or the horizon is extended, which is how a test holds the pacer in a waiting state and inspects it.
type fakeClock struct {
	mu        sync.Mutex
	now       time.Time
	horizon   time.Time
	horizonCh chan struct{} // closed and replaced whenever the horizon moves
}

func newFakeClock() *fakeClock {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	return &fakeClock{now: now, horizon: now.Add(1000 * time.Hour), horizonCh: make(chan struct{})}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(d)
	c.mu.Unlock()
}

// SetHorizon limits how far Sleep may jump the clock.
func (c *fakeClock) SetHorizon(t time.Time) {
	c.mu.Lock()
	c.horizon = t
	close(c.horizonCh)
	c.horizonCh = make(chan struct{})
	c.mu.Unlock()
}

// Sleep jumps the clock to t unless woken first or t lies beyond the horizon, in which case it blocks until woken,
// cancelled or the horizon moves. A real timer is not needed: the pacer loop re-evaluates after every sleep.
func (c *fakeClock) Sleep(ctx context.Context, t time.Time, wake <-chan struct{}) {
	for {
		select {
		case <-wake:
			return
		case <-ctx.Done():
			return
		default:
		}
		c.mu.Lock()
		if !t.After(c.horizon) {
			if t.After(c.now) {
				c.now = t
			}
			c.mu.Unlock()
			return
		}
		horizonCh := c.horizonCh
		c.mu.Unlock()
		select {
		case <-wake:
			return
		case <-ctx.Done():
			return
		case <-horizonCh:
		}
	}
}

// payload is what the tests carry through the pacer: an id, so a dropped item can be identified.
type payload int

type release struct {
	item Item[payload]
	at   time.Time
}

func runPacer(t *testing.T, p *Pacer[payload], clock *fakeClock, n int) []release {
	t.Helper()
	var mu sync.Mutex
	var got []release
	done := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		_ = p.Run(ctx, func(it Item[payload]) error {
			mu.Lock()
			got = append(got, release{item: it, at: clock.Now()})
			if len(got) == n {
				close(done)
			}
			mu.Unlock()
			return nil
		})
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for %d releases, got %d", n, len(got))
	}
	cancel()
	mu.Lock()
	defer mu.Unlock()
	return got
}

func TestPacer_ReleasesAtLatency(t *testing.T) {
	clock := newFakeClock()
	p := New(Config[payload]{Latency: 500 * time.Millisecond, Now: clock.Now, Sleep: clock.Sleep})
	start := clock.Now()
	ctx := context.Background()

	// a burst of one second of video at 25 fps and audio every 21.33 ms, pushed instantly
	for i := 0; i < 25; i++ {
		require.NoError(t, p.Push(ctx,
			Item[payload]{Track: Video, DTS: time.Duration(i) * 40 * time.Millisecond, Sync: i == 0}))
	}
	for i := 0; i < 47; i++ {
		require.NoError(t, p.Push(ctx, Item[payload]{Track: Audio, DTS: time.Duration(i) * 64 * time.Millisecond / 3}))
	}

	got := runPacer(t, p, clock, 72)
	require.Equal(t, Video, got[0].item.Track)
	require.Equal(t, start.Add(500*time.Millisecond), got[0].at, "the first item is released one latency after the anchor")

	last := got[0]
	for _, r := range got[1:] {
		require.GreaterOrEqual(t, r.item.DTS, last.item.DTS, "interleaved by DTS")
		require.Equal(t, start.Add(r.item.DTS+500*time.Millisecond), r.at, "released on schedule")
		last = r
	}
	s := p.Stats()
	require.EqualValues(t, 72, s.Delivered)
	require.Zero(t, s.Dropped)
}

func TestPacer_SkewHoldsVideoUntilAudioArrives(t *testing.T) {
	clock := newFakeClock()
	start := clock.Now()
	p := New(Config[payload]{Latency: 100 * time.Millisecond, MaxSkew: 200 * time.Millisecond,
		StallTimeout: 3 * time.Second,
		MaxBuffered:  10 * time.Second, Now: clock.Now, Sleep: clock.Sleep})
	ctx := context.Background()

	// video runs one second ahead of audio: only the first 200 ms of video may be released before audio shows up
	for i := 0; i < 25; i++ {
		require.NoError(t, p.Push(ctx,
			Item[payload]{Track: Video, DTS: time.Duration(i) * 40 * time.Millisecond, Sync: i == 0}))
	}
	require.NoError(t, p.Push(ctx, Item[payload]{Track: Audio, DTS: 0}))
	// the audio stall timeout must not expire on its own while the test inspects the held state
	clock.SetHorizon(start.Add(time.Second))

	var mu sync.Mutex
	var got []Item[payload]
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_ = p.Run(ctx2, func(it Item[payload]) error {
			mu.Lock()
			got = append(got, it)
			mu.Unlock()
			return nil
		})
	}()

	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) >= 7 // audio@0 + video up to 200 ms
	}, 2*time.Second, time.Millisecond)
	time.Sleep(20 * time.Millisecond)
	mu.Lock()
	n := len(got)
	mu.Unlock()
	require.Equal(t, 7, n, "video beyond MaxSkew past the last audio DTS waits")

	// audio catches up: the rest flows
	clock.SetHorizon(start.Add(time.Hour))
	for i := 1; i < 47; i++ {
		require.NoError(t, p.Push(ctx, Item[payload]{Track: Audio, DTS: time.Duration(i) * 64 * time.Millisecond / 3}))
	}
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) == 25+47
	}, 2*time.Second, time.Millisecond)
}

// TestPacer_FullQueueOverridesSkewGate covers the deadlock between the two backpressure mechanisms. One track runs
// ahead and fills the queue to its bound, which blocks both producers in Push. The head is then beyond MaxSkew of the
// track that is behind, and that track cannot push anything until something is released, so waiting for it only
// stalls everything until StallTimeout expires. The clock's horizon here forbids a jump of that size, so a pacer that
// waits never finishes the test.
func TestPacer_FullQueueOverridesSkewGate(t *testing.T) {
	clock := newFakeClock()
	start := clock.Now()
	p := New(Config[payload]{
		Latency:      0,
		MaxSkew:      200 * time.Millisecond,
		StallTimeout: time.Hour,
		MaxBuffered:  time.Second,
		Now:          clock.Now,
		Sleep:        clock.Sleep,
	})
	ctx := context.Background()

	const audioItems = 200
	// One video sample, then audio racing four seconds ahead of it.
	require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: 0, Sync: true}))
	clock.SetHorizon(start.Add(time.Minute)) // enough for the release schedule, far short of StallTimeout

	pushed := make(chan error, 1)
	go func() {
		for i := 1; i <= audioItems; i++ {
			it := Item[payload]{Track: Audio, DTS: time.Duration(i) * 20 * time.Millisecond, Payload: payload(i)}
			if err := p.Push(ctx, it); err != nil {
				pushed <- err
				return
			}
		}
		pushed <- nil
	}()

	var mu sync.Mutex
	released := 0
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_ = p.Run(ctx2, func(Item[payload]) error {
			mu.Lock()
			released++
			mu.Unlock()
			return nil
		})
	}()

	select {
	case err := <-pushed:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		require.Fail(t, "the producer never finished: the pacer held its queue at the bound instead of releasing")
	}
	require.Less(t, clock.Now().Sub(start), time.Minute, "no wait of StallTimeout length was taken")
	mu.Lock()
	defer mu.Unlock()
	require.Greater(t, released, audioItems/2, "releases kept pace with the producer")
}

func TestPacer_StalledTrackStopsGating(t *testing.T) {
	clock := newFakeClock()
	p := New(Config[payload]{Latency: 100 * time.Millisecond, MaxSkew: 100 * time.Millisecond, StallTimeout: time.Second,
		Now: clock.Now, Sleep: clock.Sleep})
	ctx := context.Background()
	for i := 0; i < 50; i++ {
		require.NoError(t, p.Push(ctx,
			Item[payload]{Track: Video, DTS: time.Duration(i) * 40 * time.Millisecond, Sync: i == 0}))
	}
	// audio never arrives: after StallTimeout the video goes out alone
	got := runPacer(t, p, clock, 50)
	require.Len(t, got, 50)
}

func TestPacer_LateBurstAndSkipToKeyframe(t *testing.T) {
	clock := newFakeClock()
	p := New(Config[payload]{Latency: 200 * time.Millisecond, LateTolerance: 200 * time.Millisecond,
		StallTimeout: time.Millisecond,
		MaxBuffered:  10 * time.Second, Now: clock.Now, Sleep: clock.Sleep})
	ctx := context.Background()

	// GOPs of 500 ms (keyframe every 5 frames at 10 fps), 4 seconds of video
	for i := 0; i < 40; i++ {
		require.NoError(t, p.Push(ctx,
			Item[payload]{Track: Video, DTS: time.Duration(i) * 100 * time.Millisecond, Sync: i%5 == 0}))
	}
	// let audio gating expire immediately
	clock.Advance(10 * time.Millisecond)

	var mu sync.Mutex
	var got []release
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_ = p.Run(ctx2, func(it Item[payload]) error {
			mu.Lock()
			got = append(got, release{item: it, at: clock.Now()})
			mu.Unlock()
			if len(got) == 1 {
				// the consumer stalls for 1.5 s after the first item: far more than the tolerance
				clock.Advance(1500 * time.Millisecond)
			}
			return nil
		})
	}()
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(got) >= 30
	}, 5*time.Second, time.Millisecond)
	cancel()

	mu.Lock()
	defer mu.Unlock()
	// After the stall the pacer must have skipped to the first keyframe at or beyond the backlog, dropping the
	// frames in between, and resumed on schedule from there.
	require.Equal(t, time.Duration(0), got[0].item.DTS)
	second := got[1].item
	require.True(t, second.Sync, "resumed on a keyframe")
	require.Equal(t, 500*time.Millisecond, second.DTS, "frames between were dropped")
	s := p.Stats()
	require.EqualValues(t, 1, s.Skips)
	require.EqualValues(t, 4, s.Dropped)
	// the keyframe is released one latency after the re-anchor, and everything after it follows that schedule
	require.Equal(t, got[0].at.Add(1500*time.Millisecond+200*time.Millisecond), got[1].at)
	anchor := got[1].at
	for _, r := range got[2:] {
		require.Equal(t, anchor.Add(r.item.DTS-second.DTS), r.at)
	}
}

func TestPacer_PushBlocksWhenFull(t *testing.T) {
	clock := newFakeClock()
	p := New(Config[payload]{Latency: 100 * time.Millisecond, MaxBuffered: time.Second,
		Now: clock.Now, Sleep: clock.Sleep})
	ctx := context.Background()
	for i := 0; i <= 10; i++ {
		require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: time.Duration(i) * 100 * time.Millisecond}))
	}
	pushed := make(chan error, 1)
	go func() {
		pushed <- p.Push(ctx, Item[payload]{Track: Video, DTS: 1100 * time.Millisecond})
	}()
	select {
	case err := <-pushed:
		t.Fatalf("push must block while the buffered span is at MaxBuffered, got %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	// cancelling the push context unblocks it
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, p.Push(cctx, Item[payload]{Track: Video, DTS: 1200 * time.Millisecond}), context.Canceled)

	// the consumer draining makes room, and the blocked push goes through
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() { _ = p.Run(runCtx, func(Item[payload]) error { return nil }) }()
	select {
	case err := <-pushed:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("the blocked push must go through once the consumer drains")
	}
}

func TestPacer_CloseDrainsAndRejectsPush(t *testing.T) {
	clock := newFakeClock()
	p := New(Config[payload]{Latency: 100 * time.Millisecond, StallTimeout: time.Millisecond,
		Now: clock.Now, Sleep: clock.Sleep})
	ctx := context.Background()
	require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: 0}))
	require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: 40 * time.Millisecond}))
	clock.Advance(10 * time.Millisecond)
	p.Close()
	require.ErrorIs(t, p.Push(ctx, Item[payload]{Track: Video, DTS: 80 * time.Millisecond}), ErrClosed)

	n := 0
	err := p.Run(ctx, func(Item[payload]) error { n++; return nil })
	require.NoError(t, err)
	require.Equal(t, 2, n, "queued items are drained before Run returns")
}

// dropRecorder collects the payloads the pacer hands back, which is where a pooled payload would be released.
type dropRecorder struct {
	mu      sync.Mutex
	dropped []payload
}

func (d *dropRecorder) onDrop(p payload) {
	d.mu.Lock()
	d.dropped = append(d.dropped, p)
	d.mu.Unlock()
}

func (d *dropRecorder) list() []payload {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]payload(nil), d.dropped...)
}

// TestPacer_DropsSkippedItems pins that the items a catch-up skip discards are handed back, one call per item.
func TestPacer_DropsSkippedItems(t *testing.T) {
	clock := newFakeClock()
	drops := &dropRecorder{}
	p := New(Config[payload]{Latency: 200 * time.Millisecond, LateTolerance: 200 * time.Millisecond,
		StallTimeout: time.Millisecond, MaxBuffered: 10 * time.Second, OnDrop: drops.onDrop,
		Now: clock.Now, Sleep: clock.Sleep})
	ctx := context.Background()
	for i := 0; i < 40; i++ {
		require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: time.Duration(i) * 100 * time.Millisecond,
			Sync: i%5 == 0, Payload: payload(i)}))
	}
	clock.Advance(10 * time.Millisecond)

	var mu sync.Mutex
	n := 0
	ctx2, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		_ = p.Run(ctx2, func(Item[payload]) error {
			mu.Lock()
			n++
			stall := n == 1
			mu.Unlock()
			if stall {
				clock.Advance(1500 * time.Millisecond)
			}
			return nil
		})
	}()
	require.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return n >= 30
	}, 5*time.Second, time.Millisecond)

	require.Equal(t, []payload{1, 2, 3, 4}, drops.list(), "the frames between the head and the next keyframe")
	require.EqualValues(t, 4, p.Stats().Dropped)
}

// TestPacer_RunExitDropsQueued pins that nothing is left without an owner when the consumer stops early, and that a
// later push is refused rather than accepted into a queue nobody reads.
func TestPacer_RunExitDropsQueued(t *testing.T) {
	clock := newFakeClock()
	drops := &dropRecorder{}
	p := New(Config[payload]{Latency: time.Hour, MaxBuffered: time.Hour, StallTimeout: time.Millisecond,
		OnDrop: drops.onDrop, Now: clock.Now, Sleep: clock.Sleep})
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: time.Duration(i) * 40 * time.Millisecond,
			Payload: payload(i)}))
	}

	// the consumer's context is cancelled with everything still held back by the one hour latency
	runCtx, cancel := context.WithCancel(ctx)
	cancel()
	require.ErrorIs(t, p.Run(runCtx, func(Item[payload]) error { return nil }), context.Canceled)

	require.ElementsMatch(t, []payload{0, 1, 2, 3, 4}, drops.list())
	require.ErrorIs(t, p.Push(ctx, Item[payload]{Track: Video, DTS: time.Second}), ErrClosed,
		"a pacer whose consumer has gone does not accept more items")
	require.Len(t, drops.list(), 5, "a refused push is not dropped: its caller still owns it")
}

// TestPacer_DeliverErrorDropsQueued covers the other early exit of Run.
func TestPacer_DeliverErrorDropsQueued(t *testing.T) {
	clock := newFakeClock()
	drops := &dropRecorder{}
	p := New(Config[payload]{Latency: 0, StallTimeout: time.Millisecond, OnDrop: drops.onDrop,
		Now: clock.Now, Sleep: clock.Sleep})
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: time.Duration(i) * 40 * time.Millisecond,
			Payload: payload(i)}))
	}
	clock.Advance(time.Second)

	boom := errors.Str("consumer failed")
	err := p.Run(ctx, func(Item[payload]) error { return boom })
	require.ErrorIs(t, err, boom)
	require.ElementsMatch(t, []payload{1, 2, 3}, drops.list(), "the item handed to deliver is the consumer's")
}

// TestPacer_DiscardDropsQueued covers the owner that never runs the pacer.
func TestPacer_DiscardDropsQueued(t *testing.T) {
	clock := newFakeClock()
	drops := &dropRecorder{}
	p := New(Config[payload]{Latency: time.Second, OnDrop: drops.onDrop, Now: clock.Now, Sleep: clock.Sleep})
	ctx := context.Background()
	require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: 0, Payload: 1}))
	require.NoError(t, p.Push(ctx, Item[payload]{Track: Audio, DTS: 0, Payload: 2}))

	p.Discard()
	require.ElementsMatch(t, []payload{1, 2}, drops.list())
	require.ErrorIs(t, p.Push(ctx, Item[payload]{Track: Video, DTS: time.Second}), ErrClosed)
	require.EqualValues(t, 2, p.Stats().Dropped)
}

// TestPacer_PushUnblocksOnClose pins that closing wakes every blocked producer, not just one.
func TestPacer_PushUnblocksOnClose(t *testing.T) {
	clock := newFakeClock()
	p := New(Config[payload]{Latency: time.Second, MaxBuffered: 100 * time.Millisecond, Now: clock.Now,
		Sleep: clock.Sleep})
	ctx := context.Background()
	require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: 0}))
	require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: time.Second}))

	pushed := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func() { pushed <- p.Push(ctx, Item[payload]{Track: Audio, DTS: 2 * time.Second}) }()
	}
	time.Sleep(20 * time.Millisecond)
	p.Close()
	for i := 0; i < 2; i++ {
		select {
		case err := <-pushed:
			require.ErrorIs(t, err, ErrClosed)
		case <-time.After(time.Second):
			t.Fatal("a blocked push must be woken by Close")
		}
	}
}

func TestPacer_DiscontinuityReanchors(t *testing.T) {
	clock := newFakeClock()
	p := New(Config[payload]{Latency: 100 * time.Millisecond, StallTimeout: time.Millisecond,
		Now: clock.Now, Sleep: clock.Sleep})
	ctx := context.Background()

	// one consumer for the whole test: a Run that returns closes the pacer, since nothing would consume afterwards
	var mu sync.Mutex
	var got []release
	runCtx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		_ = p.Run(runCtx, func(it Item[payload]) error {
			mu.Lock()
			got = append(got, release{item: it, at: clock.Now()})
			mu.Unlock()
			return nil
		})
	}()
	released := func(n int) []release {
		t.Helper()
		require.Eventually(t, func() bool {
			mu.Lock()
			defer mu.Unlock()
			return len(got) >= n
		}, 5*time.Second, time.Millisecond)
		mu.Lock()
		defer mu.Unlock()
		return append([]release(nil), got...)
	}

	require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: 10 * time.Second, Sync: true}))
	clock.Advance(10 * time.Millisecond)
	first := released(1)[0].at

	// the source restarts at time zero: without a discontinuity this would be judged 10 s early and wait
	p.Discontinuity()
	require.NoError(t, p.Push(ctx, Item[payload]{Track: Video, DTS: 0, Sync: true}))
	clock.Advance(10 * time.Millisecond)
	second := released(2)[1]
	require.Equal(t, first.Add(110*time.Millisecond), second.at, "re-anchored: released one latency after now")
}
