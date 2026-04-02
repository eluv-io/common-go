package pacer_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	"github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

// ticksMS converts a number of milliseconds to RTP ticks (90kHz clock).
func ticksMS(ms int) int64 {
	return rtpDurationToTicks(time.Duration(ms) * time.Millisecond)
}

// newTestPacerLogic creates a PacerLogic for testing. discardPeriod=0 means that discarding is disabled; pass a value >
// 0 to test the timed discard phase.
func newTestPacerLogic(discardPeriod, delay time.Duration, maxDiscardPeriod ...time.Duration) (*pacer.PacerLogic, *pacer.InStats) {
	stats := &pacer.InStats{}
	maxDiscard := discardPeriod * 2
	if len(maxDiscardPeriod) > 0 {
		maxDiscard = maxDiscardPeriod[0]
	}

	conf := pacer.PacerLogicConfig{
		Stream:           "test-stream",
		EventLog:         log.Get("/test/pacer"),
		DiscardPeriod:    duration.Spec(discardPeriod),
		MaxDiscardPeriod: duration.Spec(maxDiscard),
		Delay:            duration.Spec(delay),
		ToDuration:       rtpToDuration,
	}

	p := pacer.NewPacerLogic(conf, stats)
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
	_, discarded, _ := p.PacketTs(T0, 0, false)
	require.True(t, discarded, "first packet must always be discarded")

	// Packets 1–9 arrive at 10ms intervals; elapsed < 100ms, still discarding.
	for i := 1; i <= 9; i++ {
		ts := ticksMS(i * 10)
		now := T0.Add(time.Duration(i) * 10 * time.Millisecond)
		_, discarded, _ = p.PacketTs(now, ts, false)
		require.True(t, discarded,
			"packet %d: elapsed=%s < %s, should still be discarded",
			i, now.Sub(T0), discardPeriod)
	}

	// Packet 10 at exactly 100ms: elapsed >= discardPeriod → discard phase ends.
	_, discarded, _ = p.PacketTs(T0.Add(100*time.Millisecond), ticksMS(100), false)
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
	target, discarded, err := p.PacketTs(now1, ts1, false)
	require.NoError(t, err)
	require.False(t, discarded)

	// The first non-discarded packet has rtpDelta=0, so target = baseTime = now + delay.
	assert.Equal(t, now1.Add(delay), target)
}

// TestPacerLogic_TargetTime verifies that subsequent packets compute
// targetTime = baseTime + rtpToDuration(ts - firstTs).
func TestPacerLogic_TargetTime(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, _ := newTestPacerLogic(0, delay)

	T0 := utc.UnixMilli(10_000)

	ts1 := ticksMS(10_000_000)
	target, discarded, err := p.PacketTs(T0, ts1, false)
	require.NoError(t, err)
	require.False(t, discarded)
	require.Equal(t, T0.Add(delay), target)

	baseTime := T0.Add(delay)

	tests := []struct {
		ts     int64         // unwrapped RTP timestamp
		nowOff time.Duration // wall-clock offset from T0
	}{
		{ts1 + ticksMS(10), 10 * time.Millisecond},
		{ts1 + ticksMS(20), 20 * time.Millisecond},
		{ts1 + ticksMS(100), 100 * time.Millisecond},
		{ts1 + ticksMS(500), 500 * time.Millisecond},
	}

	for _, tt := range tests {
		now := T0.Add(tt.nowOff)
		target, discarded, err = p.PacketTs(now, tt.ts, false)
		require.NoError(t, err)
		require.False(t, discarded, "ts=%d should not be discarded", tt.ts)

		rtpDelta := tt.ts - ts1
		wantTarget := baseTime.Add(rtpToDuration(rtpDelta))
		assert.Equal(t, wantTarget, target, "ts=%d: wrong target time", tt.ts)
	}
}

