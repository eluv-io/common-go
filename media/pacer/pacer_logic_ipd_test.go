package pacer_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/utc-go"
)

// TestPacerLogic_DriftCorrectionSpread_ConstantPositiveDrift verifies the fix for the IPD-peak behavior for a source
// whose media clock runs persistently (but only slightly) slower than wall clock: pacer#394bffea made the
// positive-drift correction proportional to the *mean* drift over PosDriftPeriod, but applied it in a single step to
// the packet that closed out the period, producing a single-packet inter-packet-delay (IPD) spike (nominal IPD plus the
// full correction). MaxDriftCorrectionStep bounds that: the same total correction is now spread over as many packets as
// needed so no single packet absorbs more than the configured step.
func TestPacerLogic_DriftCorrectionSpread_ConstantPositiveDrift(t *testing.T) {
	const delay = 500 * time.Millisecond
	const nominalIPD = 8 * time.Millisecond // each packet's RTP timestamp advances by 8ms
	const maxStep = 3 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift:        true,
		PosDriftPeriod:         duration.Spec(60 * time.Millisecond),
		DriftThreshold:         duration.Spec(2 * time.Millisecond),
		MaxDriftCorrectionStep: duration.Duration(maxStep),
		Delay:                  duration.Spec(delay),
		ToDuration:             rtp.TicksToDuration,
	})

	T0 := utc.UnixMilli(10_000)

	// Source clock is a steady 20% slow: every 10ms of wall clock, the media clock advances only 8ms. This is a
	// constant drift rate, not a one-off glitch.
	const packets = 10
	now := T0
	ts := ticksMS(0)
	targets := make([]utc.UTC, 0, packets)

	target, discarded, err := p.Packet(now, ts, false)
	require.NoError(t, err)
	require.False(t, discarded)
	targets = append(targets, target)

	for i := 2; i <= packets; i++ {
		now = now.Add(10 * time.Millisecond)
		ts += ticksMS(8)
		target, discarded, err = p.Packet(now, ts, false)
		require.NoError(t, err)
		require.False(t, discarded, "packet %d should not be discarded", i)
		targets = append(targets, target)
	}

	// The period-1 mean drift (6ms, as before the fix) is now drained over two packets (3ms + 3ms) instead of one.
	require.Equal(t, uint64(2), stats.PosDriftApplied.Count, "the 6ms correction must be drained over multiple packets")
	require.EqualValues(t, 6*time.Millisecond, stats.PosDriftApplied.Sum,
		"the total amount corrected is unchanged - spreading bounds the rate, not the total")

	// Compute inter-packet delays (IPD) between consecutive target times.
	ipds := make([]time.Duration, len(targets)-1)
	for i := 1; i < len(targets); i++ {
		ipds[i-1] = targets[i].Sub(targets[i-1])
	}

	// Packets 2-7 (indices 0-5): no correction pending yet, IPD is nominal.
	for i := 0; i < 6; i++ {
		require.Equal(t, nominalIPD, ipds[i], "ipd[%d]: no correction pending, IPD must be nominal", i)
	}

	// Packets 8-9 (indices 6-7): the period-end correction drains at maxStep per packet, so each absorbs only
	// nominal+3ms instead of the old nominal+6ms lump.
	require.Equal(t, nominalIPD+maxStep, ipds[6], "packet 8 must absorb at most maxStep on top of nominal IPD")
	require.Equal(t, nominalIPD+maxStep, ipds[7], "packet 9 must absorb the remaining half of the correction")

	// Packet 10 (index 8): the correction has fully drained, IPD is nominal again.
	require.Equal(t, nominalIPD, ipds[8], "IPD must return to nominal once the pending correction has fully drained")
}

