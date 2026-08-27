package rtp

import (
	"cmp"
	"math"
	"slices"
	"time"
)

// Default tuning for ReorderBuffer, chosen to correct short-range network reordering without absorbing sustained jitter
// (a different problem, handled elsewhere). See NewReorderBuffer. There is no DefaultReorderMaxJump constant: unlike
// MaxWindow/MaxWait, MaxJump's fallback is always derived as a multiple of the *effective*
// MaxWindow (see NewReorderBuffer).
const (
	DefaultReorderMaxWindow = 32
	DefaultReorderMaxWait   = 20 * time.Millisecond
)

// ReorderStats holds ReorderBuffer's cumulative counters, plus a point-in-time gauge (CurrentOccupancy).
type ReorderStats struct {
	// Reordered is the number of packets released out of network-arrival order - i.e. a real reorder was corrected.
	Reordered uint64 `json:"reordered"`
	// MaxReorderDelta is the largest |seq-nextExpected| ever observed at admission time.
	MaxReorderDelta int64 `json:"max_reorder_delta"`
	// LostAfterTimeout is the number of sequence numbers skipped because their gap's MaxWait elapsed with no fill.
	LostAfterTimeout uint64 `json:"lost_after_timeout"`
	// LateDropped is the number of packets that arrived already behind nextExpected - too late to place in order,
	// because their gap had already timed out (or been resynced past) by the time they showed up.
	LateDropped uint64 `json:"late_dropped"`
	// Resyncs is the number of discontinuities that forced nextExpected to be re-based directly to an arriving
	// sequence number (see NewReorderBuffer's doc on MaxJump).
	Resyncs uint64 `json:"resyncs"`
	// CurrentOccupancy is the number of packets currently held in the window (a gauge, not cumulative).
	CurrentOccupancy int `json:"current_occupancy"`
}

// ReorderBuffer corrects short-range reordering of items (network packets) keyed by a 16-bit RTP-style sequence number,
// releasing them strictly in ascending order. It is not a jitter buffer: it does not absorb sustained delay, only local
// reordering, so it should be tuned small (see NewReorderBuffer's defaults).
//
// The buffer tracks one core value, nextExpected: the sequence number due for release next. Every arriving item is
// classified against it:
//   - On time (its sequence number == nextExpected): released immediately. If any later items are already held, they
//     cascade out right behind it, for as long as each next sequence number is already present.
//   - Early (sequence number > nextExpected): held in buffer, not released yet. If this is the first item held since
//     the window was last empty, its arrival starts the wait timer for nextExpected's own gap.
//   - Late (sequence number < nextExpected): dropped. Its gap has already been resolved one way or another - filled,
//     timed out, or resynced past - so there is no correct place left to release it into.
//
// Three parameters shape these dynamics:
//   - maxWindow bounds how far ahead of nextExpected the buffer will hold an item - it's the buffer's capacity. An item
//     arriving farther ahead forces the current gap to be declared lost immediately, freeing room, instead of growing
//     the window further.
//   - maxWait bounds how long a gap may stay open. Once it elapses, Expire declares the missing sequence number
//     lost, advances nextExpected past it, and releases whatever is now sequence-contiguous.
//   - maxJump guards against a discontinuity - e.g. a reconnect where the source restarts its sequence numbering.
//     An item arriving more than maxJump away from nextExpected, in either direction, is treated as a new stream:
//     whatever is held is flushed, and nextExpected rebases directly to it. Without this, a discontinuity would
//     wedge the buffer instead, since every item afterward would keep reading as "late" forever.
//
// ReorderBuffer has no internal timer, goroutine, or channel: time is a parameter to every call, and the caller
// owns the timer. After every Push, call Deadline and (re)arm a single timer to the time it returns (or disarm it
// when ok is false, meaning the window is empty and nothing can time out). Call Expire only when that timer fires -
// there is no fixed poll period, and no need to call Expire on every packet: it is a cheap no-op whenever the
// current deadline has not yet elapsed. See ExampleReorderBuffer for this pattern end to end.
//
// It is not safe for concurrent use - like SequenceUnwrapper and GapDetector in this package, all calls must be
// serialized by the caller (typically a single reader goroutine).
type ReorderBuffer[T any] struct {
	// maxWindow is the configured max sequence-number span (the buffer's capacity): the largest distance an arriving
	// packet may be ahead of nextExpected before its gap is forced to time out to make room for it.
	maxWindow int
	// maxWait is the configured max wall-clock time a gap may stay open before Expire treats it as genuine loss.
	maxWait time.Duration
	// maxJump is the configured discontinuity threshold (see NewReorderBuffer): an arriving sequence number more
	// than this far from nextExpected, in either direction, triggers a resync instead of normal gap handling.
	maxJump int

	// hasNext is false only until the very first packet Push has ever seen, at which point nextExpected is seeded
	// from it and hasNext becomes permanently true (a resync rebases nextExpected but does not touch hasNext).
	hasNext bool
	// nextExpected is the sequence number the buffer is currently waiting to release; it only ever advances (by one
	// on a clean release or cascade step, by more on a forced expiry, or via a direct rebase on resync) and never
	// moves backward. Every admission decision in Push is a comparison against this value.
	nextExpected uint16
	// slots holds items that have arrived ahead of nextExpected, keyed by their own sequence number, waiting for
	// nextExpected to reach them (via cascade) or for the window/deadline to force a decision. Its size is bounded
	// by maxWindow: Push never lets the span between nextExpected and an admitted slot reach maxWindow.
	slots map[uint16]T
	// gapDeadline is the wall-clock time at which the current head-of-window gap (nextExpected's own missing slot)
	// should be treated as lost - the value Deadline reports and Expire compares now against. The zero value means
	// no gap is currently pending (slots is empty), so no timer is needed.
	gapDeadline time.Time

	// stats accumulates the buffer's cumulative counters; Stats returns a snapshot with CurrentOccupancy filled in
	// from len(slots) at read time, since occupancy is a gauge rather than something tracked incrementally here.
	stats ReorderStats
}

