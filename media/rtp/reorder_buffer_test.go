package rtp

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// t0 is an arbitrary fixed base time; tests advance from it by explicit offsets so nothing depends on wall time.
var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func at(ms int) time.Time {
	return t0.Add(time.Duration(ms) * time.Millisecond)
}

// TestReorderBuffer_SingleSwappedPair verifies that a single out-of-order pair (seq 6 arriving before seq 5) is
// corrected: 6 is held until 5 arrives, then both release in order.
func TestReorderBuffer_SingleSwappedPair(t *testing.T) {
	b := NewReorderBuffer[int](0, 0, 0)

	out := b.Push(at(0), 5, 500, nil)
	require.Equal(t, []int{500}, out, "the first packet ever is released immediately")

	out = b.Push(at(1), 7, 700, nil)
	require.Empty(t, out, "7 arrives ahead of expected 6 - held, not released yet")
	require.EqualValues(t, 1, b.Stats().CurrentOccupancy)

	out = b.Push(at(2), 6, 600, nil)
	require.Equal(t, []int{600, 700}, out, "6 fills the gap and cascades straight into the held 7")
	require.EqualValues(t, 1, b.Stats().Reordered)
	require.Zero(t, b.Stats().CurrentOccupancy)
	require.Zero(t, b.Stats().LostAfterTimeout)
}

// TestReorderBuffer_BurstReorder verifies a larger scrambled run within the window all releases in strictly
// ascending order once the run completes.
func TestReorderBuffer_BurstReorder(t *testing.T) {
	b := NewReorderBuffer[int](0, 0, 0)

	var out []int
	out = b.Push(at(0), 10, 10, out)
	// Arrival order: 10, 13, 11, 15, 12, 14 - a scrambled run of 10..15.
	for i, seq := range []uint16{13, 11, 15, 12, 14} {
		out = b.Push(at(i+1), seq, int(seq), out)
	}
	require.Equal(t, []int{10, 11, 12, 13, 14, 15}, out, "the whole run must emerge in strictly ascending order")
	// 11, 12 and 14 each arrive exactly when they're the awaited packet, so they release directly; 13 and 15 each
	// arrive ahead of their turn and are only released via cascade once their predecessor fills in - Reordered
	// counts releases delayed past their arrival, i.e. these two.
	require.EqualValues(t, 2, b.Stats().Reordered)
	require.Zero(t, b.Stats().CurrentOccupancy)
	require.Zero(t, b.Stats().LostAfterTimeout)
}

// TestReorderBuffer_GenuineLossTimesOut verifies that a gap which is never filled is declared lost once Expire is
// called at/after the computed deadline, and that whatever was buffered behind it is released at that point.
func TestReorderBuffer_GenuineLossTimesOut(t *testing.T) {
	maxWait := 20 * time.Millisecond
	b := NewReorderBuffer[int](0, maxWait, 0)

	out := b.Push(at(0), 1, 1, nil)
	require.Equal(t, []int{1}, out)

	out = b.Push(at(1), 3, 3, nil)
	require.Empty(t, out, "2 is missing - 3 is held")
	deadline, ok := b.Deadline()
	require.True(t, ok)
	require.Equal(t, at(1).Add(maxWait), deadline)

	// Not yet expired.
	out = b.Expire(deadline.Add(-time.Millisecond), nil)
	require.Empty(t, out)

	// Expired: 2 is declared lost, 3 cascades out.
	out = b.Expire(deadline, nil)
	require.Equal(t, []int{3}, out)
	stats := b.Stats()
	require.EqualValues(t, 1, stats.LostAfterTimeout)
	require.Zero(t, stats.CurrentOccupancy)
	_, ok = b.Deadline()
	require.False(t, ok, "window is empty again, no pending deadline")
}

