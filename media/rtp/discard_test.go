package rtp_test

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/utc-go"
)

func TestDiscardContext_Disabled(t *testing.T) {
	now := utc.Now()
	dc := rtp.NewDiscardContext(0, 0)
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
	dc := rtp.NewDiscardContext(5*duration.Second, 10*duration.Second)

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
	dc := rtp.NewDiscardContext(5*duration.Second, 10*duration.Second)

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
	dc := rtp.NewDiscardContext(5*duration.Second, 9*duration.Second)

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

	// next packet is outside max discard period, so ShouldDiscard returns an error
	now = now.Add(time.Second)
	assertDiscard(t, dc, now.Sub(t0), now, true, true)
}

func TestDiscardContext_ResetOnGapDuringNormalOperation(t *testing.T) {
	now := utc.MustParse("2000-01-01T12:00:00Z")
	t0 := now
	dc := rtp.NewDiscardContext(5*duration.Second, 9*duration.Second)

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

func assertDiscard(t *testing.T, dc *rtp.DiscardContext, rtpTs time.Duration, now utc.UTC, wantDiscard bool, wantErr bool) {
	discard, err := dc.ShouldDiscard(rtp.DurationToTicks(rtpTs), now)
	require.Equal(t, wantDiscard, discard, "discard mismatch")
	require.Equal(t, wantErr, err != nil, "discard error")
}
