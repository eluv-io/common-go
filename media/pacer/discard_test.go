package pacer_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/utc-go"
)

func TestDiscardContext_Disabled(t *testing.T) {
	now := utc.Now()
	dc := pacer.NewDiscardContext(0, 0, rtp.TicksToDuration)
	discard, err := dc.ShouldDiscard(0, now)
	require.NoError(t, err)
	require.False(t, discard)
	require.Equal(t, now, dc.T0)                            // T0 should be set even though discarding is disabled
	require.EqualValues(t, 0, dc.StartupT0Correction.Count) // no adjustments, however

	discard, err = dc.ShouldDiscard(1, now.Add(time.Millisecond))
	require.NoError(t, err)
	require.False(t, discard)
	require.Equal(t, now, dc.T0)                            // T0 should be set even though discarding is disabled
	require.EqualValues(t, 0, dc.StartupT0Correction.Count) // no adjustments, however
}

func TestDiscardContext_ShouldDiscard(t *testing.T) {
	now := utc.MustParse("2000-01-01T12:00:00Z")
	dc := pacer.NewDiscardContext(5*duration.Second, 10*duration.Second, rtp.TicksToDuration)

	seq := int64(rand.Int32())
	t0 := now.Add(-rtp.TicksToDuration(seq))

	for i := 0; i < 10; i++ {
		discard, err := dc.ShouldDiscard(seq, now)
		if i < 5 {
			require.True(t, discard, "packet %d", i)
			require.NoError(t, err, "packet %d", i)
		} else {
			require.False(t, discard, "packet %d", i)
			require.NoError(t, err, "packet %d", i)
		}
		now = now.Add(time.Second) // advance time by on second
		seq += 90000               // advance seq by 90k ticks = 1 second ==> no T0 adjustment
	}
	require.Equal(t, t0, dc.T0)
	require.EqualValues(t, 0, dc.StartupT0Correction.Count)
	require.EqualValues(t, 0, dc.StartupT0Correction.Sum)
}

func TestDiscardContext_ShouldDiscardWithAdjustment(t *testing.T) {
	now := utc.MustParse("2000-01-01T12:00:00Z")
	dc := pacer.NewDiscardContext(5*duration.Second, 10*duration.Second, rtp.TicksToDuration)

	seq := int64(rand.Int32())
	t0 := now.Add(-rtp.TicksToDuration(seq))

	for i := 0; i < 10; i++ {
		var jitter time.Duration
		if i == 2 {
			// 2nd packet arrives 5ms early, which should cause T0 to be adjusted backwards by 5ms, and startup discard
			// period to be extended by 2 seconds
			jitter = -5 * time.Millisecond
		}
		discard, err := dc.ShouldDiscard(seq, now.Add(jitter))
		if i < 7 {
			require.True(t, discard, "packet %d", i)
			require.NoError(t, err, "packet %d", i)
		} else {
			require.False(t, discard, "packet %d", i)
			require.NoError(t, err, "packet %d", i)
		}
		now = now.Add(time.Second) // advance time by on second
		seq += 90000               // advance seq by 90k ticks = 1 second ==> no T0 adjustment
	}
	require.Equal(t, t0.Add(-5*time.Millisecond), dc.T0)
	require.EqualValues(t, 1, dc.StartupT0Correction.Count)
	require.EqualValues(t, 5*time.Millisecond, dc.StartupT0Correction.Sum)
}

func TestDiscardContext_ResetOnGapDuringDiscardPhase(t *testing.T) {
	now := utc.MustParse("2000-01-01T12:00:00Z")
	t0 := now
	dc := pacer.NewDiscardContext(5*duration.Second, 9*duration.Second, rtp.TicksToDuration)

	for j := 0; j < 3; j++ {
		for i := 0; i < 3; i++ {
			assertDiscard(t, dc, now.Sub(t0), now, true, false)
			now = now.Add(time.Second)
		}
		// signal an RTP gap during discard phase
		dc.ResetOnGap()
	}

	// last packet during discard phase
	assertDiscard(t, dc, now.Sub(t0), now, true, false)

	// The next packet is outside the max discard period. This stream's T0 improves on every packet, so it would
	// otherwise never converge - the cap gives up and completes the phase with the best baseline found, rather than
	// failing a stream that is merely never settling.
	now = now.Add(time.Second)
	assertDiscard(t, dc, now.Sub(t0), now, false, false)
	require.True(t, dc.DiscardComplete)
}