// TestReorderBuffer_MixedReorderAndLoss verifies that a present-but-late packet is absorbed without triggering a
// false timeout, while a genuinely missing one still times out on schedule.
func TestReorderBuffer_MixedReorderAndLoss(t *testing.T) {
	maxWait := 20 * time.Millisecond
	b := NewReorderBuffer[int](0, maxWait, 0)

	out := b.Push(at(0), 1, 1, nil) // expected becomes 2
	require.Equal(t, []int{1}, out)

	out = b.Push(at(1), 4, 4, nil) // gap: 2,3 missing; deadline anchored at at(1)+maxWait
	require.Empty(t, out)
	deadline, _ := b.Deadline()

	out = b.Push(at(2), 3, 3, nil) // 3 present but still can't release (2 still missing)
	require.Empty(t, out)
	// deadline must be unchanged: 3's arrival doesn't touch the head-of-window gap's budget
	d2, ok := b.Deadline()
	require.True(t, ok)
	require.Equal(t, deadline, d2)

	// 2 never arrives; expire at the original deadline.
	out = b.Expire(deadline, nil)
	require.Equal(t, []int{3, 4}, out, "3 and 4 cascade out once 2 is declared lost")
	stats := b.Stats()
	require.EqualValues(t, 1, stats.LostAfterTimeout)
	require.EqualValues(t, 2, stats.Reordered, "3 and 4 both released via cascade")
}

// TestReorderBuffer_Wraparound verifies the signed-delta admission math treats a sequence straddling 65535->0 as a
// normal +1 step, not a huge jump that would misfire the resync/window-full logic.
func TestReorderBuffer_Wraparound(t *testing.T) {
	b := NewReorderBuffer[int](0, 0, 0)

	out := b.Push(at(0), 65534, 1, nil)
	require.Equal(t, []int{1}, out)

	// Out of order across the wrap: 0 arrives before 65535.
	out = b.Push(at(1), 0, 3, nil)
	require.Empty(t, out, "0 is ahead of expected 65535 (a normal +1 gap, not a huge jump)")
	require.Zero(t, b.Stats().Resyncs)

	out = b.Push(at(2), 65535, 2, nil)
	require.Equal(t, []int{2, 3}, out, "65535 fills the gap and cascades into the held wrapped 0")
	require.Zero(t, b.Stats().Resyncs)
	require.Zero(t, b.Stats().LostAfterTimeout)
}

// TestReorderBuffer_WindowFullForcesEarlyTimeout verifies that a packet arriving further ahead than maxWindow forces
// the head-of-window gap to be treated as lost (possibly repeatedly) to make room, without panicking or
// mis-ordering, and reduces to the same accounting as a real Expire.
func TestReorderBuffer_WindowFullForcesEarlyTimeout(t *testing.T) {
	b := NewReorderBuffer[int](4, time.Hour, 0) // maxWait large so only window-fullness forces this, not the timer

	out := b.Push(at(0), 0, 0, nil) // expected becomes 1
	require.Equal(t, []int{0}, out)

	// Arrives at distance 6 from expected(1) -> exceeds maxWindow(4), forcing 1,2 to be skipped (lost) before 7 can
	// be admitted (expected becomes 3, distance 7-3=4 == maxWindow, fits: maxWindow bounds how far ahead the buffer
	// will hold an item *inclusive* of maxWindow itself - only strictly farther forces expiry).
	out = b.Push(at(1), 7, 7, nil)
	require.Empty(t, out, "7 itself is buffered, not released - the gap 3..6 is still open")
	stats := b.Stats()
	require.EqualValues(t, 2, stats.LostAfterTimeout, "1 and 2 were forced out to make room")
	require.EqualValues(t, 1, stats.CurrentOccupancy)
	require.EqualValues(t, 6, stats.MaxReorderDelta,
		"must reflect the true arrival distance (6), not the distance left over after the window-full loop shrank it")

	// Filling 3,4,5,6 must cascade 7 out too.
	out = b.Push(at(2), 3, 3, nil)
	out = b.Push(at(2), 4, 4, out)
	out = b.Push(at(2), 5, 5, out)
	out = b.Push(at(2), 6, 6, out)
	require.Equal(t, []int{3, 4, 5, 6, 7}, out)
	require.Zero(t, b.Stats().CurrentOccupancy)
}

