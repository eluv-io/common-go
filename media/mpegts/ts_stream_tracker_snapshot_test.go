package mpegts

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/eluv-io/utc-go"
)

// TestTsStreamTracker_Snapshot_ReusesStreamsInPlace verifies that calling Snapshot repeatedly with the same
// destination reuses each *StreamStats (and its JitterMillisHist) in place rather than reallocating, as long as the
// PID set and order are stable across calls - the common case for a live stream. The synthetic makeTsBatch fixture
// doesn't produce continuity-counter-clean packet sequences (see other tests in this package), so Track's error
// return is intentionally ignored here - only packet counts and *StreamStats identity are under test.
func TestTsStreamTracker_Snapshot_ReusesStreamsInPlace(t *testing.T) {
	tr := NewTsStreamTracker("test", 0, false)
	_, _ = tr.Track(makeTsBatch(100, 1_000_000, 3))
	_, _ = tr.Track(makeTsBatch(200, 2_000_000, 2))

	snap := &Stats{}
	tr.Snapshot(snap, true)
	require.Len(t, snap.Streams, 2)
	first0, first1 := snap.Streams[0], snap.Streams[1]
	require.Equal(t, 100, first0.Pid)
	require.Equal(t, 200, first1.Pid)

	_, _ = tr.Track(makeTsBatch(100, 3_000_000, 3))
	_, _ = tr.Track(makeTsBatch(200, 4_000_000, 2))

	tr.Snapshot(snap, true)
	require.Len(t, snap.Streams, 2)
	require.Same(t, first0, snap.Streams[0], "same PID at the same position reuses the *StreamStats")
	require.Same(t, first1, snap.Streams[1])
	require.EqualValues(t, 6, snap.Streams[0].PacketCount, "values are updated in place, not stale")
	require.EqualValues(t, 4, snap.Streams[1].PacketCount)
}

// TestTsStreamTracker_Snapshot_NewPidAppears verifies Snapshot handles a PID set that grows between calls (a new
// elementary stream is discovered mid-capture) without corrupting previously-reused entries.
func TestTsStreamTracker_Snapshot_NewPidAppears(t *testing.T) {
	tr := NewTsStreamTracker("test", 0, false)
	_, _ = tr.Track(makeTsBatch(100, 1_000_000, 3))

	snap := &Stats{}
	tr.Snapshot(snap, true)
	require.Len(t, snap.Streams, 1)

	_, _ = tr.Track(makeTsBatch(200, 2_000_000, 5))

	tr.Snapshot(snap, true)
	require.Len(t, snap.Streams, 2)
	require.Equal(t, 100, snap.Streams[0].Pid)
	require.Equal(t, 200, snap.Streams[1].Pid)
	require.EqualValues(t, 5, snap.Streams[1].PacketCount)
}

// TestTsStreamTracker_Snapshot_NotFull verifies that full=false skips the per-PID walk (Streams stays empty) while
// still reporting the cheap running totals (PacketCount from every stream, ErrorCount including CC mismatches).
func TestTsStreamTracker_Snapshot_NotFull(t *testing.T) {
	tr := NewTsStreamTracker("test", 0, false)
	// n=3 packets on one PID: makeTsBatch's packets don't form a continuity-clean sequence, so this alone already
	// produces CC errors - exactly what this test wants to confirm is visible without the per-PID walk.
	_, err := tr.Track(makeTsBatch(100, 1_000_000, 3))
	require.Error(t, err)

	full := &Stats{}
	tr.Snapshot(full, true)

	lean := &Stats{}
	tr.Snapshot(lean, false)

	require.Empty(t, lean.Streams, "full=false skips the per-PID walk")
	require.Equal(t, full.PacketCount, lean.PacketCount, "PacketCount is available cheaply either way")
	require.Equal(t, full.ErrorCount, lean.ErrorCount, "ErrorCount is available cheaply either way")
	require.NotZero(t, lean.ErrorCount)
}