// TestPacerLogic_GapReset verifies that a gap signal resets all per-session state, restarts the discard phase,
// increments StreamResets, and that StreamResets accumulates correctly across multiple gaps.
func TestPacerLogic_GapReset(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogic(15*time.Millisecond, delay)

	T0 := utc.UnixMilli(10_000)

	// Establish baseline and accumulate some stats.
	_, discarded, err := p.PacketTs(T0, 0, false)
	require.NoError(t, err)
	require.True(t, discarded)

	_, discarded, err = p.PacketTs(T0.Add(10*time.Millisecond), ticksMS(10), false)
	require.NoError(t, err)
	require.True(t, discarded)

	// discard period (15ms) has elapsed, so baseline established.
	_, discarded, err = p.PacketTs(T0.Add(20*time.Millisecond), ticksMS(20), false)
	require.NoError(t, err)
	require.False(t, discarded)
	require.NotZero(t, stats.MinT0, "MinT0 should be set after baseline")
	require.NotZero(t, stats.PushAhead.Min, "PushAhead.Min should be set after baseline")

	// Signal gap 1 via gap=true.
	_, discarded, err = p.PacketTs(T0.Add(30*time.Millisecond), ticksMS(30), true)
	require.NoError(t, err)

	// The gap triggers a reset; the gap packet enters a new discard phase.
	assert.True(t, discarded, "first packet after gap should be discarded (new discard phase)")
	assert.Zero(t, stats.MinT0, "MinT0 should be reset after gap")
	assert.Zero(t, stats.PushAhead.Min, "PushAhead.Min should be reset after gap")
	assert.Zero(t, stats.PushAhead.Max, "PushAhead.Max should be reset after gap")
	assert.Equal(t, 1, stats.StreamResets, "StreamResets should be 1 after first gap")
	assert.Equal(t, T0.Add(30*time.Millisecond), stats.LastStreamReset, "LastStreamReset should record the gap time")

	// Complete the new discard phase and re-establish a baseline.
	_, discarded, err = p.PacketTs(T0.Add(40*time.Millisecond), ticksMS(40), false)
	require.NoError(t, err)
	require.True(t, discarded, "second packet in post-gap discard phase should still be discarded")

	_, discarded, err = p.PacketTs(T0.Add(50*time.Millisecond), ticksMS(50), false)
	require.NoError(t, err)
	require.False(t, discarded, "packet after post-gap discard period should not be discarded")
	require.NotZero(t, stats.MinT0, "MinT0 should be set after second baseline")
	require.NotZero(t, stats.PushAhead.Min, "PushAhead.Min should be set after second baseline")

	// Signal gap 2. StreamResets must accumulate to 2, not reset to 1.
	_, discarded, err = p.PacketTs(T0.Add(60*time.Millisecond), ticksMS(60), true)
	require.NoError(t, err)
	assert.True(t, discarded, "first packet after second gap should be discarded")
	assert.Equal(t, 2, stats.StreamResets, "StreamResets must accumulate across gaps, not reset to 1")
	assert.Equal(t, T0.Add(60*time.Millisecond), stats.LastStreamReset, "LastStreamReset should record the second gap time")
}

// TestPacerLogic_PushAheadStats verifies that PushAhead.Min and PushAhead.Max
// track the min and max lead time of packets relative to their target times.
func TestPacerLogic_PushAheadStats(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogic(0, delay)

	T0 := utc.UnixMilli(10_000)

	// Establish baseline.
	p.PacketTs(T0, 0, false)
	now1 := T0.Add(10 * time.Millisecond)
	ts1 := ticksMS(10)
	target1, _, _ := p.PacketTs(now1, ts1, false)

	// Baseline packet: target = now1+delay, pushAhead = delay.
	assert.Equal(t, delay, target1.Sub(now1))
	assert.EqualValues(t, delay, stats.PushAhead.Min)
	assert.EqualValues(t, delay, stats.PushAhead.Max)

	// Late arrival: wall clock advances 20ms, RTP only 10ms.
	now2 := now1.Add(20 * time.Millisecond)
	ts2 := ts1 + ticksMS(10)
	p.PacketTs(now2, ts2, false)
	assert.EqualValues(t, delay-10*time.Millisecond, stats.PushAhead.Min, "late arrival shrinks PushAhead.Min")
	assert.EqualValues(t, delay, stats.PushAhead.Max, "PushAhead.Max unchanged")

	// Early arrival: wall clock advances 5ms, RTP advances 20ms.
	now3 := now2.Add(5 * time.Millisecond)
	ts3 := ts2 + ticksMS(20)
	p.PacketTs(now3, ts3, false)
	assert.EqualValues(t, delay-10*time.Millisecond, stats.PushAhead.Min, "PushAhead.Min unchanged")
	assert.EqualValues(t, delay+5*time.Millisecond, stats.PushAhead.Max, "early arrival grows PushAhead.Max")
}

