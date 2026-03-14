package rtp_test

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

// ticksMS converts a number of milliseconds to RTP ticks (90kHz clock).
func ticksMS(ms int) uint32 {
	return uint32(rtp.DurationToTicks(time.Duration(ms) * time.Millisecond))
}

// newTestPacerLogic creates a PacerLogic for testing. discardPeriod=0 means that discarding is disabled; pass a value >
// 0 to test the timed discard phase.
func newTestPacerLogic(discardPeriod, delay time.Duration, maxDiscardPeriod ...time.Duration) (*rtp.PacerLogic, *rtp.InStats) {
	stats := &rtp.InStats{}
	maxDiscard := discardPeriod * 2
	if len(maxDiscardPeriod) > 0 {
		maxDiscard = maxDiscardPeriod[0]
	}

	conf := rtp.PacerLogicConfig{
		Stream:           "test-stream",
		EventLog:         log.Get("/test/rtp/pacer"),
		DiscardPeriod:    duration.Spec(discardPeriod),
		MaxDiscardPeriod: duration.Spec(maxDiscard),
		Delay:            duration.Spec(delay),
		RtpSeqThreshold:  1,
		RtpTsThreshold:   duration.Spec(time.Second),
	}

	p := rtp.NewPacerLogic(conf, stats)
	return p, stats
}

// TestPacerLogic_DiscardPhase verifies that all packets are discarded during
// the discard period and that the first packet after the period is kept.
func TestPacerLogic_DiscardPhase(t *testing.T) {
	const discardPeriod = 100 * time.Millisecond
	const delay = 500 * time.Millisecond
	p, _ := newTestPacerLogic(discardPeriod, delay)

	// Wall-clock epoch: RTP timestamp 0 corresponds to this time.
	T0 := utc.UnixMilli(10_000)

	// Packet 0: first packet is always discarded (establishes discard baseline).
	_, discarded, _ := p.Packet(T0, 0, 0)
	require.True(t, discarded, "first packet must always be discarded")

	// Packets 1–9 arrive at 10ms intervals; elapsed < 100ms, still discarding.
	for i := 1; i <= 9; i++ {
		ts := ticksMS(i * 10)
		now := T0.Add(time.Duration(i) * 10 * time.Millisecond)
		_, discarded, _ = p.Packet(now, uint16(i), ts)
		require.True(t, discarded,
			"packet %d: elapsed=%s < %s, should still be discarded",
			i, now.Sub(T0), discardPeriod)
	}

	// Packet 10 at exactly 100ms: elapsed >= discardPeriod → discard phase ends.
	_, discarded, _ = p.Packet(T0.Add(100*time.Millisecond), 10, ticksMS(100))
	require.False(t, discarded, "packet at 100ms: elapsed >= discardPeriod, should not be discarded")
}

// TestPacerLogic_TimingBaseline verifies that the timing baseline is established
// on the first non-discarded packet: target = now + delay.
func TestPacerLogic_TimingBaseline(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, _ := newTestPacerLogic(0, delay)

	T0 := utc.UnixMilli(10_000)

	// Packet 1: discard period (0ms) has elapsed → baseline established.
	now1 := T0.Add(10 * time.Millisecond)
	ts1 := ticksMS(10)
	target, discarded, err := p.Packet(now1, 1, ts1)
	require.NoError(t, err)
	require.False(t, discarded)

	// The first non-discarded packet has rtpDelta=0, so target = baseTime = now + delay.
	assert.Equal(t, now1.Add(delay), target)
}