// maxSequenceWindow is the largest value Push's delta (signed 16-bit sequence-distance arithmetic) can ever take
// on the positive side: the true reachable ceiling for maxWindow. A maxWindow configured above this can never
// actually bind - delta can never exceed it, so the window-full check would never fire - while still being used
// as the slots map's allocation-size hint and as an input to the maxJump fallback below (4 * maxWindow), so
// NewReorderBuffer clamps to it before either of those, guarding against excessive/failed allocation for the
// former and integer overflow for the latter (an unclamped maxWindow near the top of int's range makes
// 4*maxWindow wrap negative, which would then silently bypass maxJump's own ceiling check just below, since a
// negative maxJump is never greater than a positive ceiling - leaving Push's abs(delta) >= maxJump check
// permanently true and misreading every single packet as a discontinuity).
const maxSequenceWindow = math.MaxInt16

// maxSequenceJump is the largest value abs(int16(seq-nextExpected)) can ever produce (achieved when that signed
// delta is math.MinInt16): the true reachable ceiling for maxJump. A maxJump configured above this can never
// trigger - the resync it's meant to provide would be silently unreachable - so NewReorderBuffer clamps to it.
const maxSequenceJump = maxSequenceWindow + 1

// NewReorderBuffer creates a ReorderBuffer. maxWindow bounds the maximum sequence-number span (in packets) the buffer
// will hold a gap open for (the buffer's capacity). An arriving packet further ahead than this forces the
// head-of-window gap to be treated as lost before it is admitted. maxWait bounds how long a gap may stay open in
// wall-clock time. maxJump guards against a discontinuity (e.g. a reconnect that restarts the far end's RTP sequence
// numbering, so a completely unrelated sequence space starts arriving): when an arriving sequence number is more than
// maxJump away from nextExpected in EITHER direction, the buffer flushes whatever it holds and rebases directly to the
// arriving sequence number, rather than either wedging (every subsequent packet reads as permanently "too late") or
// slowly grinding through maxWindow-at-a-time forced expiry.
//
// maxJump must be larger than maxWindow to be meaningful - otherwise ordinary in-window gaps would be misread as
// discontinuities - and is enforced accordingly: a zero/non-positive maxWindow or maxWait falls back to this
// package's Default* constants; a maxJump at or below the (possibly just-defaulted) effective maxWindow falls back
// to 4 times it instead - not a fixed constant, so it stays proportional to whatever window is actually in effect.
// maxWindow is clamped to maxSequenceWindow and maxJump to maxSequenceJump, since larger values could never bind
// (see those constants' doc comments) - maxWindow is clamped first, since it both sizes the slots map and feeds
// the 4x maxJump fallback above, so an unclamped maxWindow could make that fallback overflow.
func NewReorderBuffer[T any](maxWindow int, maxWait time.Duration, maxJump int) *ReorderBuffer[T] {
	if maxWindow <= 0 {
		maxWindow = DefaultReorderMaxWindow
	}
	if maxWindow > maxSequenceWindow {
		maxWindow = maxSequenceWindow
	}
	if maxWait <= 0 {
		maxWait = DefaultReorderMaxWait
	}
	if maxJump <= maxWindow {
		maxJump = 4 * maxWindow
	}
	if maxJump > maxSequenceJump {
		maxJump = maxSequenceJump
	}
	return &ReorderBuffer[T]{
		maxWindow: maxWindow,
		maxWait:   maxWait,
		maxJump:   maxJump,
		slots:     make(map[uint16]T, maxWindow),
	}
}