// TestPacerLogic_T0Adjustments verifies that T0 decreases in the active phase
// are counted and accumulated in NegDrift stats.
func TestPacerLogic_T0Adjustments(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogic(0, delay)

	wallEpoch := utc.UnixMilli(10_000)

	// Establish baseline: packet 0 discarded, packet 1 sets baseline.
	p.PacketTs(wallEpoch, 0, false)
	now1 := wallEpoch.Add(10 * time.Millisecond)
	ts1 := ticksMS(10)
	p.PacketTs(now1, ts1, false)

	assert.Equal(t, wallEpoch, stats.MinT0)
	assert.Zero(t, stats.NegDrift.Count)

	// Packet 2: wall advances 15ms, RTP advances 20ms → packet arrives early.
	now2 := now1.Add(15 * time.Millisecond)
	ts2 := ts1 + ticksMS(20)
	p.PacketTs(now2, ts2, false)
	assert.Equal(t, uint64(1), stats.NegDrift.Count)
	assert.EqualValues(t, 5*time.Millisecond, stats.NegDrift.Sum)
	assert.Equal(t, wallEpoch.Add(-5*time.Millisecond), stats.MinT0)

	// Packet 3: wall advances 10ms, RTP advances 15ms → another early arrival.
	now3 := now2.Add(10 * time.Millisecond)
	ts3 := ts2 + ticksMS(15)
	p.PacketTs(now3, ts3, false)
	assert.Equal(t, uint64(2), stats.NegDrift.Count)
	assert.EqualValues(t, 10*time.Millisecond, stats.NegDrift.Sum)
	assert.EqualValues(t, wallEpoch.Add(-10*time.Millisecond), stats.MinT0)

	// Packet 4: wall and RTP advance in sync → T0 stable, no adjustment.
	now4 := now3.Add(10 * time.Millisecond)
	ts4 := ts3 + ticksMS(10)
	p.PacketTs(now4, ts4, false)
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

	// Packet 0: ts=0, now=T0 → discard.T0 = T0.
	_, d0, err := p.PacketTs(T0, 0, false)
	require.NoError(t, err)
	require.True(t, d0)

	// Packet 1: RTP advances 20ms while wall clock only advances 10ms.
	_, d1, err := p.PacketTs(T0.Add(10*time.Millisecond), ticksMS(20), false)
	require.NoError(t, err)
	require.True(t, d1, "T0 moved backward so discard timer resets, still discarding")

	// Packet 2: T0 stable; discardPeriod elapsed → discard ends.
	_, d2, err := p.PacketTs(T0.Add(20*time.Millisecond), ticksMS(30), false)
	require.NoError(t, err)
	require.False(t, d2, "T0 stable and discardPeriod elapsed → baseline established")

	assert.EqualValues(t, 10*time.Millisecond, stats.StartupT0Correction.Sum)
}

// newTestPacerLogicFull creates a PacerLogic from a complete PacerLogicConfig.
func newTestPacerLogicFull(conf pacer.PacerLogicConfig) (*pacer.PacerLogic, *pacer.InStats) {
	stats := &pacer.InStats{}
	if conf.EventLog == nil {
		conf.EventLog = log.Get("/test/pacer")
	}
	if conf.ToDuration == nil {
		conf.ToDuration = rtpToDuration
	}
	return pacer.NewPacerLogic(conf, stats), stats
}