// TestTsStreamTracker_Snapshot_ResetClearsRunningTotals verifies Reset also clears the incrementally-maintained
// totalCcErrors/totalPacketCount used by Snapshot(full=false), not just the per-stream counters Stats() derives from.
func TestTsStreamTracker_Snapshot_ResetClearsRunningTotals(t *testing.T) {
	tr := NewTsStreamTracker("test", 0, false)
	_, _ = tr.Track(makeTsBatch(100, 1_000_000, 3))

	tr.Reset()

	lean := &Stats{}
	tr.Snapshot(lean, false)
	require.Zero(t, lean.PacketCount)
	require.Zero(t, lean.ErrorCount)
}

// TestStats_CopyInto verifies CopyInto produces a deeply independent copy - mutating the source after copying must
// not affect the destination - and reuses the destination's existing Streams/JitterMillisHist where already shaped
// to match, rather than reallocating.
func TestStats_CopyInto(t *testing.T) {
	tr := NewTsStreamTracker("test", 0, false)
	_, _ = tr.Track(makeTsBatch(100, 1_000_000, 3))

	src := &Stats{}
	tr.Snapshot(src, true)

	dst := &Stats{}
	src.CopyInto(dst)
	require.Len(t, dst.Streams, 1)
	require.NotSame(t, src.Streams[0], dst.Streams[0], "CopyInto must not alias the source's *StreamStats")
	require.Equal(t, src.Streams[0].PacketCount, dst.Streams[0].PacketCount)
	dstStream0 := dst.Streams[0]

	// Mutate the source (as a live tracker reusing src via Snapshot would) and confirm dst is unaffected.
	_, _ = tr.Track(makeTsBatch(100, 2_000_000, 4))
	tr.Snapshot(src, true)
	require.NotEqual(t, src.Streams[0].PacketCount, dst.Streams[0].PacketCount,
		"dst must not have changed just because src was refreshed")

	// A second CopyInto into the same dst reuses the existing *StreamStats in place.
	src.CopyInto(dst)
	require.Same(t, dstStream0, dst.Streams[0], "destination reuse: same *StreamStats, updated in place")
	require.Equal(t, src.Streams[0].PacketCount, dst.Streams[0].PacketCount)
}

// TestStats_CopyInto_JitterMillisHist verifies the JitterMillisHist pointer itself is never shared between source
// and destination, only its values.
func TestStats_CopyInto_JitterMillisHist(t *testing.T) {
	src := &Stats{Streams: []*StreamStats{{Pid: 100, JitterMillisHist: &HistogramCapture{Mean: 42}}}}
	dst := &Stats{}

	src.CopyInto(dst)
	require.NotNil(t, dst.Streams[0].JitterMillisHist)
	require.NotSame(t, src.Streams[0].JitterMillisHist, dst.Streams[0].JitterMillisHist)
	require.Equal(t, 42.0, dst.Streams[0].JitterMillisHist.Mean)

	src.Streams[0].JitterMillisHist.Mean = 99
	require.Equal(t, 42.0, dst.Streams[0].JitterMillisHist.Mean, "dst's copy is unaffected by mutating src's")
}

// TestStats_CopyInto_Pcr0 is a regression test for CopyInto allocating a fresh *utc.UTC for Pcr0 on every call instead
// of reusing dst's existing allocation. Verifies both that dst's copy is independent of src's (mutating src afterward
// must not affect dst) and that a second CopyInto into the same dst reuses the same *utc.UTC in place.
func TestStats_CopyInto_Pcr0(t *testing.T) {
	t0 := utc.Now()
	src := &Stats{Streams: []*StreamStats{{Pid: 100, Pcr0: &t0}}}
	dst := &Stats{}

	src.CopyInto(dst)
	require.NotNil(t, dst.Streams[0].Pcr0)
	require.NotSame(t, src.Streams[0].Pcr0, dst.Streams[0].Pcr0, "CopyInto must not alias the source's Pcr0")
	require.Equal(t, t0, *dst.Streams[0].Pcr0)
	dstPcr0 := dst.Streams[0].Pcr0

	t1 := t0.Add(1)
	src.Streams[0].Pcr0 = &t1
	require.Equal(t, t0, *dst.Streams[0].Pcr0, "dst's copy is unaffected by mutating src's")

	src.CopyInto(dst)
	require.Same(t, dstPcr0, dst.Streams[0].Pcr0, "destination reuse: same *utc.UTC, updated in place")
	require.Equal(t, t1, *dst.Streams[0].Pcr0)
}
