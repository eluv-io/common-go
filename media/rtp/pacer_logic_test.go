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

// TestPacerLogic_GapReset verifies that a detected RTP gap resets all per-session state, restarts the discard phase,
// increments StreamResets, and that StreamResets accumulates correctly across multiple gaps.
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
	require.NotZero(t, stats.MinT0, "MinT0 should be set after baseline")
	require.NotZero(t, stats.PushAhead.Min, "PushAhead.Min should be set after baseline")

	// Inject gap 1: sequence jumps from 2 → 100 (diff=98 > threshold=1).
	_, discarded, err = p.Packet(T0.Add(30*time.Millisecond), 100, ticksMS(30))
	require.NoError(t, err)

	// The gap triggers a reset; the gap packet enters a new discard phase.
	assert.True(t, discarded, "first packet after gap should be discarded (new discard phase)")
	assert.Zero(t, stats.MinT0, "MinT0 should be reset after gap")
	assert.Zero(t, stats.PushAhead.Min, "PushAhead.Min should be reset after gap")
	assert.Zero(t, stats.PushAhead.Max, "PushAhead.Max should be reset after gap")
	assert.Equal(t, 1, stats.StreamResets, "StreamResets should be 1 after first gap")
	assert.Equal(t, T0.Add(30*time.Millisecond), stats.LastStreamReset, "LastStreamReset should record the gap time")

	// Complete the new discard phase and re-establish a baseline.
	_, discarded, err = p.Packet(T0.Add(40*time.Millisecond), 101, ticksMS(40))
	require.NoError(t, err)
	require.True(t, discarded, "second packet in post-gap discard phase should still be discarded")

	_, discarded, err = p.Packet(T0.Add(50*time.Millisecond), 102, ticksMS(50))
	require.NoError(t, err)
	require.False(t, discarded, "packet after post-gap discard period should not be discarded")
	require.NotZero(t, stats.MinT0, "MinT0 should be set after second baseline")
	require.NotZero(t, stats.PushAhead.Min, "PushAhead.Min should be set after second baseline")

	// Inject gap 2: sequence jumps from 102 → 200. StreamResets must accumulate to 2, not reset to 1.
	_, discarded, err = p.Packet(T0.Add(60*time.Millisecond), 200, ticksMS(60))
	require.NoError(t, err)
	assert.True(t, discarded, "first packet after second gap should be discarded")
	assert.Equal(t, 2, stats.StreamResets, "StreamResets must accumulate across gaps, not reset to 1")
	assert.Equal(t, T0.Add(60*time.Millisecond), stats.LastStreamReset, "LastStreamReset should record the second gap time")
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
	assert.EqualValues(t, delay, stats.PushAhead.Min)
	assert.EqualValues(t, delay, stats.PushAhead.Max)

	// Late arrival: wall clock advances 20ms, RTP only 10ms.
	// target2 = baseTime+10ms = T0+520ms; now2 = T0+30ms → pushAhead2 = 490ms.
	now2 := now1.Add(20 * time.Millisecond)
	ts2 := ts1 + ticksMS(10)
	p.Packet(now2, 2, ts2)
	assert.EqualValues(t, delay-10*time.Millisecond, stats.PushAhead.Min, "late arrival shrinks PushAhead.Min")
	assert.EqualValues(t, delay, stats.PushAhead.Max, "PushAhead.Max unchanged")

	// Early arrival: wall clock advances 5ms, RTP advances 20ms.
	// target3 = baseTime+30ms = T0+540ms; now3 = T0+35ms → pushAhead3 = 505ms.
	now3 := now2.Add(5 * time.Millisecond)
	ts3 := ts2 + ticksMS(20)
	p.Packet(now3, 3, ts3)
	assert.EqualValues(t, delay-10*time.Millisecond, stats.PushAhead.Min, "PushAhead.Min unchanged")
	assert.EqualValues(t, delay+5*time.Millisecond, stats.PushAhead.Max, "early arrival grows PushAhead.Max")
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
	assert.Zero(t, stats.NegDrift.Count)

	// Packet 2: wall advances 15ms, RTP advances 20ms → packet arrives early.
	// t0 = (wallEpoch+25ms) - 30ms = wallEpoch-5ms (5ms earlier → adjustment).
	now2 := now1.Add(15 * time.Millisecond)
	ts2 := ts1 + ticksMS(20)
	p.Packet(now2, 2, ts2)
	assert.Equal(t, uint64(1), stats.NegDrift.Count)
	assert.EqualValues(t, 5*time.Millisecond, stats.NegDrift.Sum)
	assert.Equal(t, wallEpoch.Add(-5*time.Millisecond), stats.MinT0)

	// Packet 3: wall advances 10ms, RTP advances 15ms → another early arrival.
	// t0 = (wallEpoch+35ms) - 45ms = wallEpoch-10ms (5ms earlier → second adjustment).
	now3 := now2.Add(10 * time.Millisecond)
	ts3 := ts2 + ticksMS(15)
	p.Packet(now3, 3, ts3)
	assert.Equal(t, uint64(2), stats.NegDrift.Count)
	assert.EqualValues(t, 10*time.Millisecond, stats.NegDrift.Sum)
	assert.EqualValues(t, wallEpoch.Add(-10*time.Millisecond), stats.MinT0)

	// Packet 4: wall and RTP advance in sync → T0 stable, no adjustment.
	// t0 = (wallEpoch+45ms) - 55ms = wallEpoch-10ms (same as MinT0).
	now4 := now3.Add(10 * time.Millisecond)
	ts4 := ts3 + ticksMS(10)
	p.Packet(now4, 4, ts4)
	assert.EqualValues(t, uint64(2), stats.NegDrift.Count, "no adjustment when T0 is stable")
	assert.EqualValues(t, 10*time.Millisecond, stats.NegDrift.Sum)
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
	assert.EqualValues(t, 10*time.Millisecond, stats.StartupT0Correction.Sum)
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