// TestReorderBuffer_LateDropped verifies that a packet arriving behind nextExpected (its gap already resolved one
// way or another) is dropped, not re-released or double-counted, and does not panic.
func TestReorderBuffer_LateDropped(t *testing.T) {
	b := NewReorderBuffer[int](0, 0, 0)

	out := b.Push(at(0), 5, 5, nil)
	require.Equal(t, []int{5}, out)
	out = b.Push(at(1), 6, 6, nil)
	require.Equal(t, []int{6}, out)

	// 5 shows up again (duplicate/straggler), already behind expected(7).
	out = b.Push(at(2), 5, 555, nil)
	require.Empty(t, out)
	require.EqualValues(t, 1, b.Stats().LateDropped)

	// Normal operation continues unaffected.
	out = b.Push(at(3), 7, 7, nil)
	require.Equal(t, []int{7}, out)
}

// TestReorderBuffer_FlushMidWindow verifies Flush drains every held item in ascending order, with none left behind
// and none duplicated, and does not touch the Reordered counter (a flush is a drain, not a correction).
func TestReorderBuffer_FlushMidWindow(t *testing.T) {
	b := NewReorderBuffer[int](0, 0, 0)

	out := b.Push(at(0), 10, 10, nil) // expected becomes 11
	require.Equal(t, []int{10}, out)
	out = b.Push(at(1), 13, 13, nil)
	require.Empty(t, out)
	out = b.Push(at(1), 12, 12, nil)
	require.Empty(t, out)

	flushed := b.Flush(nil)
	require.Equal(t, []int{12, 13}, flushed, "held items drain in ascending order (11 was never held)")
	require.Zero(t, b.Stats().Reordered, "Flush is a drain, not a correction")
	require.Zero(t, b.Stats().CurrentOccupancy)
	_, ok := b.Deadline()
	require.False(t, ok)

	// Buffer is usable afterward: the next Push re-seeds from scratch relative to whatever arrives (nextExpected is
	// unchanged by Flush, so 11 is still expected).
	out = b.Push(at(2), 11, 11, nil)
	require.Equal(t, []int{11}, out)
}

// TestReorderBuffer_ReconnectResync verifies the discontinuity/resync rule: a stable increasing run, then a single
// packet whose sequence number is far below nextExpected (simulating a reconnect that restarted the far end's RTP
// sequence numbering), followed by a normal run from the new base. Without maxJump this would degenerate into every
// subsequent packet reading as permanently "late" (see TestReorderBuffer_WithoutResync_WouldWedge below for the
// contrast). With it, the discontinuity is detected, the window flushes, nextExpected rebases, and normal emission
// resumes.
func TestReorderBuffer_ReconnectResync(t *testing.T) {
	maxWindow := 8
	b := NewReorderBuffer[int](maxWindow, time.Hour, 4*maxWindow) // maxWait large: only maxJump should fire here

	// Stable run: 100, 101, 102 - each released immediately.
	var out []int
	out = b.Push(at(0), 100, 100, out)
	out = b.Push(at(1), 101, 101, out)
	out = b.Push(at(2), 102, 102, out)
	require.Equal(t, []int{100, 101, 102}, out)
	require.Zero(t, b.Stats().Resyncs)

	// Hold one packet ahead so the flush-on-resync path has something to drain.
	out = b.Push(at(3), 105, 105, nil)
	require.Empty(t, out)
	require.EqualValues(t, 1, b.Stats().CurrentOccupancy)

	// Reconnect: the far end restarts at sequence 10 - far below nextExpected(103), well beyond maxJump(32).
	out = b.Push(at(4), 10, 10, nil)
	require.Equal(t, []int{105, 10}, out, "the stale held packet flushes first, then the resync packet itself releases")
	stats := b.Stats()
	require.EqualValues(t, 1, stats.Resyncs)
	require.Zero(t, stats.CurrentOccupancy)

	// Normal run resumes from the new base.
	out = b.Push(at(5), 11, 11, nil)
	require.Equal(t, []int{11}, out)
	out = b.Push(at(6), 13, 13, nil)
	require.Empty(t, out)
	out = b.Push(at(7), 12, 12, nil)
	require.Equal(t, []int{12, 13}, out, "reordering correction works normally again after the resync")
}