// TestPacerLogic_AdjustTimeDrift_Applied verifies that when AdjustTimeDrift=true (no cap), a T0 backward shift of X ms
// causes subsequent target times to be pulled X ms earlier compared to the unadjusted case.
func TestPacerLogic_AdjustTimeDrift_Applied(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift: true,
		Delay:           duration.Spec(delay),
		ToDuration:      rtpToDuration,
	})

	T0 := utc.UnixMilli(10_000)

	now1 := T0
	ts1 := ticksMS(0)
	_, discarded, err := p.PacketTs(now1, ts1, false)
	require.NoError(t, err)
	require.False(t, discarded)
	baseTime := now1.Add(delay)

	// Packet 2: wall clock advances 15ms, RTP advances 20ms → T0 shifts 5ms earlier.
	now2 := now1.Add(15 * time.Millisecond)
	ts2 := ts1 + ticksMS(20)
	target2, discarded, err := p.PacketTs(now2, ts2, false)
	require.NoError(t, err)
	require.False(t, discarded)
	require.Equal(t, uint64(1), stats.NegDrift.Count, "nominal adjustment must be recorded")
	require.EqualValues(t, 5*time.Millisecond, stats.NegDrift.Sum)
	require.EqualValues(t, uint64(1), stats.NegDriftApplied.Count, "applied adjustment must be recorded")
	require.EqualValues(t, 5*time.Millisecond, stats.NegDriftApplied.Sum, "full drift applied (no cap)")

	rtpDelta2 := rtpToDuration(ts2 - ts1)
	wantUnadjusted := baseTime.Add(rtpDelta2)
	wantAdjusted := wantUnadjusted.Add(-5 * time.Millisecond)
	require.Equal(t, wantAdjusted, target2, "target must be 5ms earlier due to time-ref adjustment")
}

// TestPacerLogic_AdjustTimeDrift_Cap verifies that MaxNegDriftCorrection limits how much of the observed T0 drift is
// applied to baseTime in a single PacketTs call.
func TestPacerLogic_AdjustTimeDrift_Cap(t *testing.T) {
	const delay = 500 * time.Millisecond
	const capAdj = 3 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift:       true,
		MaxNegDriftCorrection: duration.Spec(capAdj),
		Delay:                 duration.Spec(delay),
		ToDuration:            rtpToDuration,
	})

	T0 := utc.UnixMilli(10_000)

	now1 := T0
	ts1 := ticksMS(0)
	_, discarded, err := p.PacketTs(now1, ts1, false)
	require.NoError(t, err)
	require.False(t, discarded)
	baseTime := now1.Add(delay)

	// Packet 2: T0 drifts back 10ms (large single-step drift, capped at 3ms).
	now2 := now1.Add(5 * time.Millisecond)
	ts2 := ts1 + ticksMS(15)
	target2, discarded, err := p.PacketTs(now2, ts2, false)
	require.NoError(t, err)
	require.False(t, discarded)

	require.Equal(t, uint64(1), stats.NegDrift.Count)
	require.EqualValues(t, 10*time.Millisecond, stats.NegDrift.Sum, "full nominal drift must be recorded")
	require.Equal(t, uint64(1), stats.NegDriftApplied.Count)
	require.EqualValues(t, capAdj, stats.NegDriftApplied.Sum, "only capped amount must be applied")

	rtpDelta2 := rtpToDuration(ts2 - ts1)
	wantUnadjusted := baseTime.Add(rtpDelta2)
	wantAdjusted := wantUnadjusted.Add(-capAdj)
	require.Equal(t, wantAdjusted, target2, "target must reflect only the capped 3ms correction")
}