func TestDiscardContext_ResetOnGapDuringNormalOperation(t *testing.T) {
	now := utc.MustParse("2000-01-01T12:00:00Z")
	t0 := now
	dc := pacer.NewDiscardContext(5*duration.Second, 9*duration.Second, rtp.TicksToDuration)

	for j := 0; j < 3; j++ {
		for i := 0; i < 5; i++ {
			assertDiscard(t, dc, now.Sub(t0), now, true, false)
			now = now.Add(time.Second)
		}
		// discard phase over
		assertDiscard(t, dc, now.Sub(t0), now, false, false)
		require.Equal(t, t0, dc.T0)
		require.EqualValues(t, 0, dc.StartupT0Correction.Count)
		// signal an RTP gap outside of discard period --> resets everything, starting a new discard phase
		dc.ResetOnGap()
	}
}

// TestDiscardContext_T0Threshold checks the jitter dead-band: an improvement in T0 always refines the baseline, but
// only one large enough to mean the reader is still catching up restarts the discard period. Without it, the running
// minimum of a jittery stream keeps creeping down and holds the phase open until the cap.
func TestDiscardContext_T0Threshold(t *testing.T) {
	now := utc.MustParse("2000-01-01T12:00:00Z")
	dc := pacer.NewDiscardContext(duration.Spec(time.Second), duration.Spec(time.Minute), rtp.TicksToDuration)
	dc.T0Threshold = duration.Spec(50 * time.Millisecond)

	// A packet whose timestamp matches its arrival puts T0 at the stream's start; keep that as the reference.
	assertDiscard(t, dc, 0, now, true, false)
	baseT0 := dc.T0

	// Sub-threshold improvements: the baseline follows them, but the period keeps running. Stream time runs 110ms per
	// 100ms of wall time, so each packet lands 10ms earlier in T0 terms than the one before.
	var rtpTs time.Duration
	for i := 0; i < 5; i++ {
		now = now.Add(100 * time.Millisecond)
		rtpTs += 110 * time.Millisecond
		assertDiscard(t, dc, rtpTs, now, true, false)
		baseT0 = baseT0.Add(-10 * time.Millisecond)
		require.Equal(t, baseT0, dc.T0, "baseline must still take a sub-threshold improvement")
	}
	require.EqualValues(t, 5, dc.StartupT0Correction.Count)

	// The period has now elapsed measured from the first packet, and no significant improvement has restarted it, so
	// the next non-improving packet completes the phase.
	now = now.Add(600 * time.Millisecond)
	assertDiscard(t, dc, 1100*time.Millisecond, now, false, false)
	require.True(t, dc.DiscardComplete, "sub-threshold improvements must not hold the phase open")

	// A super-threshold improvement, by contrast, restarts the period.
	dc = pacer.NewDiscardContext(duration.Spec(time.Second), duration.Spec(time.Minute), rtp.TicksToDuration)
	dc.T0Threshold = duration.Spec(50 * time.Millisecond)
	now = utc.MustParse("2000-01-01T12:00:00Z")
	assertDiscard(t, dc, 0, now, true, false)

	now = now.Add(900 * time.Millisecond)
	assertDiscard(t, dc, time.Second, now, true, false) // 100ms early: a real catch-up step
	restartedAt := dc.T0UpdatedAt
	require.Equal(t, now, restartedAt, "a super-threshold improvement must restart the period")

	// Still inside the restarted period, so the phase is not yet over.
	now = now.Add(500 * time.Millisecond)
	assertDiscard(t, dc, time.Second+500*time.Millisecond, now, true, false)
	require.False(t, dc.DiscardComplete)
}