// TestReorderBuffer_WithoutResync_WouldWedge documents, as a regression guard, that a small maxJump is what makes
// TestReorderBuffer_ReconnectResync's recovery possible: with maxJump effectively disabled (larger than the
// sequence distance involved), the same reconnect scenario wedges - every packet from the new stream reads as
// permanently behind nextExpected and is dropped, never resyncing.
func TestReorderBuffer_WithoutResync_WouldWedge(t *testing.T) {
	b := NewReorderBuffer[int](8, time.Hour, 60000) // maxJump so large it never fires for this scenario

	out := b.Push(at(0), 100, 100, nil)
	require.Equal(t, []int{100}, out)

	out = b.Push(at(1), 10, 10, nil) // far below nextExpected(101), but within the (huge) maxJump
	require.Empty(t, out, "without resync, this reads as a late/behind packet, not a discontinuity")
	require.EqualValues(t, 1, b.Stats().LateDropped)
	require.Zero(t, b.Stats().Resyncs)

	out = b.Push(at(2), 11, 11, nil)
	require.Empty(t, out, "still behind nextExpected(101) - wedged")
	require.EqualValues(t, 2, b.Stats().LateDropped)
}

// TestReorderBuffer_Expire_MultiPacketLoss verifies that a burst loss of several consecutive sequence numbers
// resolves in a single Expire call once the deadline elapses, rather than requiring one call - and one full
// MaxWait - per missing sequence number.
func TestReorderBuffer_Expire_MultiPacketLoss(t *testing.T) {
	maxWait := 20 * time.Millisecond
	b := NewReorderBuffer[int](0, maxWait, 0)

	out := b.Push(at(0), 1, 1, nil) // expected becomes 2
	require.Equal(t, []int{1}, out)

	out = b.Push(at(1), 5, 5, nil) // 2, 3 and 4 are all missing; 5 is held
	require.Empty(t, out)
	deadline, ok := b.Deadline()
	require.True(t, ok)

	out = b.Expire(deadline, nil)
	require.Equal(t, []int{5}, out, "5 must release from a single Expire call, not require 3 separate timeouts")
	stats := b.Stats()
	require.EqualValues(t, 3, stats.LostAfterTimeout, "2, 3 and 4 are all counted lost by this one call")
	require.Zero(t, stats.CurrentOccupancy)
	_, ok = b.Deadline()
	require.False(t, ok)
}

// TestReorderBuffer_MaxJumpBelowMaxWindow_Corrected verifies that a caller-supplied maxJump at or below maxWindow
// (a nonsensical configuration - see NewReorderBuffer's doc) is corrected rather than left to misread ordinary
// in-window gaps as discontinuities.
func TestReorderBuffer_MaxJumpBelowMaxWindow_Corrected(t *testing.T) {
	b := NewReorderBuffer[int](32, time.Hour, 16) // maxJump(16) <= maxWindow(32): must be corrected to 4*32=128

	out := b.Push(at(0), 0, 0, nil) // expected becomes 1
	require.Equal(t, []int{0}, out)

	// Distance 19 from expected(1): well within maxWindow(32), and would have exceeded the misconfigured
	// maxJump(16) had it not been corrected.
	out = b.Push(at(1), 20, 20, nil)
	require.Empty(t, out, "held as a normal gap, not resynced")
	require.Zero(t, b.Stats().Resyncs)
	require.EqualValues(t, 1, b.Stats().CurrentOccupancy)
}