// TestPacerLogic_TargetTime verifies that subsequent packets compute
// targetTime = baseTime + TicksToDuration(ts - firstTs).
func TestPacerLogic_TargetTime(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, _ := newTestPacerLogic(0, delay)

	T0 := utc.UnixMilli(10_000)

	ts1 := ticksMS(10_000_000)
	target, discarded, err := p.Packet(T0, 1, ts1)
	require.NoError(t, err)
	require.False(t, discarded)
	require.Equal(t, T0.Add(delay), target)

	baseTime := T0.Add(delay)

	tests := []struct {
		seq    uint16
		ts     uint32        // absolute RTP timestamp
		nowOff time.Duration // wall-clock offset from T0
	}{
		{2, ts1 + ticksMS(10), 10 * time.Millisecond},
		{3, ts1 + ticksMS(20), 20 * time.Millisecond},
		{4, ts1 + ticksMS(100), 100 * time.Millisecond},
		{5, ts1 + ticksMS(500), 500 * time.Millisecond},
	}

	for _, tt := range tests {
		now := T0.Add(tt.nowOff)
		target, discarded, err = p.Packet(now, tt.seq, tt.ts)
		require.NoError(t, err)
		require.False(t, discarded, "seq=%d should not be discarded", tt.seq)

		rtpDelta := int64(tt.ts) - int64(ts1)
		wantTarget := baseTime.Add(rtp.TicksToDuration(rtpDelta))
		assert.Equal(t, wantTarget, target, "seq=%d: wrong target time", tt.seq)
	}
}

// TestPacerLogic_GapReset verifies that a detected RTP gap resets all state,
// restarts the discard phase, and clears statistics.
func TestPacerLogic_GapReset(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogic(15*time.Millisecond, delay)

	T0 := utc.UnixMilli(10_000)

	// Establish baseline and accumulate some stats.
	_, discarded, err := p.Packet(T0, 0, 0)
	require.NoError(t, err)
	require.True(t, discarded)

	_, discarded, err = p.Packet(T0.Add(10*time.Millisecond), 1, ticksMS(10))
	require.NoError(t, err)
	require.True(t, discarded)

	// discard period (15ms) has elapsed, so baseline established.
	_, discarded, err = p.Packet(T0.Add(20*time.Millisecond), 2, ticksMS(20))
	require.NoError(t, err)
	require.False(t, discarded)
	require.NotZero(t, stats.MinT0, "MinT0 should be set before gap")
	require.NotZero(t, stats.PushAhead.Min, "PushAhead.Min should be set before gap")

	// Inject a gap: sequence jumps from 2 → 100 (diff=98 > threshold=1).
	_, discarded, err = p.Packet(T0.Add(30*time.Millisecond), 100, ticksMS(30))
	assert.NoError(t, err)

	// The gap triggers a reset; the gap packet enters a new discard phase.
	assert.True(t, discarded, "first packet after gap should be discarded (new discard phase)")
	assert.Zero(t, stats.MinT0, "MinT0 should be reset after gap")
	assert.Zero(t, stats.PushAhead.Min, "PushAhead.Min should be reset after gap")
	assert.Zero(t, stats.PushAhead.Max, "PushAhead.Max should be reset after gap")

	// The next packet is still in the discard phase
	_, discarded, err = p.Packet(T0.Add(40*time.Millisecond), 101, ticksMS(40))
	assert.NoError(t, err)
	assert.True(t, discarded, "packet after post-gap discard should not be discarded")

	// The next packet completes the new discard phase (discardPeriod=15ms) and
	// establishes a fresh baseline.
	_, discarded, err = p.Packet(T0.Add(50*time.Millisecond), 102, ticksMS(50))
	assert.NoError(t, err)
	assert.False(t, discarded, "packet after post-gap discard should not be discarded")
	require.NotZero(t, stats.MinT0, "MinT0 should be set before gap")
	require.NotZero(t, stats.PushAhead.Min, "PushAhead.Min should be set before gap")
}