// MaxWindow returns the configured window size, e.g. for a caller sizing its own reusable scratch slice for the
// out parameter of Push/Expire/Flush.
func (b *ReorderBuffer[T]) MaxWindow() int {
	return b.maxWindow
}

// Push admits one arriving item carrying the given sequence number. Items that become releasable - immediately, or
// via a cascade of already-held successors, or because a discontinuity forced a resync - are appended to out (which
// may be nil, or a caller-owned slice reused across calls, e.g. out[:0]) and returned in ascending sequence order.
// now is used to (re)establish the gap deadline reported by Deadline; the caller must supply real wall-clock time
// for that to behave sensibly (a fixed/synthetic clock is fine, and useful, in tests).
func (b *ReorderBuffer[T]) Push(now time.Time, seq uint16, item T, out []T) []T {
	if !b.hasNext {
		b.hasNext = true
		b.nextExpected = seq + 1
		return append(out, item)
	}

	delta := int(int16(seq - b.nextExpected)) // same arithmetic trick as in SequenceUnwrapper to handle wrap-around

	// Record the raw admission distance before anything below can shrink it (the window-full loop advances
	// nextExpected, which would otherwise make a large delta look smaller by the time it's recorded) - so this
	// reflects the true displacement observed on the wire, in either direction, matching the field's own doc.
	if d := int64(abs(delta)); d > b.stats.MaxReorderDelta {
		b.stats.MaxReorderDelta = d
	}

	if abs(delta) >= b.maxJump {
		// Discontinuity: whatever is held belongs to a stream position we're abandoning - flush it (in the order it
		// would have been released, for whatever that's worth to the caller) and rebase to the arriving packet.
		out = b.flush(out)
		b.stats.Resyncs++
		b.nextExpected = seq + 1
		out = append(out, item)
		b.updateDeadline(now)
		return out
	}

	if delta < 0 {
		// Already behind nextExpected: its gap either timed out or was resynced past. Too late to place in order.
		b.stats.LateDropped++
		return out
	}

	if delta == 0 {
		out = append(out, item)
		b.nextExpected++
		if len(b.slots) > 0 {
			// Nothing to cascade or re-arm when the window is already empty - the common in-order steady state.
			out = b.cascade(out)
			b.updateDeadline(now)
		}
		return out
	}

	// delta > 0: a gap relative to nextExpected. If admitting it would make the held span exceed maxWindow, the
	// head-of-window gap has to give first - force it to time out (possibly more than once) until there's room.
	// Strictly greater-than: a packet arriving exactly maxWindow positions ahead still fits (maxWindow bounds how
	// far ahead the buffer will hold an item, inclusive - see the type's doc comment), only farther forces expiry.
	ranForcedExpiry := false
	for delta > b.maxWindow {
		ranForcedExpiry = true
		out = b.expireHead(out)
		delta = int(int16(seq - b.nextExpected))
		if delta == 0 {
			out = append(out, item)
			b.nextExpected++
			out = b.cascade(out)
			b.updateDeadline(now)
			return out
		}
		if delta < 0 {
			b.stats.LateDropped++
			b.updateDeadline(now)
			return out
		}
	}

	wasEmpty := len(b.slots) == 0
	b.slots[seq] = item
	if wasEmpty || ranForcedExpiry {
		// Either the first held gap since the window was last empty, or forced expiry just advanced nextExpected to
		// a position nobody has budgeted a wait for yet (even if another, earlier-held item survived that walk and
		// kept slots non-empty throughout) - either way this is a gap whose wait budget starts now. A later
		// out-of-order arrival that doesn't force any expiry and doesn't change nextExpected leaves this deadline
		// untouched (see the delta>0 success path above, which never calls updateDeadline) - the wait is for the
		// head of the window, not extended by activity behind it.
		b.gapDeadline = now.Add(b.maxWait)
	}
	return out
}