// TestPacerLogic_DriftCorrectionSpread_BackwardTimestampStep verifies the same fix for an IPD peak triggered by an RTP
// timestamp irregularity rather than by continuous drift: a faulty encoder that permanently falls behind its own clock
// (holds or under-increments its timestamp counter for one frame, then resumes normal cadence from the lower value).
// From the pacer's point of view this step is indistinguishable from "positive drift" - both raise T0 relative to MinT0
// and stay there - so, like constant drift, it is now drained over multiple packets instead of dumped into one. Note
// this only produces a correction at all because the step is *persistent* (like a genuine clock-rate change): see
// TestPacerLogic_IPDPeak_ForwardTimestampJump_SelfCancels for the opposite-direction case.
func TestPacerLogic_DriftCorrectionSpread_BackwardTimestampStep(t *testing.T) {
	const delay = 500 * time.Millisecond
	const maxStep = 3 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift:        true,
		PosDriftPeriod:         duration.Spec(60 * time.Millisecond),
		DriftThreshold:         duration.Spec(2 * time.Millisecond),
		MaxDriftCorrectionStep: duration.Duration(maxStep),
		Delay:                  duration.Spec(delay),
		ToDuration:             rtp.TicksToDuration,
	})

	T0 := utc.UnixMilli(10_000)

	type packetSpec struct {
		nowOff time.Duration // offset against T0 in millis
		tsMs   int           // timestamp in millis
	}
	// Packet 1: baseline. Packet 2: a single faulty-encoder hold - the timestamp advances only 3ms while 10ms of wall
	// clock elapse (a 7ms hold). Packets 3-10: the encoder resumes normal cadence, but permanently offset by the hold -
	// exactly what a real timestamp-counter glitch looks like (a step, not a self-correcting blip).
	specs := []packetSpec{
		{0, 0},
		{10 * time.Millisecond, 3}, // faulty-encoder hold: wall+10ms, ts+3ms
		{20 * time.Millisecond, 13},
		{30 * time.Millisecond, 23},
		{40 * time.Millisecond, 33},
		{50 * time.Millisecond, 43},
		{60 * time.Millisecond, 53},
		{70 * time.Millisecond, 63}, // period ends here (70ms > 60ms) -> correction begins draining
		{80 * time.Millisecond, 73},
		{90 * time.Millisecond, 83},
	}

	targets := make([]utc.UTC, 0, len(specs))
	for i, s := range specs {
		now := T0.Add(s.nowOff)
		target, discarded, err := p.Packet(now, ticksMS(s.tsMs), false)
		require.NoError(t, err)
		require.False(t, discarded, "packet %d should not be discarded", i+1)
		targets = append(targets, target)
	}

	// The hold never looks like negative drift (T0 only ever increases relative to MinT0), so it is exclusively
	// handled by the positive-drift path.
	require.Zero(t, stats.NegDrift.Count, "a timestamp hold must not be seen as negative drift")

	// Mean drift over period 1 (packets 1-7): 0ms (baseline) then 7ms for each packet 2-7
	// ==> mean = 6pkt*7ms/7pkt=6ms
	// drained over two packets (3ms + 3ms) because of 3ms maxStep.
	require.Equal(t, uint64(2), stats.PosDriftApplied.Count, "the 6ms correction must be drained over multiple packets")
	require.EqualValues(t, 6*time.Millisecond, stats.PosDriftApplied.Sum,
		"the total amount corrected is unchanged - spreading bounds the rate, not the total")

	ipds := make([]time.Duration, len(targets)-1)
	for i := 1; i < len(targets); i++ {
		ipds[i-1] = targets[i].Sub(targets[i-1])
	}

	// Packet 2 (the hold itself): IPD dips to 3ms - this is the genuine, harmless effect of the encoder's own
	// under-increment and is not what this test is about.
	require.Equal(t, 3*time.Millisecond, ipds[0], "the hold itself produces a (harmless) IPD dip, not a peak")

	// Packets 3-7 resume nominal 10ms IPD; no correction is pending yet.
	for i := 1; i < 6; i++ {
		require.Equal(t, 10*time.Millisecond, ipds[i], "ipd[%d]: no correction pending yet, IPD must be nominal", i)
	}

	// Packets 8-9: the period-end correction drains at maxStep per packet - 13ms instead of the old 16ms lump.
	require.Equal(t, 10*time.Millisecond+maxStep, ipds[6], "packet 8 must absorb at most maxStep on top of nominal IPD")
	require.Equal(t, 10*time.Millisecond+maxStep, ipds[7], "packet 9 must absorb the remaining half of the correction")

	// Packet 10: the correction has fully drained, IPD is nominal again.
	require.Equal(t, 10*time.Millisecond, ipds[8], "IPD must return to nominal once the pending correction has drained")
}