// TestPacerLogic_PushAheadStats verifies that PushAhead.Min and PushAhead.Max
// track the min and max lead time of packets relative to their target times.
//
// In a perfectly-synchronized stream (wall clock and RTP advance together)
// pushAhead equals the configured delay. Late arrivals shrink it; early
// arrivals grow it.
func TestPacerLogic_PushAheadStats(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogic(0, delay)

	T0 := utc.UnixMilli(10_000)

	// Establish baseline.
	p.Packet(T0, 0, 0)
	now1 := T0.Add(10 * time.Millisecond)
	ts1 := ticksMS(10)
	target1, _, _ := p.Packet(now1, 1, ts1)

	// Baseline packet: target = now1+delay, pushAhead = delay.
	assert.Equal(t, delay, target1.Sub(now1))
	assert.Equal(t, delay, stats.PushAhead.Min)
	assert.Equal(t, delay, stats.PushAhead.Max)

	// Late arrival: wall clock advances 20ms, RTP only 10ms.
	// target2 = baseTime+10ms = T0+520ms; now2 = T0+30ms → pushAhead2 = 490ms.
	now2 := now1.Add(20 * time.Millisecond)
	ts2 := ts1 + ticksMS(10)
	p.Packet(now2, 2, ts2)
	assert.Equal(t, delay-10*time.Millisecond, stats.PushAhead.Min, "late arrival shrinks PushAhead.Min")
	assert.Equal(t, delay, stats.PushAhead.Max, "PushAhead.Max unchanged")

	// Early arrival: wall clock advances 5ms, RTP advances 20ms.
	// target3 = baseTime+30ms = T0+540ms; now3 = T0+35ms → pushAhead3 = 505ms.
	now3 := now2.Add(5 * time.Millisecond)
	ts3 := ts2 + ticksMS(20)
	p.Packet(now3, 3, ts3)
	assert.Equal(t, delay-10*time.Millisecond, stats.PushAhead.Min, "PushAhead.Min unchanged")
	assert.Equal(t, delay+5*time.Millisecond, stats.PushAhead.Max, "early arrival grows PushAhead.Max")
}

// TestPacerLogic_T0Adjustments verifies that T0 decreases in the active phase
// are counted and accumulated in T0Adjustments and T0AdjustmentSumNs.
//
// T0 is the inferred wall-clock time when the RTP timestamp was 0:
//
//	t0 = now - TicksToDuration(ts)
//
// When a packet arrives slightly earlier than the wall clock expects (RTP
// timestamp is ahead of the wall clock), T0 appears to move backward. Each
// such decrease increments T0Adjustments.
func TestPacerLogic_T0Adjustments(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogic(0, delay)

	// wallEpoch is the wall-clock time corresponding to RTP timestamp 0.
	wallEpoch := utc.UnixMilli(10_000)

	// Establish baseline: packet 0 discarded, packet 1 sets baseline.
	p.Packet(wallEpoch, 0, 0)
	now1 := wallEpoch.Add(10 * time.Millisecond)
	ts1 := ticksMS(10)
	p.Packet(now1, 1, ts1)

	// t0 = now1 - TicksToDuration(ts1) = wallEpoch+10ms - 10ms = wallEpoch
	assert.Equal(t, wallEpoch, stats.MinT0)
	assert.Zero(t, stats.T0Adjustment.Count)

	// Packet 2: wall advances 15ms, RTP advances 20ms → packet arrives early.
	// t0 = (wallEpoch+25ms) - 30ms = wallEpoch-5ms (5ms earlier → adjustment).
	now2 := now1.Add(15 * time.Millisecond)
	ts2 := ts1 + ticksMS(20)
	p.Packet(now2, 2, ts2)
	assert.Equal(t, uint64(1), stats.T0Adjustment.Count)
	assert.Equal(t, 5*time.Millisecond, stats.T0Adjustment.Sum)
	assert.Equal(t, wallEpoch.Add(-5*time.Millisecond), stats.MinT0)

	// Packet 3: wall advances 10ms, RTP advances 15ms → another early arrival.
	// t0 = (wallEpoch+35ms) - 45ms = wallEpoch-10ms (5ms earlier → second adjustment).
	now3 := now2.Add(10 * time.Millisecond)
	ts3 := ts2 + ticksMS(15)
	p.Packet(now3, 3, ts3)
	assert.Equal(t, uint64(2), stats.T0Adjustment.Count)
	assert.Equal(t, 10*time.Millisecond, stats.T0Adjustment.Sum)
	assert.Equal(t, wallEpoch.Add(-10*time.Millisecond), stats.MinT0)

	// Packet 4: wall and RTP advance in sync → T0 stable, no adjustment.
	// t0 = (wallEpoch+45ms) - 55ms = wallEpoch-10ms (same as MinT0).
	now4 := now3.Add(10 * time.Millisecond)
	ts4 := ts3 + ticksMS(10)
	p.Packet(now4, 4, ts4)
	assert.Equal(t, uint64(2), stats.T0Adjustment.Count, "no adjustment when T0 is stable")
	assert.Equal(t, 10*time.Millisecond, stats.T0Adjustment.Sum)
}

