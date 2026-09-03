package pacer

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/utc-go"
)

// pacedScheduler spaces successive packets by a fixed interval, the way a real stream's timestamps do, so a test
// controls the consumer's drain rate: the consumer sleeps until each packet's target before releasing its slot.
// Targets must increase per packet - if every packet were due at the same instant the consumer would release the whole
// batch at once, which is not how a paced stream behaves.
type pacedScheduler struct {
	stats    InStats
	interval time.Duration
	base     utc.UTC
	n        int64
}

func (s *pacedScheduler) Schedule(now utc.UTC, bts []byte) (utc.UTC, []byte, bool, error) {
	if s.base.IsZero() {
		s.base = now
	}
	target := s.base.Add(time.Duration(s.n) * s.interval)
	s.n++
	return target, bts, false, nil
}

func (s *pacedScheduler) InStats() *InStats { return &s.stats }

func (s *pacedScheduler) ResetSource() {
	s.base = utc.Zero
	s.n = 0
}

// newTestEngine builds an engine whose consumer is not yet running, returning it plus a func that starts the consumer
// and a func that stops the engine.
//
// The consumer is started separately so a test can fill the ring first. That ordering is the one that matters: a
// consumer already draining takes a small batch and frees those slots quickly, while a burst that fills the ring
// before the consumer wakes hands it the entire ring as one batch - which is the production case, and the only one
// where the producer waits for the whole buffer to play out.
func newTestEngine(t *testing.T, conf DisruptorEngineConfig, interval time.Duration) (e *DisruptorEngine, start, stop func()) {
	t.Helper()

	conf.StatsInterval = -1 // no periodic logging goroutine in tests
	e, err := NewDisruptorEngine(conf, &pacedScheduler{interval: interval})
	require.NoError(t, err)

	done := make(chan struct{})
	started := false
	start = func() {
		started = true
		go func() {
			defer close(done)
			_ = e.Run(func([]byte, utc.UTC) error { return nil })
		}()
	}
	stop = func() {
		e.Shutdown()
		if !started {
			return
		}
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("engine did not shut down")
		}
	}
	return e, start, stop
}

// fill pushes exactly enough packets to fill the ring buffer, so the next push has to wait for a slot.
func fill(t *testing.T, e *DisruptorEngine) {
	t.Helper()
	pkt := []byte("packet")
	for i := 0; i < e.BufferCap(); i++ {
		require.NoError(t, e.Push(pkt))
	}
}

// TestDisruptorEngine_FullBufferBlocksUntilDrained documents why MaxBlock exists. A full ring does not release one
// slot at a time: the consumer is handed the whole committed batch and publishes its position only after pacing
// through all of it, so the producer waits for the ring's entire contents to play out.
func TestDisruptorEngine_FullBufferBlocksUntilDrained(t *testing.T) {
	var conf DisruptorEngineConfig
	conf.InitDefaults()
	conf.BufferCapacity = 8
	conf.TickerPeriod = duration.Spec(2 * time.Millisecond)

	// 20ms per packet: draining all 8 slots takes ~140ms, far more than one slot's worth.
	e, startConsumer, stop := newTestEngine(t, conf, 20*time.Millisecond)
	defer stop()

	fill(t, e)
	startConsumer() // hands the consumer the whole ring as one batch

	start := time.Now()
	require.NoError(t, e.Push([]byte("overflow")))
	blocked := time.Since(start)

	// The wait is the whole buffer draining, not a single slot. Asserted loosely: the point is the order of
	// magnitude, and CI timing is not precise.
	require.Greater(t, blocked, 100*time.Millisecond, "expected to wait for the whole buffer to drain")

	require.Zero(t, e.Stats().Out.Dropped, "must not drop when MaxBlock is unset")
	require.Positive(t, e.Stats().Out.Blocked.Count, "the stall must be recorded")
}