// newTestPacerLogicFull creates a PacerLogic from a complete PacerLogicConfig.
func newTestPacerLogicFull(conf rtp.PacerLogicConfig) (*rtp.PacerLogic, *rtp.InStats) {
	stats := &rtp.InStats{}
	if conf.EventLog == nil {
		conf.EventLog = log.Get("/test/rtp/pacer")
	}
	return rtp.NewPacerLogic(conf, stats), stats
}

// TestPacerLogic_AdjustTimeDrift_Applied verifies that when AdjustTimeDrift=true (no cap), a T0 backward shift of X ms
// causes subsequent target times to be pulled X ms earlier compared to the unadjusted case.
func TestPacerLogic_AdjustTimeDrift_Applied(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogicFull(rtp.PacerLogicConfig{
		AdjustTimeDrift: true,
		Delay:           duration.Spec(delay),
		RtpSeqThreshold: 1,
		RtpTsThreshold:  duration.Spec(time.Second),
	})

	T0 := utc.UnixMilli(10_000)

	// Packet 1: establish baseline (DiscardPeriod=0 → not discarded, baseTime = T0 + delay)
	now1 := T0
	ts1 := ticksMS(0)
	_, discarded, err := p.Packet(now1, 1, ts1)
	require.NoError(t, err)
	require.False(t, discarded)
	baseTime := now1.Add(delay) // expected unadjusted baseTime (firstRtpTimestamp=0)

	// Packet 2: wall clock advances 15ms, RTP advances 20ms → T0 shifts 5ms earlier.
	// t0_2 = (T0+15ms) - 20ms = T0-5ms → adjustment = 5ms.
	now2 := now1.Add(15 * time.Millisecond)
	ts2 := ts1 + ticksMS(20)
	target2, discarded, err := p.Packet(now2, 2, ts2)
	require.NoError(t, err)
	require.False(t, discarded)
	require.Equal(t, uint64(1), stats.NegDrift.Count, "nominal adjustment must be recorded")
	require.EqualValues(t, 5*time.Millisecond, stats.NegDrift.Sum)
	require.EqualValues(t, uint64(1), stats.NegDriftApplied.Count, "applied adjustment must be recorded")
	require.EqualValues(t, 5*time.Millisecond, stats.NegDriftApplied.Sum, "full drift applied (no cap)")

	// target2 is adjusted immediately: unadjusted = baseTime + 20ms, adjusted = unadjusted - 5ms.
	rtpDelta2 := rtp.TicksToDuration(int64(ts2) - int64(ts1))
	wantUnadjusted := baseTime.Add(rtpDelta2)
	wantAdjusted := wantUnadjusted.Add(-5 * time.Millisecond)
	require.Equal(t, wantAdjusted, target2, "target must be 5ms earlier due to time-ref adjustment")
}