// TestPacerLogic_SlowDrift_Correction verifies that when the mean positive T0 drift over a period exceeds the
// threshold, a positive correction is applied to baseTime and recorded in stats.
func TestPacerLogic_SlowDrift_Correction(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift:    true,
		PosDriftPeriod:     duration.Spec(60 * time.Millisecond),
		PosDriftThreshold:  duration.Spec(2 * time.Millisecond),
		PosDriftCorrection: duration.Spec(time.Millisecond),
		Delay:              duration.Spec(delay),
		ToDuration:         rtpToDuration,
	})

	T0 := utc.UnixMilli(10_000)

	now := T0
	ts := ticksMS(0)
	_, discarded, err := p.PacketTs(now, ts, false)
	require.NoError(t, err)
	require.False(t, discarded)
	baseTime := now.Add(delay)

	for i := 2; i <= 7; i++ {
		now = now.Add(10 * time.Millisecond)
		ts += ticksMS(8)
		_, discarded, err = p.PacketTs(now, ts, false)
		require.NoError(t, err)
		require.False(t, discarded, "packet %d should not be discarded", i)
	}
	require.Zero(t, stats.PosDriftApplied.Count, "no correction yet: period not elapsed")

	// Packet 8 at +70ms: period ends (70ms > 60ms), mean=6ms > 2ms → correction applied.
	now = now.Add(10 * time.Millisecond)
	ts += ticksMS(8)
	target8, discarded, err := p.PacketTs(now, ts, false)
	require.NoError(t, err)
	require.False(t, discarded)

	require.Equal(t, uint64(1), stats.PosDrift.Count, "one period must be recorded")
	require.EqualValues(t, 6*time.Millisecond, stats.PosDrift.Sum, "mean drift for the period")
	require.Equal(t, uint64(1), stats.PosDriftApplied.Count, "one correction must be applied")
	require.EqualValues(t, time.Millisecond, stats.PosDriftApplied.Sum)

	rtpDelta8 := rtpToDuration(ts - ticksMS(0))
	wantUnadjusted := baseTime.Add(rtpDelta8)
	require.Equal(t, wantUnadjusted.Add(time.Millisecond), target8,
		"target must be 1ms later due to slow-drift correction")
}

// TestPacerLogic_SlowDrift_BelowThreshold verifies that no correction is applied when the mean positive drift
// is below the configured threshold.
func TestPacerLogic_SlowDrift_BelowThreshold(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift:    true,
		PosDriftPeriod:     duration.Spec(60 * time.Millisecond),
		PosDriftThreshold:  duration.Spec(10 * time.Millisecond),
		PosDriftCorrection: duration.Spec(time.Millisecond),
		Delay:              duration.Spec(delay),
		ToDuration:         rtpToDuration,
	})

	T0 := utc.UnixMilli(10_000)

	now := T0
	ts := ticksMS(0)
	_, discarded, err := p.PacketTs(now, ts, false)
	require.NoError(t, err)
	require.False(t, discarded)

	for i := 2; i <= 8; i++ {
		now = now.Add(10 * time.Millisecond)
		ts += ticksMS(8)
		_, discarded, err = p.PacketTs(now, ts, false)
		require.NoError(t, err)
		require.False(t, discarded, "packet %d should not be discarded", i)
	}

	require.Zero(t, stats.PosDriftApplied.Count, "mean=6ms < threshold=10ms: no correction must be applied")
}

// TestPacerLogic_SlowDrift_StatsWithoutCorrection verifies that T0SlowDrift is recorded even when AdjustTimeDrift is
// false.
func TestPacerLogic_SlowDrift_StatsWithoutCorrection(t *testing.T) {
	const delay = 500 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift:    false,
		PosDriftPeriod:     duration.Spec(60 * time.Millisecond),
		PosDriftThreshold:  duration.Spec(2 * time.Millisecond),
		PosDriftCorrection: duration.Spec(time.Millisecond),
		Delay:              duration.Spec(delay),
		ToDuration:         rtpToDuration,
	})

	T0 := utc.UnixMilli(10_000)

	now := T0
	ts := ticksMS(0)
	_, discarded, err := p.PacketTs(now, ts, false)
	require.NoError(t, err)
	require.False(t, discarded)
	baseTime := now.Add(delay)

	for i := 2; i <= 7; i++ {
		now = now.Add(10 * time.Millisecond)
		ts += ticksMS(8)
		_, discarded, err = p.PacketTs(now, ts, false)
		require.NoError(t, err)
		require.False(t, discarded, "packet %d should not be discarded", i)
	}

	now = now.Add(10 * time.Millisecond)
	ts += ticksMS(8)
	target8, discarded, err := p.PacketTs(now, ts, false)
	require.NoError(t, err)
	require.False(t, discarded)

	require.Equal(t, uint64(1), stats.PosDrift.Count, "drift must be recorded even without AdjustTimeDrift")
	require.EqualValues(t, 6*time.Millisecond, stats.PosDrift.Sum)
	require.Zero(t, stats.PosDriftApplied.Count, "no correction must be applied when AdjustTimeDrift=false")

	rtpDelta8 := rtpToDuration(ts - ticksMS(0))
	wantUnadjusted := baseTime.Add(rtpDelta8)
	require.Equal(t, wantUnadjusted, target8, "target must be unadjusted when AdjustTimeDrift=false")
}