// TestReorderBuffer_MaxJumpCapped verifies that an absurdly large explicit maxJump is clamped to the true
// reachable ceiling (maxSequenceJump) rather than silently making resync unreachable.
func TestReorderBuffer_MaxJumpCapped(t *testing.T) {
	b := NewReorderBuffer[int](8, time.Hour, 1_000_000) // must be clamped to maxSequenceJump (32768)

	out := b.Push(at(0), 100, 100, nil) // expected becomes 101
	require.Equal(t, []int{100}, out)

	// Sequence distance exactly maxSequenceJump (32768) from expected(101) - the largest distance abs(delta) can
	// ever produce, and thus the largest value a resync can ever actually trigger on. If the cap were ineffective
	// (still 1,000,000), this would instead grind through the window-full forced-expiry path.
	out = b.Push(at(1), 101+32768, 99999, nil)
	require.Equal(t, []int{99999}, out, "must resync immediately, not fall through to forced expiry")
	require.EqualValues(t, 1, b.Stats().Resyncs)
}

// TestReorderBuffer_ForwardJumpResync verifies a large *positive* discontinuity (arriving far ahead, e.g. a
// reconnect where the far end restarts its sequence numbering at a much higher value) resyncs immediately, the
// same as the backward case already covered by TestReorderBuffer_ReconnectResync.
func TestReorderBuffer_ForwardJumpResync(t *testing.T) {
	maxWindow := 8
	b := NewReorderBuffer[int](maxWindow, time.Hour, 4*maxWindow) // maxJump=32

	out := b.Push(at(0), 100, 100, nil) // expected becomes 101
	require.Equal(t, []int{100}, out)

	// Far ahead of expected(101) - well beyond maxJump(32) - not just outside maxWindow(8).
	out = b.Push(at(1), 50000, 50000, nil)
	require.Equal(t, []int{50000}, out, "must resync immediately, not iterate through forced expiry")
	stats := b.Stats()
	require.EqualValues(t, 1, stats.Resyncs)
	require.Zero(t, stats.LostAfterTimeout, "resync doesn't count each skipped position as a timeout loss")

	// Normal operation resumes from the new base.
	out = b.Push(at(2), 50001, 50001, nil)
	require.Equal(t, []int{50001}, out)
}

// TestReorderBuffer_FirstPacketWraparound verifies the seed path wraps nextExpected correctly to 0 when the very
// first packet the buffer ever sees is sequence number 65535.
func TestReorderBuffer_FirstPacketWraparound(t *testing.T) {
	b := NewReorderBuffer[int](0, 0, 0)

	out := b.Push(at(0), 65535, 1, nil)
	require.Equal(t, []int{1}, out)

	out = b.Push(at(1), 0, 2, nil)
	require.Equal(t, []int{2}, out, "0 is exactly the wrapped successor of 65535")
}

// TestReorderBuffer_MaxWindowOne_HoldsSwappedPair verifies that maxWindow bounds how far ahead the buffer will
// hold an item *inclusive* of maxWindow itself: with maxWindow=1, a packet arriving exactly one position ahead
// (the simplest possible single swapped pair) must be held, not immediately force-expired past.
func TestReorderBuffer_MaxWindowOne_HoldsSwappedPair(t *testing.T) {
	b := NewReorderBuffer[int](1, time.Hour, 0) // maxWait large: only window-fullness matters here

	out := b.Push(at(0), 5, 5, nil) // expected becomes 6
	require.Equal(t, []int{5}, out)

	out = b.Push(at(1), 7, 7, nil) // distance 1 from expected(6) == maxWindow(1): must fit, not force-expire
	require.Empty(t, out, "7 must be held, not released or force-expired")
	stats := b.Stats()
	require.EqualValues(t, 1, stats.CurrentOccupancy)
	require.Zero(t, stats.LostAfterTimeout, "nothing should have been force-expired to admit this")

	out = b.Push(at(2), 6, 6, nil) // fills the gap
	require.Equal(t, []int{6, 7}, out, "6 fills the gap and cascades the held 7 out")
}