// TestPacerLogic_AdjustTimeDrift_Cap verifies that MaxNegDriftCorrection limits how much of the observed T0 drift is
// applied to baseTime in a single Packet call, while the full nominal drift is still recorded in T0Adjustment.
func TestPacerLogic_AdjustTimeDrift_Cap(t *testing.T) {
	const delay = 500 * time.Millisecond
	const capAdj = 3 * time.Millisecond
	p, stats := newTestPacerLogicFull(rtp.PacerLogicConfig{
		AdjustTimeDrift:       true,
		MaxNegDriftCorrection: duration.Spec(capAdj),
		Delay:                 duration.Spec(delay),
		RtpSeqThreshold:       1,
		RtpTsThreshold:        duration.Spec(time.Second),
	})

	T0 := utc.UnixMilli(10_000)

	// Packet 1: establish baseline (DiscardPeriod=0 → not discarded, baseTime = T0 + delay)
	now1 := T0
	ts1 := ticksMS(0)
	_, discarded, err := p.Packet(now1, 1, ts1)
	require.NoError(t, err)
	require.False(t, discarded)
	baseTime := now1.Add(delay)

	// Packet 2: T0 drifts back 10ms (large single-step drift, capped at 3ms).
	// wall +5ms, RTP +15ms → t0 = (T0+5ms) - 15ms = T0-10ms → nominal adj = 10ms.
	now2 := now1.Add(5 * time.Millisecond)
	ts2 := ts1 + ticksMS(15)
	target2, discarded, err := p.Packet(now2, 2, ts2)
	require.NoError(t, err)
	require.False(t, discarded)

	// Full nominal drift recorded.
	require.Equal(t, uint64(1), stats.NegDrift.Count)
	require.EqualValues(t, 10*time.Millisecond, stats.NegDrift.Sum, "full nominal drift must be recorded")

	// Only capped amount applied.
	require.Equal(t, uint64(1), stats.NegDriftApplied.Count)
	require.EqualValues(t, capAdj, stats.NegDriftApplied.Sum, "only capped amount must be applied")

	// Target time reflects only the 3ms correction (not the full 10ms).
	rtpDelta2 := rtp.TicksToDuration(int64(ts2) - int64(ts1))
	wantUnadjusted := baseTime.Add(rtpDelta2)
	wantAdjusted := wantUnadjusted.Add(-capAdj)
	require.Equal(t, wantAdjusted, target2, "target must reflect only the capped 3ms correction")
}

// TestPacerLogic_SlowDrift_Correction verifies that when the mean positive T0 drift over a period exceeds the
// threshold, a positive correction is applied to baseTime and recorded in stats.
//
// Setup: wall clock advances 10ms per packet, RTP advances ticksMS(8) (8ms) per packet → t0 increases by 2ms/packet.
// After 7 tracker updates (packets 1–7, drifts 0,2,4,6,8,10,12 ms), packet 8 at +70ms triggers the period
// transition: Previous.Mean = 42/7 = 6ms > 2ms threshold → 1ms correction applied.
func TestPacerLogic_SlowDrift_Correction(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogicFull(rtp.PacerLogicConfig{
		AdjustTimeDrift:    true,
		PosDriftPeriod:     duration.Spec(60 * time.Millisecond),
		PosDriftThreshold:  duration.Spec(2 * time.Millisecond),
		PosDriftCorrection: duration.Spec(time.Millisecond),
		Delay:              duration.Spec(delay),
		RtpSeqThreshold:    1,
		RtpTsThreshold:     duration.Spec(time.Second),
	})

	T0 := utc.UnixMilli(10_000)

	// Packet 1: establish baseline (DiscardPeriod=0 → DiscardComplete set immediately).
	now := T0
	ts := ticksMS(0)
	seq := uint16(1)
	_, discarded, err := p.Packet(now, seq, ts)
	require.NoError(t, err)
	require.False(t, discarded)
	baseTime := now.Add(delay)

	// Packets 2–7: t0 drifts up by 2ms per packet (wall +10ms, RTP +8ms each).
	for i := 2; i <= 7; i++ {
		now = now.Add(10 * time.Millisecond)
		ts += ticksMS(8)
		seq++
		_, discarded, err = p.Packet(now, seq, ts)
		require.NoError(t, err)
		require.False(t, discarded, "packet %d should not be discarded", i)
	}
	require.Zero(t, stats.PosDriftApplied.Count, "no correction yet: period not elapsed")

	// Packet 8 at +70ms: period ends (70ms > 60ms), mean=6ms > 2ms → correction applied.
	now = now.Add(10 * time.Millisecond)
	ts += ticksMS(8)
	seq++
	target8, discarded, err := p.Packet(now, seq, ts)
	require.NoError(t, err)
	require.False(t, discarded)

	require.Equal(t, uint64(1), stats.PosDrift.Count, "one period must be recorded")
	require.EqualValues(t, 6*time.Millisecond, stats.PosDrift.Sum, "mean drift for the period")
	require.Equal(t, uint64(1), stats.PosDriftApplied.Count, "one correction must be applied")
	require.EqualValues(t, time.Millisecond, stats.PosDriftApplied.Sum)

	// target8 must be 1ms later than the unadjusted value: baseTime + TicksToDuration(ts8 - ts1) + 1ms.
	rtpDelta8 := rtp.TicksToDuration(int64(ts) - int64(ticksMS(0)))
	wantUnadjusted := baseTime.Add(rtpDelta8)
	require.Equal(t, wantUnadjusted.Add(time.Millisecond), target8,
		"target must be 1ms later due to slow-drift correction")
}