// TestPacerLogic_StartupJitter verifies that positive jitter on the first non-discarded packet does not offset the
// entire timeline, because baseTime is anchored to discard.T0 rather than `now`.
func TestPacerLogic_StartupJitter(t *testing.T) {
	const delay = 500 * time.Millisecond
	const discardPeriod = 15 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift:  false,
		DiscardPeriod:    duration.Spec(discardPeriod),
		MaxDiscardPeriod: duration.Spec(time.Minute),
		Delay:            duration.Spec(delay),
		ToDuration:       rtpToDuration,
	})

	T_wall := utc.UnixMilli(10_000)

	_, d, err := p.PacketTs(T_wall, 0, false)
	require.NoError(t, err)
	require.True(t, d)

	_, d, err = p.PacketTs(T_wall.Add(10*time.Millisecond), ticksMS(10), false)
	require.NoError(t, err)
	require.True(t, d)

	// Packet 3 arrives 5ms late.
	now3 := T_wall.Add(25 * time.Millisecond)
	ts3 := ticksMS(20)
	target3, d, err := p.PacketTs(now3, ts3, false)
	require.NoError(t, err)
	require.False(t, d)
	wantTarget3 := T_wall.Add(20*time.Millisecond + delay)
	require.Equal(t, wantTarget3, target3, "first packet target must be anchored to discard.T0, not jitter-affected now")

	now4 := T_wall.Add(30 * time.Millisecond)
	ts4 := ticksMS(30)
	target4, d, err := p.PacketTs(now4, ts4, false)
	require.NoError(t, err)
	require.False(t, d)
	wantTarget4 := T_wall.Add(30*time.Millisecond + delay)
	require.Equal(t, wantTarget4, target4, "subsequent packet target must be correct")
	require.Equal(t, delay, target4.Sub(now4), "pushAhead must equal delay, not delay+jitter")

	require.Zero(t, stats.NegDrift.Count, "no spurious NegDrift correction despite 5ms arrival jitter on first packet")
}

func TestPacerLogicConfig_Unmarshal(t *testing.T) {
	t.Run("InitDefaults", func(t *testing.T) {

		cfg := pacer.PacerLogicConfig{}
		cfg.InitDefaults()
		err := json.Unmarshal([]byte(`{}`), &cfg)
		require.NoError(t, err)
		require.Equal(t, *new(pacer.PacerLogicConfig).InitDefaults(), cfg)
	})

	t.Run("InitDefaults", func(t *testing.T) {
		cfg := pacer.PacerLogicConfig{
			DiscardPeriod:         duration.Second,
			MaxDiscardPeriod:      10 * duration.Second,
			Delay:                 200 * duration.Millisecond,
			AdjustTimeDrift:       false,
			MaxNegDriftCorrection: 100 * duration.Millisecond,
			PosDriftPeriod:        5 * duration.Minute,
			PosDriftThreshold:     5 * duration.Millisecond,
			PosDriftCorrection:    10 * duration.Millisecond,
		}

		marshaled, err := json.Marshal(cfg)
		require.NoError(t, err)

		unmarshaled := pacer.PacerLogicConfig{}
		err = json.Unmarshal(marshaled, &unmarshaled)
		require.NoError(t, err)
		require.Equal(t, cfg, unmarshaled)
	})
}