// TestReorderBuffer_ForcedExpiry_RearmsDeadlineForNewGap verifies that when forced expiry advances nextExpected
// to a new position - exposing a gap nobody has budgeted a wait for yet - the deadline is re-armed even if an
// earlier-held item survives the forced-expiry walk and keeps slots non-empty throughout (the case the
// wasEmpty-only check used to miss, leaving the new gap to inherit a stale, already-expiring deadline that
// belonged to the old gap the walk just discarded).
func TestReorderBuffer_ForcedExpiry_RearmsDeadlineForNewGap(t *testing.T) {
	maxWait := 20 * time.Millisecond
	b := NewReorderBuffer[int](4, maxWait, 0) // maxWindow=4

	out := b.Push(at(0), 1, 1, nil) // expected becomes 2
	require.Equal(t, []int{1}, out)

	// 6 is held directly: distance 4 from expected(2) == maxWindow(4), fits.
	out = b.Push(at(1), 6, 6, nil)
	require.Empty(t, out)
	require.EqualValues(t, 1, b.Stats().CurrentOccupancy)
	oldDeadline, ok := b.Deadline()
	require.True(t, ok)

	// Much later - just before the old gap-at-2 deadline would fire - packet 8 arrives at distance 6 from
	// expected(2): forces 2 expiries (2->3->4, since 8-4=4 fits), but that walk never reaches position 6, so the
	// held 6 survives - slots stays non-empty throughout, which is exactly the case the old wasEmpty-only check
	// missed.
	later := oldDeadline.Add(-time.Microsecond)
	out = b.Push(later, 8, 8, nil)
	require.Empty(t, out, "8 itself is buffered too - the gap at the new expected(4) is still open")
	require.EqualValues(t, 2, b.Stats().CurrentOccupancy, "6 must have survived the forced-expiry walk")

	newDeadline, ok := b.Deadline()
	require.True(t, ok)
	require.Equal(t, later.Add(maxWait), newDeadline,
		"the new gap (at expected=4) must get a fresh deadline, not inherit the old gap's stale one")

	// Behavioral check, not just the reported value: the new gap must not fire at the old, stale deadline.
	out = b.Expire(oldDeadline, nil)
	require.Empty(t, out, "the new gap must not expire at the old gap's deadline")
}

// TestReorderBuffer_MaxWindowClamped verifies that an absurdly large maxWindow is clamped to its true reachable
// ceiling (maxSequenceWindow) before it can size the slots map unreasonably or overflow the 4*maxWindow maxJump
// fallback - an unclamped huge maxWindow wraps 4*maxWindow negative, which then silently bypasses maxJump's own
// ceiling check (a negative value is never greater than a positive ceiling), leaving every single packet
// misread as a discontinuity.
func TestReorderBuffer_MaxWindowClamped(t *testing.T) {
	b := NewReorderBuffer[int](math.MaxInt64/4+1000, 0, 0)

	require.EqualValues(t, maxSequenceWindow, b.maxWindow, "maxWindow must be clamped to its reachable ceiling")
	require.EqualValues(t, maxSequenceJump, b.maxJump, "maxJump must be a sane, positive, reachable value")

	out := b.Push(at(0), 100, 100, nil)
	require.Equal(t, []int{100}, out)

	// An ordinary out-of-order pair must still be corrected, not misread as a resync - which an unclamped,
	// overflowed-to-negative maxJump would have caused for every single packet.
	out = b.Push(at(1), 102, 102, nil)
	require.Empty(t, out, "held, not resynced")
	require.Zero(t, b.Stats().Resyncs)

	out = b.Push(at(2), 101, 101, nil)
	require.Equal(t, []int{101, 102}, out, "reordering still works correctly with the clamped values")
}