// TestPacerLogic_SlowDrift_BelowThreshold verifies that no correction is applied when the mean positive drift
// is below the configured threshold.
func TestPacerLogic_SlowDrift_BelowThreshold(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogicFull(rtp.PacerLogicConfig{
		AdjustTimeDrift:    true,
		PosDriftPeriod:     duration.Spec(60 * time.Millisecond),
		PosDriftThreshold:  duration.Spec(10 * time.Millisecond), // high threshold: 6ms mean won't trigger
		PosDriftCorrection: duration.Spec(time.Millisecond),
		Delay:              duration.Spec(delay),
		RtpSeqThreshold:    1,
		RtpTsThreshold:     duration.Spec(time.Second),
	})

	T0 := utc.UnixMilli(10_000)

	now := T0
	ts := ticksMS(0)
	seq := uint16(1)
	_, discarded, err := p.Packet(now, seq, ts)
	require.NoError(t, err)
	require.False(t, discarded)

	for i := 2; i <= 8; i++ {
		now = now.Add(10 * time.Millisecond)
		ts += ticksMS(8)
		seq++
		_, discarded, err = p.Packet(now, seq, ts)
		require.NoError(t, err)
		require.False(t, discarded, "packet %d should not be discarded", i)
	}

	require.Zero(t, stats.PosDriftApplied.Count, "mean=6ms < threshold=10ms: no correction must be applied")
}

// TestPacerLogic_SlowDrift_StatsWithoutCorrection verifies that T0SlowDrift is recorded even when AdjustTimeDrift is
// false: drift is observed and logged, but no baseTime correction is applied.
func TestPacerLogic_SlowDrift_StatsWithoutCorrection(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogicFull(rtp.PacerLogicConfig{
		AdjustTimeDrift:    false, // corrections disabled
		PosDriftPeriod:     duration.Spec(60 * time.Millisecond),
		PosDriftThreshold:  duration.Spec(2 * time.Millisecond),
		PosDriftCorrection: duration.Spec(time.Millisecond),
		Delay:              duration.Spec(delay),
		RtpSeqThreshold:    1,
		RtpTsThreshold:     duration.Spec(time.Second),
	})

	T0 := utc.UnixMilli(10_000)

	// Same slow-stream pattern as TestPacerLogic_SlowDrift_Correction (mean=6ms > 2ms threshold).
	now := T0
	ts := ticksMS(0)
	seq := uint16(1)
	_, discarded, err := p.Packet(now, seq, ts)
	require.NoError(t, err)
	require.False(t, discarded)
	baseTime := now.Add(delay)

	for i := 2; i <= 7; i++ {
		now = now.Add(10 * time.Millisecond)
		ts += uint32(ticksMS(8))
		seq++
		_, discarded, err = p.Packet(now, seq, ts)
		require.NoError(t, err)
		require.False(t, discarded, "packet %d should not be discarded", i)
	}

	// Packet 8: period ends, mean=6ms > 2ms → drift recorded, but no correction applied.
	now = now.Add(10 * time.Millisecond)
	ts += ticksMS(8)
	seq++
	target8, discarded, err := p.Packet(now, seq, ts)
	require.NoError(t, err)
	require.False(t, discarded)

	require.Equal(t, uint64(1), stats.PosDrift.Count, "drift must be recorded even without AdjustTimeDrift")
	require.EqualValues(t, 6*time.Millisecond, stats.PosDrift.Sum)
	require.Zero(t, stats.PosDriftApplied.Count, "no correction must be applied when AdjustTimeDrift=false")

	// target8 must be unadjusted.
	rtpDelta8 := rtp.TicksToDuration(int64(ts) - int64(ticksMS(0)))
	wantUnadjusted := baseTime.Add(rtpDelta8)
	require.Equal(t, wantUnadjusted, target8, "target must be unadjusted when AdjustTimeDrift=false")
}