// TestPacerLogic_IPDPeak_ForwardTimestampJump_SelfCancels covers the mirror-image timestamp irregularity: the encoder
// jumps *ahead* (timestamp advances more than wall clock for one frame, then resumes normal cadence from the higher
// value) instead of falling behind. This is deliberately run with production defaults - AdjustTimeDrift=true,
// MaxNegDriftCorrection=0 (see config/testdata/default-config.yaml in content-fabric; InitDefaults() matches) -
// because the outcome depends entirely on that cap being unset.
//
// A forward jump makes T0 drop below MinT0, so it is handled by the *negative*-drift path, not the positive-drift path
// exercised by the other two tests above. That path applies its correction immediately (not spread over a period), and
// with no cap the correction is mathematically exactly equal to the raw timestamp-driven jump it needs to offset: the
// packet's target time ends up unchanged, and IPD stays nominal. So - contrary to the hypothesis that any timestamp
// jump reproduces the same peak as constant drift - a forward jump under today's default config produces no peak at
// all; it is the backward/under-increment direction (previous tests) that behaves like positive drift and needed the
// spreading fix. If MaxNegDriftCorrection were configured non-zero, this cancellation would become partial - see
// TestPacerLogic_NegativeDrift_NotSpreadByMaxDriftCorrectionStep for why MaxDriftCorrectionStep does not, and must
// not, apply to this path at all.
func TestPacerLogic_IPDPeak_ForwardTimestampJump_SelfCancels(t *testing.T) {
	const delay = 500 * time.Millisecond
	const nominalIPD = 10 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift: true,
		DriftThreshold:  duration.Spec(2 * time.Millisecond),
		Delay:           duration.Spec(delay),
		ToDuration:      rtp.TicksToDuration,
		// MaxNegDriftCorrection, MaxDriftCorrectionStep and PosDriftPeriod left at zero value; NewPacerLogic applies
		// the same defaults as InitDefaults() (uncapped negative correction, no per-packet spread cap, 1-minute
		// positive-drift period - irrelevant here).
	})

	T0 := utc.UnixMilli(10_000)

	type packetSpec struct {
		nowOff time.Duration
		tsMs   int
	}
	// Packet 1: baseline. Packet 2: steady (in sync). Packet 3: the encoder jumps its timestamp ahead by 40ms (13ms
	// wall-clock elapsed, but the timestamp reports 60ms instead of the expected 20ms). Packets 4-5: the encoder
	// resumes normal cadence from the jumped (higher) value - a permanent step, not a self-correcting blip.
	specs := []packetSpec{
		{0, 0},
		{10 * time.Millisecond, 10},
		{20 * time.Millisecond, 60}, // +40ms forward jump
		{30 * time.Millisecond, 70},
		{40 * time.Millisecond, 80},
	}

	targets := make([]utc.UTC, 0, len(specs))
	for i, s := range specs {
		now := T0.Add(s.nowOff)
		target, discarded, err := p.Packet(now, ticksMS(s.tsMs), false)
		require.NoError(t, err)
		require.False(t, discarded, "packet %d should not be discarded", i+1)
		targets = append(targets, target)
	}

	// The jump is handled once, immediately, by the negative-drift path - not the positive-drift path.
	require.Equal(t, uint64(1), stats.NegDrift.Count, "a forward jump must be seen as negative drift, not positive")
	require.EqualValues(t, 40*time.Millisecond, stats.NegDrift.Sum, "the nominal (uncapped) drift equals the jump size")
	require.EqualValues(t, 40*time.Millisecond, stats.NegDriftApplied.Sum, "uncapped: the full jump is applied at once")
	require.Zero(t, stats.PosDriftApplied.Count, "the positive-drift path must never fire for this jump direction")

	ipds := make([]time.Duration, len(targets)-1)
	for i := 1; i < len(targets); i++ {
		ipds[i-1] = targets[i].Sub(targets[i-1])
	}

	for i, ipd := range ipds {
		require.Equal(t, nominalIPD, ipd,
			"ipd[%d]: the immediate, uncapped negative-drift correction exactly cancels the raw timestamp jump", i)
	}
}