// TestPacerLogic_StartupT0Adjustment verifies that T0 drift accumulated during
// the discard phase is captured in StartupT0Adjustment when the timing
// baseline is first established.
func TestPacerLogic_StartupT0Adjustment(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogic(5*time.Millisecond, delay, time.Minute)

	T0 := utc.UnixMilli(10_000)

	// Packet 0: ts=0, now=T0 → t0=T0 (discard baseline established).
	_, d0, err := p.Packet(T0, 0, 0)
	require.NoError(t, err)
	require.True(t, d0)

	// Packet 1: RTP advances 20ms while wall clock only advances 10ms.
	// t0 = (T0+10ms) - 20ms = T0-10ms → T0 moves 10ms earlier → adjustment tracked,
	// and the discard timer resets to now.
	_, d1, err := p.Packet(T0.Add(10*time.Millisecond), 1, ticksMS(20))
	require.NoError(t, err)
	require.True(t, d1, "T0 moved backward so discard timer resets, still discarding")

	// Packet 2: T0 stable at T0-10ms; discardPeriod=0 so elapsed (10ms) >= 0 → discard ends.
	// t0 = (T0+20ms) - 30ms = T0-10ms (stable).
	_, d2, err := p.Packet(T0.Add(20*time.Millisecond), 2, ticksMS(30))
	require.NoError(t, err)
	require.False(t, d2, "T0 stable and discardPeriod elapsed → baseline established")

	// The 10ms T0 shift from the discard phase must be captured.
	assert.Equal(t, 10*time.Millisecond, stats.StartupT0Adjustment.Sum)
}

// TestPacerLogic_SequenceWrapAround verifies that a uint16 sequence number
// wrap-around (65535 → 0) is handled transparently: no gap is detected and
// target times continue to be computed correctly.
func TestPacerLogic_SequenceWrapAround(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, _ := newTestPacerLogic(0, delay)

	T0 := utc.UnixMilli(10_000)

	// Establish baseline near the wrap boundary.
	// Packet 0: seq=65534; baseline
	_, _, err := p.Packet(T0, 65534, 0)
	require.NoError(t, err)

	// Packet 1 (baseline): seq=65535; diff=1, no gap.
	now1 := T0.Add(10 * time.Millisecond)
	ts1 := ticksMS(10)
	_, _, err = p.Packet(now1, 65535, ts1)
	require.NoError(t, err)
	baseTime := now1.Add(delay)

	// Packets 2–4: sequence wraps 65535 → 0 → 1 → 2.
	// The unwrapper computes diff = int16(0 - 65535) = int16(1) = 1 → no gap.
	packets := []struct {
		seq uint16
		ms  int // cumulative ms of RTP since ts1
	}{
		{0, 10}, // seq wraps here
		{1, 20},
		{2, 30},
	}
	for _, pkt := range packets {
		now := now1.Add(time.Duration(pkt.ms) * time.Millisecond)
		ts := ts1 + ticksMS(pkt.ms)
		target, discarded, err := p.Packet(now, pkt.seq, ts)
		require.NoError(t, err)
		require.False(t, discarded, "seq=%d should not be discarded after wrap", pkt.seq)
		wantTarget := baseTime.Add(rtp.TicksToDuration(int64(ticksMS(pkt.ms))))
		assert.Equal(t, wantTarget, target, "seq=%d: wrong target after sequence wrap", pkt.seq)
	}
}