// TestDiscardContext_T0ThresholdCompletesWhileStillImproving covers the case the dead-band actually exists for: a
// jittery stream whose running-minimum T0 keeps creeping down, so *every* packet is an improvement. Sub-threshold
// improvements must not be able to hold the phase open between them - the period has to elapse from the last
// significant improvement and complete the phase, well before the cap.
func TestDiscardContext_T0ThresholdCompletesWhileStillImproving(t *testing.T) {
	now := utc.MustParse("2000-01-01T12:00:00Z")
	dc := pacer.NewDiscardContext(duration.Spec(time.Second), duration.Spec(time.Minute), rtp.TicksToDuration)
	dc.T0Threshold = duration.Spec(50 * time.Millisecond)

	assertDiscard(t, dc, 0, now, true, false)

	// Stream time runs 101ms per 100ms of wall time, so every packet improves T0 by 1ms - always sub-threshold, and
	// never stopping. The phase must still complete once the period has elapsed.
	var rtpTs time.Duration
	completedAt := -1
	for i := range 30 {
		now = now.Add(100 * time.Millisecond)
		rtpTs += 101 * time.Millisecond
		discard, err := dc.ShouldDiscard(rtp.DurationToTicks(rtpTs), now)
		require.NoError(t, err)
		if !discard {
			completedAt = i
			break
		}
	}

	require.True(t, dc.DiscardComplete, "a continuously improving sub-threshold stream must not run to the cap")
	// The period is 1s and packets arrive every 100ms, so completion is due on the 10th packet after the baseline.
	require.Equal(t, 9, completedAt, "phase must complete one period after the last significant improvement")
	require.Less(t, now.Sub(dc.FirstPacketTime), time.Minute, "must complete well inside the cap")
}

// TestDiscardContext_MaxSourceChangePeriodNeverCapsBelowPeriod covers the fallback interaction: with no
// SourceChangePeriod configured the phase runs on DiscardPeriod, and a shorter MaxSourceChangePeriod must not cap it
// below that.
func TestDiscardContext_MaxSourceChangePeriodNeverCapsBelowPeriod(t *testing.T) {
	now := utc.MustParse("2000-01-01T12:00:00Z")
	dc := pacer.NewDiscardContext(duration.Spec(5*time.Second), duration.Spec(30*time.Second), rtp.TicksToDuration)
	dc.MaxSourceChangePeriod = duration.Spec(2 * time.Second) // shorter than the 5s period the phase will run on
	dc.ResetForSourceChange()

	assertDiscard(t, dc, 0, now, true, false)

	// Past the 2s cap but inside the 5s period, with no improvement to extend anything: the cap must not have fired.
	now = now.Add(3 * time.Second)
	assertDiscard(t, dc, 3*time.Second, now, true, false)
	require.False(t, dc.DiscardComplete, "the cap must not fall below the period in force")

	// Past the period, so the phase completes normally rather than via the cap.
	now = now.Add(2100 * time.Millisecond)
	assertDiscard(t, dc, 5100*time.Millisecond, now, false, false)
	require.True(t, dc.DiscardComplete)
}

// TestDiscardContext_CapKeepsFinalImprovement checks that the packet on which the cap fires still contributes its
// baseline improvement, rather than the phase completing on the previous, worse T0.
func TestDiscardContext_CapKeepsFinalImprovement(t *testing.T) {
	now := utc.MustParse("2000-01-01T12:00:00Z")
	dc := pacer.NewDiscardContext(duration.Spec(time.Second), duration.Spec(2*time.Second), rtp.TicksToDuration)

	assertDiscard(t, dc, 0, now, true, false)
	baseT0 := dc.T0

	// Past the cap, on a packet that is 500ms early - a real improvement the completed phase should keep.
	now = now.Add(2500 * time.Millisecond)
	assertDiscard(t, dc, 3*time.Second, now, false, false)
	require.True(t, dc.DiscardComplete)
	require.Equal(t, baseT0.Add(-500*time.Millisecond), dc.T0, "the cap must complete on the best baseline seen")
}

func assertDiscard(t *testing.T, dc *pacer.DiscardContext, rtpTs time.Duration, now utc.UTC, wantDiscard bool, wantErr bool) {
	discard, err := dc.ShouldDiscard(rtp.DurationToTicks(rtpTs), now)
	require.Equal(t, wantDiscard, discard, "discard mismatch")
	require.Equal(t, wantErr, err != nil, "discard error")
}