// TestPacerLogic_NegativeDrift_NotSpreadByMaxDriftCorrectionStep guards against reintroducing spreading on the
// negative-drift path. Unlike a positive correction (which only ever lengthens the gap to the next target time and so
// can be safely rate-limited), a negative correction shortens that gap - spreading it across several packets would
// shrink, and can collapse or invert, their IPD instead of bounding it. This was caught during development of
// MaxDriftCorrectionStep: with a naive symmetric implementation, a 40ms forward timestamp jump with a 10ms step
// produced a burst of four back-to-back (0ms IPD) deliveries instead of the intended bounded spread. This test pins
// down the fix: negative drift is always applied in a single step, regardless of MaxDriftCorrectionStep.
func TestPacerLogic_NegativeDrift_NotSpreadByMaxDriftCorrectionStep(t *testing.T) {
	const delay = 500 * time.Millisecond
	const nominalIPD = 10 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift:        true,
		DriftThreshold:         duration.Spec(2 * time.Millisecond),
		MaxDriftCorrectionStep: duration.Duration(10 * time.Millisecond),
		Delay:                  duration.Spec(delay),
		ToDuration:             rtp.TicksToDuration,
	})

	T0 := utc.UnixMilli(10_000)

	type packetSpec struct {
		nowOff time.Duration
		tsMs   int
	}
	// Packet 3: a 40ms forward timestamp jump - much larger than the 10ms MaxDriftCorrectionStep.
	specs := []packetSpec{
		{0, 0},
		{10 * time.Millisecond, 10},
		{20 * time.Millisecond, 60}, // +40ms forward jump
		{30 * time.Millisecond, 70},
		{40 * time.Millisecond, 80},
	}

	targets := make([]utc.UTC, 0, len(specs))
	for i, s := range specs {
		now := T0.Add(s.nowOff)
		target, discarded, err := p.Packet(now, ticksMS(s.tsMs), false)
		require.NoError(t, err)
		require.False(t, discarded, "packet %d should not be discarded", i+1)
		targets = append(targets, target)
	}

	require.EqualValues(t, 40*time.Millisecond, stats.NegDriftApplied.Sum,
		"the full 40ms jump must still be applied, MaxDriftCorrectionStep notwithstanding")
	require.Equal(t, uint64(1), stats.NegDriftApplied.Count,
		"negative drift must be applied in a single step, never spread across packets")

	ipds := make([]time.Duration, len(targets)-1)
	for i := 1; i < len(targets); i++ {
		ipds[i-1] = targets[i].Sub(targets[i-1])
	}

	for i, ipd := range ipds {
		require.Equal(t, nominalIPD, ipd,
			"ipd[%d]: IPD must stay nominal - spreading this correction would shrink or invert it instead", i)
	}
}

// TestPacerLogic_SpreadDrain_NotFoughtByFrontLoadedPeriod guards against a flip-flop found during development of
// MaxDriftCorrectionStep: a period whose drift is front-loaded (one large early sample) and has already subsided back
// to the old baseline by the time the period's mean correction is queued. If the positive-drift reference used to
// detect drift were advanced immediately by the full queued correction (rather than gradually, in lockstep with what
// the drain step has actually applied to baseTime), the very next nominal packet's T0 would read as spuriously
// negative relative to that prematurely-advanced reference. That triggered an immediate, uncapped negative-drift
// correction fighting the still-draining positive one - observed as either a target-time reversal (negative IPD) or,
// once fixed halfway, a packet-by-packet oscillation between two IPD values while the correction was slowly clawed
// back by repeated spurious negative-drift events, undermining the total amount actually corrected. This test pins
// down the fix: the queued correction drains smoothly and completely, with no negative-drift events at all.
func TestPacerLogic_SpreadDrain_NotFoughtByFrontLoadedPeriod(t *testing.T) {
	const delay = 500 * time.Millisecond
	const nominalIPD = 10 * time.Millisecond
	const maxStep = 2 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift:        true,
		PosDriftPeriod:         duration.Spec(60 * time.Millisecond),
		DriftThreshold:         duration.Spec(2 * time.Millisecond),
		MaxDriftCorrectionStep: duration.Duration(maxStep),
		Delay:                  duration.Spec(delay),
		ToDuration:             rtp.TicksToDuration,
	})

	T0 := utc.UnixMilli(10_000)

	type packetSpec struct {
		nowOff time.Duration
		tsMs   int
	}
	// Packet 4: one large early-period spike (100ms) that fully subsides by packet 5 (back to nominal cadence).
	// Packets 8+: perfectly nominal cadence continuing after the period-1 mean correction is queued at packet 8.
	specs := []packetSpec{
		{0, 0},
		{10 * time.Millisecond, 10},
		{20 * time.Millisecond, 20},
		{30 * time.Millisecond, -70}, // spike: t0 jumps to 100ms above baseline
		{40 * time.Millisecond, 40},  // subsides: back to baseline
		{50 * time.Millisecond, 50},
		{60 * time.Millisecond, 60},
		{70 * time.Millisecond, 70}, // period ends (70>60); mean of [0,0,0,100,0,0,0]/7 ~= 14.286ms queued
		{80 * time.Millisecond, 80},
		{90 * time.Millisecond, 90},
		{100 * time.Millisecond, 100},
		{110 * time.Millisecond, 110},
		{120 * time.Millisecond, 120},
		{130 * time.Millisecond, 130},
		{140 * time.Millisecond, 140},
	}

	targets := make([]utc.UTC, 0, len(specs))
	for i, s := range specs {
		now := T0.Add(s.nowOff)
		target, discarded, err := p.Packet(now, ticksMS(s.tsMs), false)
		require.NoError(t, err)
		require.False(t, discarded, "packet %d should not be discarded", i+1)
		targets = append(targets, target)
	}

	require.Zero(t, stats.NegDrift.Count,
		"the front-loaded-then-subsided period must never be misread as a negative-drift event")
	require.Positive(t, stats.PosDriftApplied.Count, "the queued correction must still be drained")

	ipds := make([]time.Duration, len(targets)-1)
	for i := 1; i < len(targets); i++ {
		ipds[i-1] = targets[i].Sub(targets[i-1])
	}

	// From packet 8 onward (index 6+), the drain must be smooth and monotonically bounded - nominal+step every
	// packet until fully drained, never oscillating and never dropping below nominal.
	for i := 6; i < len(ipds); i++ {
		require.GreaterOrEqual(t, ipds[i], nominalIPD, "ipd[%d]: drain must never shrink IPD below nominal", i)
		require.LessOrEqual(t, ipds[i], nominalIPD+maxStep, "ipd[%d]: drain must never exceed nominal+maxStep", i)
	}
}