// TestPacerLogic_TimestampLargeAccumulation verifies that the pacer continues
// to compute correct target times after multiple uint32 timestamp wrap-arounds,
// exercising the int64 unwrapped timestamp accumulation over ~26.5 hours of
// stream time (just over two full uint32 wrap-arounds at 90kHz).
//
// The TimestampUnwrapper computes diff = int32(new - old) in uint32 arithmetic,
// so a step of exactly 90000 ticks always produces diff = 90000 — no gap ever
// triggers. The unwrapped int64 accumulates correctly across each overflow.
func TestPacerLogic_TimestampLargeAccumulation(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, _ := newTestPacerLogic(0, delay)

	T0 := utc.UnixMilli(10_000)

	// Step size: 1 second = 90000 ticks, which equals the gap threshold so
	// abs(90000) > 90000 is false and no gap fires.
	const tsStep = uint32(90000)
	const stepDur = time.Second

	// Establish baseline.
	p.Packet(T0, 0, 0)
	now1 := T0.Add(stepDur)
	ts1 := tsStep
	p.Packet(now1, 1, ts1)
	baseTime := now1.Add(delay)

	// Run through just over 2 full uint32 wrap-arounds.
	// One wrap = 4294967296 ticks / 90000 ticks/s ≈ 47722 steps.
	// After 95500 steps the unwrapped int64 timestamp is ~8.6e9 ticks.
	// (MaxInt64 overflow would require ~3 million years; the test exercises
	// correctness through repeated uint32 wrap-arounds and large accumulated values.)
	const totalSteps = 95500

	var ts uint32 = ts1
	now := now1
	var seq uint16 = 2

	for step := 1; step <= totalSteps; step++ {
		now = now.Add(stepDur)
		ts += tsStep // overflows uint32 naturally; TimestampUnwrapper recovers int64
		target, discarded, _ := p.Packet(now, seq, ts)
		seq++ // overflows uint16 naturally; SequenceUnwrapper handles it

		require.False(t, discarded, "step %d: unexpected discard", step)

		// Spot-check target time just before and after each wrap boundary and at the end.
		if step == 47721 || step == 47722 || step == 95443 || step == 95444 || step == totalSteps {
			wantTarget := baseTime.Add(time.Duration(step) * stepDur)
			assert.Equal(t, wantTarget, target, "step %d: wrong target time", step)
		}
	}
}

// TestPacerLogic_TimestampWrapAround verifies that a uint32 RTP timestamp
// wrap-around (MaxUint32 → 0) is handled transparently: no gap is detected
// and target times continue to be computed correctly.
func TestPacerLogic_TimestampWrapAround(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, _ := newTestPacerLogic(0, delay)

	T0 := utc.UnixMilli(10_000)

	// Place the baseline timestamp exactly at MaxUint32 so the very next
	// 10ms step overflows to 899 (MaxUint32 + 900 mod 2^32).
	//   Packet 0 (discarded): ts = MaxUint32 - 900; primes the timestamp unwrapper.
	//   Packet 1 (baseline):  ts = MaxUint32; diff = 900, no gap.
	tsStep := ticksMS(10) // 900 ticks = 10ms
	p.Packet(T0, 0, uint32(math.MaxUint32)-tsStep)
	now1 := T0.Add(10 * time.Millisecond)
	ts1 := uint32(math.MaxUint32) // baseline sits at the wrap boundary
	p.Packet(now1, 1, ts1)
	baseTime := now1.Add(delay)

	// Packets 2–4: timestamp wraps MaxUint32 → 899 → 1799 → 2699.
	// The unwrapper computes diff = int32(899 - MaxUint32) = int32(900) = 900 → no gap.
	for i, seq := 1, uint16(2); i <= 3; i, seq = i+1, seq+1 {
		now := now1.Add(time.Duration(i*10) * time.Millisecond)
		ts := ts1 + uint32(i)*tsStep // wraps naturally in uint32 arithmetic
		target, discarded, _ := p.Packet(now, seq, ts)
		require.False(t, discarded, "seq=%d should not be discarded after ts wrap", seq)
		wantTarget := baseTime.Add(rtp.TicksToDuration(int64(i) * int64(tsStep)))
		assert.Equal(t, wantTarget, target, "seq=%d: wrong target after timestamp wrap", seq)
	}
}
