package rtp

import (
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
	s.Seq = 42
	s.Sequ = 1042
	s.Ts = 90000
	s.Tsu = 190000

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
	require.Equal(t, uint16(0), s.Seq)
	require.Equal(t, int64(0), s.Sequ)
	require.Equal(t, uint32(0), s.Ts)
	require.Equal(t, int64(0), s.Tsu)
}

func TestOutStats_switchPeriod(t *testing.T) {
	now := utc.Now()
	s := newOutStats(duration.Spec(time.Second))

	// Add one observation to each collector
	s.wait.UpdateNow(now, duration.Millis(10*time.Millisecond))
	s.ipd.UpdateNow(now, duration.Millis(20*time.Millisecond))
	s.chd.UpdateNow(now, duration.Millis(30*time.Millisecond))
	s.lateness.UpdateNow(now, duration.Millis(40*time.Millisecond))
	s.sendAhead.UpdateNow(now, duration.Millis(50*time.Millisecond))
	s.oversleeps.UpdateNow(now, duration.Millis(60*time.Millisecond))
	s.bufFill.UpdateNow(now, int32(8))

	// Set per-period counters
	s.delayedPackets = 5
	s.sleeps = 7
	s.buffered.Store(12)

	p := s.switchPeriod(now.Add(time.Second))

	// Each collector must have exactly one observation with the correct value
	require.Equal(t, uint64(1), p.Wait.Count)
	require.Equal(t, duration.Millis(10*time.Millisecond), p.Wait.Min)

	require.Equal(t, uint64(1), p.IPD.Count)
	require.Equal(t, duration.Millis(20*time.Millisecond), p.IPD.Min)

	require.Equal(t, uint64(1), p.CHD.Count)
	require.Equal(t, duration.Millis(30*time.Millisecond), p.CHD.Min)

	require.Equal(t, uint64(1), p.Lateness.Count)
	require.Equal(t, duration.Millis(40*time.Millisecond), p.Lateness.Min)

	require.Equal(t, uint64(1), p.SendAhead.Count)
	require.Equal(t, duration.Millis(50*time.Millisecond), p.SendAhead.Min)

	require.Equal(t, uint64(1), p.OverSleeps.Count)
	require.Equal(t, duration.Millis(60*time.Millisecond), p.OverSleeps.Min)

	require.Equal(t, uint64(1), p.BufFill.Count)
	require.Equal(t, int32(8), p.BufFill.Min)

	// Plain counters and buffered snapshot
	require.Equal(t, 5, p.DelayedPackets)
	require.Equal(t, 7, p.Sleeps)
	require.Equal(t, int32(12), p.BufferedPackets)

	// Per-period counters must be reset after switchPeriod
	require.Equal(t, 0, s.delayedPackets)
	require.Equal(t, 0, s.sleeps)

	// A second switchPeriod with no new observations must return an empty snapshot
	p2 := s.switchPeriod(now.Add(2 * time.Second))
	require.Equal(t, uint64(0), p2.Wait.Count)
	require.Equal(t, uint64(0), p2.IPD.Count)
	require.Equal(t, uint64(0), p2.CHD.Count)
	require.Equal(t, uint64(0), p2.Lateness.Count)
	require.Equal(t, uint64(0), p2.SendAhead.Count)
	require.Equal(t, uint64(0), p2.OverSleeps.Count)
	require.Equal(t, uint64(0), p2.BufFill.Count)
}
