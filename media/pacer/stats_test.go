package pacer

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/utc-go"
)

func TestInStats_Reset(t *testing.T) {
	var s InStats
	now := utc.Now()

	// Populate all RawStatistics fields
	s.PushAhead.Update(now, duration.Millis(10*time.Millisecond))
	s.StartupT0Correction.Update(now, duration.Millis(5*time.Millisecond))
	s.NegDrift.Update(now, duration.Millis(3*time.Millisecond))
	s.NegDriftApplied.Update(now, duration.Millis(2*time.Millisecond))
	s.PosDrift.Update(now, duration.Millis(4*time.Millisecond))
	s.PosDriftApplied.Update(now, duration.Millis(1*time.Millisecond))

	// Populate scalar fields
	s.MinT0 = now
	s.StreamResets = 3
	s.LastStreamReset = now.Add(-time.Minute)
	s.Rtp.Seq = 42
	s.Rtp.Sequ = 1042
	s.Rtp.Ts = 90000
	s.Rtp.Tsu = 190000

	lastReset := s.LastStreamReset
	s.Reset()

	// Lifetime counters must be preserved
	require.Equal(t, 3, s.StreamResets)
	require.Equal(t, lastReset, s.LastStreamReset)

	// All RawStatistics must be zeroed (Count == 0 is the canonical zero check)
	require.Equal(t, uint64(0), s.PushAhead.Count)
	require.Equal(t, uint64(0), s.StartupT0Correction.Count)
	require.Equal(t, uint64(0), s.NegDrift.Count)
	require.Equal(t, uint64(0), s.NegDriftApplied.Count)
	require.Equal(t, uint64(0), s.PosDrift.Count)
	require.Equal(t, uint64(0), s.PosDriftApplied.Count)

	// Scalar fields must be cleared
	require.True(t, s.MinT0.IsZero())
	require.Equal(t, RtpInStats{}, s.Rtp)
}

// TestInStats_Marshal verifies that InStats marshals and unmarshals correctly, and that RtpInStats and TsInStats
// fields are correctly included in/excluded from the JSON output depending on which pacer type populated them.
func TestInStats_Marshal(t *testing.T) {
	t.Run("RtpOnly", func(t *testing.T) {
		var s InStats
		s.Rtp.Seq = 1234
		s.Rtp.Sequ = 56789
		s.Rtp.Ts = 90000
		s.Rtp.Tsu = 190000

		b, err := json.Marshal(s)
		require.NoError(t, err)
		require.Contains(t, string(b), `"rtp":{`)
		require.NotContains(t, string(b), `"ts":{`)

		var got InStats
		require.NoError(t, json.Unmarshal(b, &got))
		require.Equal(t, s.Rtp, got.Rtp)
		require.Equal(t, TsInStats{}, got.Ts)
	})

	t.Run("TsOnly", func(t *testing.T) {
		var s InStats
		s.Ts.PCR = 27_000_000
		s.Ts.PCRu = 54_000_000
		s.Ts.PID = 256

		b, err := json.Marshal(s)
		require.NoError(t, err)
		require.NotContains(t, string(b), `"rtp":{`)
		require.Contains(t, string(b), `"ts":{`)

		var got InStats
		require.NoError(t, json.Unmarshal(b, &got))
		require.Equal(t, RtpInStats{}, got.Rtp)
		require.Equal(t, s.Ts, got.Ts)
	})

	t.Run("CommonFields", func(t *testing.T) {
		now := utc.MustParse("2000-01-01T12:00:00Z")
		var s InStats
		s.PushAhead.Update(now, duration.Millis(50*time.Millisecond))
		s.NegDrift.Update(now, duration.Millis(3*time.Millisecond))
		s.StreamResets = 2
		s.LastStreamReset = now
		s.MinT0 = now.Add(-time.Second)

		b, err := json.Marshal(s)
		require.NoError(t, err)

		var got InStats
		require.NoError(t, json.Unmarshal(b, &got))
		// RawStatistics has unexported fields that don't survive JSON — compare the serialized values only.
		require.Equal(t, uint64(1), got.PushAhead.Count)
		require.Equal(t, duration.Millis(50*time.Millisecond), got.PushAhead.Min)
		require.Equal(t, uint64(1), got.NegDrift.Count)
		require.Equal(t, duration.Millis(3*time.Millisecond), got.NegDrift.Min)
		require.Equal(t, s.StreamResets, got.StreamResets)
		require.Equal(t, s.LastStreamReset, got.LastStreamReset)
		require.Equal(t, s.MinT0, got.MinT0)
		require.Equal(t, RtpInStats{}, got.Rtp)
		require.Equal(t, TsInStats{}, got.Ts)
	})
}

func TestOutStats_switchPeriod(t *testing.T) {
	now := utc.Now()
	s := newOutStats(duration.Spec(time.Second))

	// Add one observation to each collector
	s.wait.UpdateNow(now, duration.Millis(10*time.Millisecond))
	s.ipd.UpdateNow(now, duration.Millis(20*time.Millisecond))
	s.jbd.UpdateNow(now, duration.Millis(30*time.Millisecond))
	s.lateness.UpdateNow(now, duration.Millis(40*time.Millisecond))
	s.sendAhead.UpdateNow(now, duration.Millis(50*time.Millisecond))
	s.oversleeps.UpdateNow(now, duration.Millis(60*time.Millisecond))
	s.bufFill.UpdateNow(now, int32(8))

	// Set per-period counters
	s.sleeps = 7
	s.buffered.Store(12)

	p := s.switchPeriod(now.Add(time.Second))

	// Each collector must have exactly one observation with the correct value
	require.Equal(t, uint64(1), p.Wait.Count)
	require.Equal(t, duration.Millis(10*time.Millisecond), p.Wait.Min)

	require.Equal(t, uint64(1), p.IPD.Count)
	require.Equal(t, duration.Millis(20*time.Millisecond), p.IPD.Min)

	require.Equal(t, uint64(1), p.JBD.Count)
	require.Equal(t, duration.Millis(30*time.Millisecond), p.JBD.Min)

	require.Equal(t, uint64(1), p.Lateness.Count)
	require.Equal(t, duration.Millis(40*time.Millisecond), p.Lateness.Min)

	require.Equal(t, uint64(1), p.SendAhead.Count)
	require.Equal(t, duration.Millis(50*time.Millisecond), p.SendAhead.Min)

	require.Equal(t, uint64(1), p.OverSleeps.Count)
	require.Equal(t, duration.Millis(60*time.Millisecond), p.OverSleeps.Min)

	require.Equal(t, uint64(1), p.BufFill.Count)
	require.Equal(t, int32(8), p.BufFill.Min)

	// Plain counters and buffered snapshot
	require.Equal(t, 7, p.Sleeps)
	require.Equal(t, int32(12), p.BufferedPackets)

	// Per-period counters must be reset after switchPeriod
	require.Equal(t, 0, s.sleeps)

	// A second switchPeriod with no new observations must return an empty snapshot
	p2 := s.switchPeriod(now.Add(2 * time.Second))
	require.Equal(t, uint64(0), p2.Wait.Count)
	require.Equal(t, uint64(0), p2.IPD.Count)
	require.Equal(t, uint64(0), p2.JBD.Count)
	require.Equal(t, uint64(0), p2.Lateness.Count)
	require.Equal(t, uint64(0), p2.SendAhead.Count)
	require.Equal(t, uint64(0), p2.OverSleeps.Count)
	require.Equal(t, uint64(0), p2.BufFill.Count)
}