// TestPacerLogic_StartupJitter verifies that positive jitter on the first non-discarded packet does not offset
// the entire timeline. Because baseTime is anchored to discard.T0 rather than to `now`, a late-arriving first
// packet does not inflate the effective delay for all subsequent packets. With AdjustTimeDrift=false there is also
// no reactive correction: subsequent jitter-free packets simply have pushAhead == delay and no NegDrift events.
func TestPacerLogic_StartupJitter(t *testing.T) {
	const delay = 500 * time.Millisecond
	const discardPeriod = 15 * time.Millisecond
	p, stats := newTestPacerLogicFull(rtp.PacerLogicConfig{
		AdjustTimeDrift:  false, // corrections disabled; any offset would be permanent
		DiscardPeriod:    duration.Spec(discardPeriod),
		MaxDiscardPeriod: duration.Spec(time.Minute),
		Delay:            duration.Spec(delay),
		RtpSeqThreshold:  1,
		RtpTsThreshold:   duration.Spec(time.Second),
	})

	// T_wall is the wall-clock time when the stream epoch (ts=0) occurred.
	T_wall := utc.UnixMilli(10_000)

	// Packet 1 (discard): ts=0, now=T_wall → discard.T0 = T_wall.
	_, d, err := p.Packet(T_wall, 1, 0)
	require.NoError(t, err)
	require.True(t, d)

	// Packet 2 (discard): ts=10ms, now=T_wall+10ms → t0=T_wall (stable), elapsed=10ms < 15ms → still discarding.
	_, d, err = p.Packet(T_wall.Add(10*time.Millisecond), 2, ticksMS(10))
	require.NoError(t, err)
	require.True(t, d)

	// Packet 3 (first non-discarded): arrives 5ms late due to jitter.
	// Expected arrival: T_wall+20ms. Actual arrival: T_wall+25ms.
	// t0_first = T_wall+25ms - 20ms = T_wall+5ms > discard.T0 = T_wall (5ms of positive jitter).
	now3 := T_wall.Add(25 * time.Millisecond)
	ts3 := ticksMS(20)
	target3, d, err := p.Packet(now3, 3, ts3)
	require.NoError(t, err)
	require.False(t, d)
	// baseTime = discard.T0 + TicksToDuration(ts3) + delay = T_wall + 20ms + delay.
	// targetTime3 = baseTime + 0 = T_wall + 20ms + delay.
	wantTarget3 := T_wall.Add(20*time.Millisecond + delay)
	require.Equal(t, wantTarget3, target3, "first packet target must be anchored to discard.T0, not jitter-affected now")

	// Packet 4 (no jitter): ts=30ms, arrives at expected time T_wall+30ms.
	// t0 = T_wall = discard.T0 = MinT0 → no NegDrift triggered.
	now4 := T_wall.Add(30 * time.Millisecond)
	ts4 := ticksMS(30)
	target4, d, err := p.Packet(now4, 4, ts4)
	require.NoError(t, err)
	require.False(t, d)
	// targetTime4 = baseTime + TicksToDuration(ts4 - ts3) = T_wall+20ms+delay + 10ms = T_wall+30ms+delay.
	wantTarget4 := T_wall.Add(30*time.Millisecond + delay)
	require.Equal(t, wantTarget4, target4, "subsequent packet target must be correct")
	require.Equal(t, delay, target4.Sub(now4), "pushAhead must equal delay, not delay+jitter")

	require.Zero(t, stats.NegDrift.Count, "no spurious NegDrift correction despite 5ms arrival jitter on first packet")
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