// TestPacerLogic_PersistentSlowSource_StaysCaughtUp_WithSpreading is the multi-period steady-state counterpart to
// TestPacerLogic_PersistentSlowSource_StaysCaughtUp (which guards the original 394bffea ratchet regression at the
// default MaxDriftCorrectionStep=0). It runs the same persistently-10%-slow source for many periods with a sane,
// non-zero MaxDriftCorrectionStep and verifies that spreading corrections does not reintroduce a ratchet: pending
// correction must not grow across periods, and push_ahead must stay near Delay just like the unspread case.
func TestPacerLogic_PersistentSlowSource_StaysCaughtUp_WithSpreading(t *testing.T) {
	const delay = 100 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift: true,
		PosDriftPeriod:  duration.Spec(60 * time.Millisecond),
		DriftThreshold:  duration.Spec(2 * time.Millisecond),
		// A 2ms step is far above the ~1ms/packet sustained injection rate this source requires (mean drift ~7ms per
		// 6-packet period, see TestPacerLogic_PersistentSlowSource_StaysCaughtUp), so it never becomes the binding
		// constraint for this steady drift - it only bounds genuine bursts.
		MaxDriftCorrectionStep: duration.Duration(2 * time.Millisecond),
		Delay:                  duration.Spec(delay),
		ToDuration:             rtp.TicksToDuration,
	})

	T0 := utc.UnixMilli(10_000)

	// Source is 10% slow: every 10ms of wall clock, the media clock advances only 9ms. Run for many periods (400
	// packets ~= 66 periods at 60ms/period) to make sure the pending correction doesn't creep up over time.
	const packets = 400
	minPushAhead := delay
	maxIPD := time.Duration(0)
	var now utc.UTC
	var prevTarget utc.UTC
	for i := 0; i < packets; i++ {
		now = T0.Add(time.Duration(i) * 10 * time.Millisecond)
		ts := ticksMS(i * 9)
		target, discarded, err := p.Packet(now, ts, false)
		require.NoError(t, err)
		require.False(t, discarded)
		if pushAhead := target.Sub(now); pushAhead < minPushAhead {
			minPushAhead = pushAhead
		}
		if !prevTarget.IsZero() {
			if ipd := target.Sub(prevTarget); ipd > maxIPD {
				maxIPD = ipd
			}
		}
		prevTarget = target
	}

	// Same acceptance bar as the unspread regression guard: push_ahead must stay comfortably positive, not decay
	// toward the buffer-collapse behavior the 394bffea fix eliminated.
	require.Greater(t, minPushAhead, delay/2,
		"push_ahead must stay near Delay - spreading must not reintroduce the ratchet")
	require.Positive(t, stats.PosDriftApplied.Count, "positive-drift corrections must have been applied")

	// The new behavior this test adds beyond the unspread guard: no single packet's IPD exceeds nominal (9ms, the
	// source's own ts delta) plus the configured 2ms step, even though many periods' worth of corrections were
	// applied over the run.
	const nominalIPD = 9 * time.Millisecond
	require.LessOrEqual(t, maxIPD, nominalIPD+2*time.Millisecond,
		"no packet's IPD may exceed nominal+MaxDriftCorrectionStep, however many periods have elapsed")
}