// Expire applies the timeout rule: if the head-of-window's deadline (see Deadline) has elapsed, every sequence
// number from nextExpected up to (but not including) the next one actually held is counted as genuinely lost and
// skipped, and whatever becomes contiguous at that point is cascade-released into out (ascending order), which is
// returned. A no-op (returns out unchanged) if there is no pending deadline or it has not yet elapsed as of now.
//
// All of these skipped sequence numbers were revealed missing by the same original gap-detection event (the
// arrival that first exposed the gap now expiring), not by anything that happened since - so they are resolved
// together, in one call, rather than one at a time across repeated Expire calls spaced a further MaxWait apart.
// Advancing only one step per call here previously multiplied the effective wait for a K-packet burst loss to
// K×MaxWait instead of MaxWait once.
func (b *ReorderBuffer[T]) Expire(now time.Time, out []T) []T {
	if b.gapDeadline.IsZero() || now.Before(b.gapDeadline) {
		return out
	}
	// Bounded by maxWindow as a defensive measure: every held key is within maxWindow of nextExpected by
	// construction (Push never admits further than that), so this is guaranteed to either find a held key or empty
	// the window well within maxWindow steps - the bound just makes that termination explicit rather than implicit.
	for i := 0; i < b.maxWindow && len(b.slots) > 0; i++ {
		b.stats.LostAfterTimeout++
		b.nextExpected++
		if _, ok := b.slots[b.nextExpected]; ok {
			out = b.cascade(out)
			break
		}
	}
	b.updateDeadline(now)
	return out
}

// Deadline reports the wall-clock time at which the caller should next call Expire - i.e. MaxWait after the current
// head-of-window gap was (re)established. ok is false when no gap is pending (the window is empty), meaning no
// timer is needed.
func (b *ReorderBuffer[T]) Deadline() (deadline time.Time, ok bool) {
	return b.gapDeadline, !b.gapDeadline.IsZero()
}

// Flush drains everything currently held, in ascending sequence order, with no timeout logic - for shutdown, when
// the caller wants to forward whatever is left rather than wait out any pending gaps.
func (b *ReorderBuffer[T]) Flush(out []T) []T {
	return b.flush(out)
}

// Stats returns a snapshot of the buffer's cumulative counters plus its current occupancy gauge.
func (b *ReorderBuffer[T]) Stats() ReorderStats {
	s := b.stats
	s.CurrentOccupancy = len(b.slots)
	return s
}

// expireHead treats nextExpected as lost, advances past it, and cascade-releases whatever becomes contiguous. Not
// safe for concurrent use, like the rest of ReorderBuffer - see the type's doc comment.
func (b *ReorderBuffer[T]) expireHead(out []T) []T {
	b.stats.LostAfterTimeout++
	b.nextExpected++
	return b.cascade(out)
}

// cascade releases nextExpected, nextExpected+1, ... for as long as each is already held, advancing nextExpected
// past each one. Every item released this way arrived before its turn, so it counts toward Reordered.
func (b *ReorderBuffer[T]) cascade(out []T) []T {
	for {
		item, ok := b.slots[b.nextExpected]
		if !ok {
			return out
		}
		delete(b.slots, b.nextExpected)
		out = append(out, item)
		b.stats.Reordered++
		b.nextExpected++
	}
}

// flush drains every held item in ascending sequence order (wraparound-safe: ordered by signed distance from
// nextExpected, not by raw uint16 value) and clears the pending deadline. Does not touch Reordered - a flush is a
// drain, not a normal in-order-correction release.
func (b *ReorderBuffer[T]) flush(out []T) []T {
	if len(b.slots) == 0 {
		return out
	}
	seqs := make([]uint16, 0, len(b.slots))
	for s := range b.slots {
		seqs = append(seqs, s)
	}
	slices.SortFunc(seqs, func(x, y uint16) int {
		return cmp.Compare(int16(x-b.nextExpected), int16(y-b.nextExpected))
	})
	for _, s := range seqs {
		out = append(out, b.slots[s])
	}
	clear(b.slots)
	b.gapDeadline = time.Time{}
	return out
}

// updateDeadline recomputes the gap deadline after nextExpected has changed (cascade, expiry, or resync): cleared if
// the window is now empty, otherwise reset to a fresh MaxWait from now for whatever is now the head-of-window gap.
func (b *ReorderBuffer[T]) updateDeadline(now time.Time) {
	if len(b.slots) == 0 {
		b.gapDeadline = time.Time{}
		return
	}
	b.gapDeadline = now.Add(b.maxWait)
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