// TestDisruptorEngine_MaxBlockDropsInsteadOfStalling checks that MaxBlock bounds the stall and drops the packet
// instead of letting the producer wait for the whole buffer to drain.
func TestDisruptorEngine_MaxBlockDropsInsteadOfStalling(t *testing.T) {
	var conf DisruptorEngineConfig
	conf.InitDefaults()
	conf.BufferCapacity = 8
	conf.TickerPeriod = duration.Spec(2 * time.Millisecond)
	conf.MaxBlock = duration.Spec(20 * time.Millisecond)

	// Same 20ms per packet as above, so without MaxBlock this would stall for ~140ms.
	e, startConsumer, stop := newTestEngine(t, conf, 20*time.Millisecond)
	defer stop()

	fill(t, e)
	startConsumer()

	start := time.Now()
	require.NoError(t, e.Push([]byte("overflow")), "a dropped packet is not an error")
	blocked := time.Since(start)

	require.Less(t, blocked, 100*time.Millisecond, "MaxBlock must bound the stall")

	stats := e.Stats().Out
	require.Equal(t, 1, stats.Dropped, "the overflowing packet must be counted as dropped")
	require.Positive(t, stats.Blocked.Count, "the stall must be recorded")
}

// TestDisruptorEngine_MaxBlockShorterThanPollInterval covers a MaxBlock smaller than the poll interval derived from
// TickerPeriod. The wait has to be clamped to what is left of the budget, or the very first sleep overshoots the cap
// by the whole interval.
func TestDisruptorEngine_MaxBlockShorterThanPollInterval(t *testing.T) {
	var conf DisruptorEngineConfig
	conf.InitDefaults()
	conf.BufferCapacity = 8
	conf.TickerPeriod = duration.Spec(time.Second) // poll interval would be 250ms
	conf.MaxBlock = duration.Spec(20 * time.Millisecond)

	// The consumer is never started, so no slot can ever free up and the push runs the budget down.
	e, _, stop := newTestEngine(t, conf, 10*time.Second)
	defer stop()

	fill(t, e)

	start := time.Now()
	require.NoError(t, e.Push([]byte("overflow")), "a dropped packet is not an error")
	blocked := time.Since(start)

	require.Less(t, blocked, 150*time.Millisecond, "the wait must be clamped to MaxBlock, not to the poll interval")
	require.Equal(t, 1, e.Stats().Out.Dropped)
}

// TestDisruptorEngine_NoStallWhenBufferHasRoom guards the fast path: pushes that fit must not touch the wait path, and
// must not record a stall.
func TestDisruptorEngine_NoStallWhenBufferHasRoom(t *testing.T) {
	var conf DisruptorEngineConfig
	conf.InitDefaults()
	conf.BufferCapacity = 64
	conf.MaxBlock = duration.Spec(time.Millisecond)

	e, startConsumer, stop := newTestEngine(t, conf, 0) // all due immediately
	defer stop()
	startConsumer()

	start := time.Now()
	for i := 0; i < 32; i++ {
		require.NoError(t, e.Push([]byte("packet")))
	}
	require.Less(t, time.Since(start), 50*time.Millisecond, "pushes that fit must not wait")

	stats := e.Stats().Out
	require.Zero(t, stats.Dropped)
	require.Zero(t, stats.Blocked.Count)
}

// TestDisruptorEngine_ShutdownReleasesBlockedPush ensures a producer waiting for a slot is released by Shutdown rather
// than hanging until the buffer drains.
func TestDisruptorEngine_ShutdownReleasesBlockedPush(t *testing.T) {
	var conf DisruptorEngineConfig
	conf.InitDefaults()
	conf.BufferCapacity = 8
	conf.TickerPeriod = duration.Spec(2 * time.Millisecond)

	// The consumer is never started, so the buffer never drains and the push waits until shutdown.
	e, _, stop := newTestEngine(t, conf, 10*time.Second)
	defer stop()

	fill(t, e)

	pushed := make(chan error, 1)
	go func() { pushed <- e.Push([]byte("overflow")) }()

	time.Sleep(20 * time.Millisecond) // let the push reach the wait path
	e.Shutdown()

	select {
	case err := <-pushed:
		require.NoError(t, err, "a push released by shutdown is not an error")
	case <-time.After(2 * time.Second):
		t.Fatal("blocked push was not released by shutdown")
	}
}