// TestPacerLogic_GapDuringSpreadDrain verifies that a gap (e.g. an RTP sequence/timestamp discontinuity detected
// upstream by the gap detector and reported via Packet's gap parameter) arriving while a positive-drift correction is
// still mid-drain is treated purely as a full reset, never as drift: Packet() resets all state - including
// pendingPosCorrection and posDriftBaseline, added for spreading - before any drift detection runs for that packet, so
// no partially-applied correction or drift bookkeeping leaks across the reset. The gap packet establishes a fresh
// baseline exactly like a genuine stream start, and subsequent packets see clean nominal IPD immediately, with no
// residual effect from the abandoned drain.
func TestPacerLogic_GapDuringSpreadDrain(t *testing.T) {
	const delay = 500 * time.Millisecond
	const maxStep = 3 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift:        true,
		PosDriftPeriod:         duration.Spec(60 * time.Millisecond),
		DriftThreshold:         duration.Spec(2 * time.Millisecond),
		MaxDriftCorrectionStep: duration.Duration(maxStep),
		Delay:                  duration.Spec(delay),
		ToDuration:             rtp.TicksToDuration,
	})

	T0 := utc.UnixMilli(10_000)
	now := T0
	ts := ticksMS(0)

	// Packets 1-8: same steady 20% slow source as TestPacerLogic_DriftCorrectionSpread_ConstantPositiveDrift. Packet 8
	// queues a 6ms correction that starts draining at maxStep=3ms/packet (so packet 8 applies 3ms, leaving 3ms pending
	// for packet 9).
	for i := 1; i <= 8; i++ {
		if i > 1 {
			now = now.Add(10 * time.Millisecond)
			ts += ticksMS(8)
		}
		_, discard, err := p.Packet(now, ts, false)
		require.NoError(t, err)
		require.False(t, discard, "packet %d should not be discarded", i)
	}
	require.EqualValues(t, 3*time.Millisecond, stats.PosDriftApplied.Sum,
		"only the first 3ms step must have drained before the gap")

	// Packet 9: a gap arrives mid-drain (e.g. an SSRC/stream restart), with a disjoint RTP timestamp.
	now = now.Add(10 * time.Millisecond)
	gapTarget, gapDiscard, err := p.Packet(now, ticksMS(0), true)
	require.NoError(t, err)
	require.False(t, gapDiscard)

	require.Equal(t, 1, stats.StreamResets, "the gap must be recorded as a stream reset")
	require.Zero(t, stats.NegDrift.Count, "a gap must never be misread as negative drift")
	require.Zero(t, stats.PosDrift.Count, "a gap must never be misread as positive drift")
	require.Zero(t, stats.PosDriftApplied.Count,
		"the abandoned drain's stats must not survive the reset (per-session stats are cleared on gap)")
	require.Equal(t, now.Add(delay), gapTarget,
		"the gap packet must establish a fresh baseline exactly like a genuine stream start, unaffected by the "+
			"abandoned drain")

	// Packets 10-12: clean continuation after the gap, no residual disruption from the abandoned drain.
	prev := gapTarget
	for i := 0; i < 3; i++ {
		now = now.Add(10 * time.Millisecond)
		target, discard, err := p.Packet(now, ticksMS(10*(i+1)), false)
		require.NoError(t, err)
		require.False(t, discard)
		require.Equal(t, 10*time.Millisecond, target.Sub(prev), "IPD must be clean and nominal right after the gap")
		prev = target
	}
	require.Zero(t, stats.NegDrift.Count, "no spurious drift after the gap either")
	require.Zero(t, stats.PosDriftApplied.Count, "no spurious drift after the gap either")
}

// TestPacerLogic_GapDuringSpreadDrain_WithDiscardPeriod is the DiscardPeriod>0 counterpart to
// TestPacerLogic_GapDuringSpreadDrain (which runs at DiscardPeriod=0, matching production's discard_period: 0s). It
// confirms that when a gap restarts the discard phase - so the gap packet itself is discarded and the new baseline is
// only established a few packets later, from a discard.T0 that may carry its own StartupT0Correction - the drain
// abandoned mid-flight still leaves no trace: no spurious drift is attributed to the gap or to the packets discarded
// while re-establishing the baseline, and the eventual new baseline is clean.
func TestPacerLogic_GapDuringSpreadDrain_WithDiscardPeriod(t *testing.T) {
	const delay = 500 * time.Millisecond
	const maxStep = 3 * time.Millisecond
	p, stats := newTestPacerLogicFull(pacer.PacerLogicConfig{
		AdjustTimeDrift:        true,
		DiscardPeriod:          duration.Spec(15 * time.Millisecond),
		MaxDiscardPeriod:       duration.Spec(100 * time.Millisecond),
		PosDriftPeriod:         duration.Spec(30 * time.Millisecond),
		DriftThreshold:         duration.Spec(2 * time.Millisecond),
		MaxDriftCorrectionStep: duration.Duration(maxStep),
		Delay:                  duration.Spec(delay),
		ToDuration:             rtp.TicksToDuration,
	})

	T0 := utc.UnixMilli(10_000)
	now := T0
	ts := ticksMS(0)

	// Packets 1-8: packets 1-2 are discarded (15ms discard period), baseline establishes at packet 3; a 20% slow
	// source then queues a 7ms correction at packet 7, which starts draining at maxStep=3ms/packet. By packet 8, only
	// 6ms has drained (3+3), leaving 1ms still pending when the gap arrives.
	for i := 1; i <= 8; i++ {
		if i > 1 {
			now = now.Add(10 * time.Millisecond)
			ts += ticksMS(8)
		}
		_, _, err := p.Packet(now, ts, false)
		require.NoError(t, err)
	}
	require.EqualValues(t, 6*time.Millisecond, stats.PosDriftApplied.Sum, "1ms of the 7ms correction must still be pending")

	// Packet 9: a gap arrives mid-drain. With DiscardPeriod>0, the gap packet itself is discarded (it restarts the
	// discard phase) rather than immediately establishing a new baseline.
	now = now.Add(10 * time.Millisecond)
	_, gapDiscard, err := p.Packet(now, ticksMS(0), true)
	require.NoError(t, err)
	require.True(t, gapDiscard, "the gap packet must re-enter the discard phase, not establish a baseline directly")

	require.Equal(t, 1, stats.StreamResets, "the gap must be recorded as a stream reset")
	require.Zero(t, stats.NegDrift.Count, "a gap must never be misread as negative drift")
	require.Zero(t, stats.PosDrift.Count, "a gap must never be misread as positive drift")
	require.Zero(t, stats.PosDriftApplied.Count, "the abandoned drain's stats must not survive the reset")

	// Packet 10: still within the new 15ms discard period (10ms elapsed since the gap re-anchored discard.T0).
	now = now.Add(10 * time.Millisecond)
	_, discard, err := p.Packet(now, ticksMS(10), false)
	require.NoError(t, err)
	require.True(t, discard, "still within the post-gap discard period")

	// Packet 11: discard period elapsed (20ms since re-anchoring) - fresh baseline established.
	now = now.Add(10 * time.Millisecond)
	target, discard, err := p.Packet(now, ticksMS(20), false)
	require.NoError(t, err)
	require.False(t, discard)
	require.Equal(t, now.Add(delay), target,
		"the post-gap baseline must be clean - no residual effect from the abandoned pre-gap drain")

	// Packets 12-14: clean nominal continuation.
	prev := target
	for i := 1; i <= 3; i++ {
		now = now.Add(10 * time.Millisecond)
		target, discard, err = p.Packet(now, ticksMS(20+10*i), false)
		require.NoError(t, err)
		require.False(t, discard)
		require.Equal(t, 10*time.Millisecond, target.Sub(prev), "IPD must be clean and nominal after the gap")
		prev = target
	}
	require.Zero(t, stats.NegDrift.Count, "no spurious drift after the gap either")
	require.Zero(t, stats.PosDrift.Count, "no spurious drift after the gap either")
}
