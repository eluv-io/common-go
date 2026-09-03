package pacer

import (
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/utc-go"
)

// DiscardContext tracks early packet discard state. It is used during startup to wait for a stable stream before
// establishing timing baselines. The caller supplies the stream's clock-to-wall-clock logic as a conversion function,
// so this works for any paced stream - RTP timestamps, MPEG-TS PCR, or ATS arrival timestamps alike.
//
// See ShouldDiscard for the detailed discard logic, and ResetOnGap and ResetForSourceChange for how to restart the
// phase.
type DiscardContext struct {
	DiscardPeriod    duration.Spec // How long to wait after baseline update
	MaxDiscardPeriod duration.Spec // Max time to wait after baseline update

	// Periods used in place of the two above while the current phase was started by ResetForSourceChange. Each falls
	// back to its startup counterpart when zero. See PacerLogicConfig.SourceChangeDiscardPeriod.
	SourceChangePeriod    duration.Spec // How long to wait after baseline update (on ResetForSourceChange)
	MaxSourceChangePeriod duration.Spec // Max time to wait after baseline update (on ResetForSourceChange)

	// T0Threshold is the improvement in T0 required to restart the discard period. Smaller improvements still refine
	// the baseline, they just do not hold the phase open. See PacerLogicConfig.DiscardT0Threshold.
	T0Threshold duration.Spec

	DiscardComplete     bool                                     // True once discard phase is over
	FirstPacketTime     utc.UTC                                  // Timestamp of the first received packet
	T0                  utc.UTC                                  // Wall clock time when the (unwrapped) stream timestamp was 0
	T0UpdatedAt         utc.UTC                                  // When the baseline was last updated
	StartupT0Correction statsutil.RawStatistics[duration.Millis] // T0 adjustment stats during startup/discard phase (reset on gap!)

	// sourceChange selects the SourceChange* periods for the current phase. Set by ResetForSourceChange and cleared by
	// ResetOnGap, since a gap is a discontinuity within one source rather than a switch to a different one.
	sourceChange bool

	toDuration func(int64) time.Duration // converts a timestamp in the stream's clock units to a duration
}

// period returns the discard period in force for the current phase.
func (d *DiscardContext) period() duration.Spec {
	if d.sourceChange && d.SourceChangePeriod > 0 {
		return d.SourceChangePeriod
	}
	return d.DiscardPeriod
}

// maxPeriod returns the cap in force for the current phase, never below that phase's own period.
func (d *DiscardContext) maxPeriod() duration.Spec {
	if d.sourceChange && d.MaxSourceChangePeriod > 0 {
		return max(d.SourceChangePeriod, d.MaxSourceChangePeriod)
	}
	if d.sourceChange && d.SourceChangePeriod > 0 {
		return max(d.SourceChangePeriod, d.MaxDiscardPeriod)
	}
	return d.MaxDiscardPeriod
}

// NewDiscardContext creates a new discard context with the specified period. toDuration is used to convert unwrapped
// timestamps to time.Duration and must not be nil.
func NewDiscardContext(discardPeriod, maxDiscardPeriod duration.Spec, toDuration func(int64) time.Duration) *DiscardContext {
	if toDuration == nil {
		panic("pacer: toDuration must not be nil")
	}
	td := toDuration
	return &DiscardContext{
		DiscardPeriod:    discardPeriod,
		MaxDiscardPeriod: max(discardPeriod, maxDiscardPeriod),
		toDuration:       td,
	}
}

// ShouldDiscard reports whether a packet should be withheld while the timing baseline is still being established.
//
// The baseline is T0, the wall clock time at which this stream's timestamp was zero. A reader that is behind the live
// edge sees stream time advance faster than wall time, so its computed T0 keeps dropping; once it reaches the edge the
// two advance together and T0 settles. Discarding until T0 stops improving is therefore how the live edge is found,
// and it needs no knowledge of the stream's bitrate.
//
//  1. First packet since a reset: establish the baseline and discard
//  2. T0 improves by more than T0Threshold: take it, restart the period, and discard
//  3. T0 improves by less: take it, but leave the period running - see T0Threshold
//  4. Period elapses with no significant improvement: the edge has been found, stop discarding for good
//  5. The cap elapses first: give up and continue with the best baseline seen, rather than failing the stream
//
// The error return is retained for compatibility and is always nil.
func (d *DiscardContext) ShouldDiscard(tsUnwrapped int64, now utc.UTC) (bool, error) {
	// If discard phase is complete, never discard
	if d.DiscardComplete {
		return false, nil
	}

	// Calculate T0 for this packet (wall clock time when the timestamp was 0)
	t0 := now.Add(-d.toDuration(tsUnwrapped))
	period := d.period()

	// Set once, and deliberately not on every reset: the cap runs from the first packet of this phase, so that
	// repeated gaps cannot keep extending the window. ResetOnGap clears it only for a phase that had completed, and
	// ResetForSourceChange clears it because the cap should then apply to the new source.
	if d.FirstPacketTime.IsZero() {
		d.FirstPacketTime = now
	}

	if d.T0UpdatedAt.IsZero() {
		// First packet (since last reset) - establish baseline
		d.T0 = t0
		d.T0UpdatedAt = now
		log.Debug("discard: first packet, establishing baseline",
			"ts", tsUnwrapped,
			"t0", t0,
			"period", period,
			"source_change", d.sourceChange)
		if period == 0 {
			// Discard is disabled: complete the phase immediately so subsequent packets bypass all discard logic,
			// including T0-shift discards.
			d.DiscardComplete = true
			return false, nil
		}
		return true, nil
	}

	// Give up on convergence once the cap elapses and continue with the best baseline found. Checked before the
	// improvement branch below, because a stream whose T0 never stops improving is exactly the case the cap is for:
	// checked after, it could never fire for one. Failing here instead of completing would take down a stream that is
	// merely jittery.
	if maxPeriod := d.maxPeriod(); maxPeriod != 0 && now.Sub(d.FirstPacketTime) > maxPeriod.Duration() {
		d.DiscardComplete = true
		log.Warn("discard: max period exceeded, continuing with the current baseline", nil,
			"ts", tsUnwrapped,
			"t0", d.T0,
			"max_period", maxPeriod,
			"elapsed", now.Sub(d.FirstPacketTime))
		return false, nil
	}

	// Take any improvement to the baseline, but only restart the period for one large enough to mean the reader is
	// still catching up. Restarting on every improvement lets ordinary jitter hold the phase open indefinitely, since
	// the running minimum of a jittery signal keeps creeping down.
	if t0.Before(d.T0) {
		adjustment := d.T0.Sub(t0)
		d.StartupT0Correction.Update(now, duration.Millis(adjustment))
		d.T0 = t0
		if adjustment > d.T0Threshold.Duration() {
			d.T0UpdatedAt = now
		}
		log.Debug("discard: T0 adjusted, updating baseline",
			"ts", tsUnwrapped,
			"new_t0", t0,
			"delta", adjustment,
			"restarted_period", adjustment > d.T0Threshold.Duration(),
			"total_adj_ms", float64(d.StartupT0Correction.Sum)/1e6)
		return true, nil // discard - baseline was just updated
	}

	// T0 is not earlier - check if discard period has elapsed
	elapsed := now.Sub(d.T0UpdatedAt)
	if elapsed < period.Duration() {
		return true, nil // still in discard period
	}

	// Discard period complete - mark it and stop discarding
	d.DiscardComplete = true
	log.Debug("discard: period complete, starting normal operation",
		"ts", tsUnwrapped,
		"t0", d.T0,
		"elapsed", elapsed)
	return false, nil
}

// ResetForSourceChange resets the discard context for a deliberate switch to a different source, as opposed to a gap
// within one. The new source has to be located in time exactly as the first one did, so the phase still runs, but it
// runs on the SourceChange periods: output is already flowing to consumers, so the window is a visible gap rather
// than mere startup latency, and it is worth trading some baseline precision to keep it short.
func (d *DiscardContext) ResetForSourceChange() {
	d.ResetOnGap()
	d.FirstPacketTime = utc.Zero // the cap applies to the new source, not to the stream this pacer started with
	d.sourceChange = true
}

// ResetOnGap resets the discard context after a gap is detected in the stream.
func (d *DiscardContext) ResetOnGap() {
	log.Debug("discard: resetting discard context after gap", "discard_complete", d.DiscardComplete)

	// A gap is a discontinuity within the current source, so the startup periods apply again. ResetForSourceChange
	// sets this back afterwards - it calls through here first.
	d.sourceChange = false

	if d.DiscardComplete {
		d.DiscardComplete = false
		d.FirstPacketTime = utc.Zero // start a new discard period
	}
	// When still in the discard phase (DiscardComplete=false), FirstPacketTime is intentionally preserved.
	// MaxDiscardPeriod is measured from the very first packet the stream ever produced, not reset on each gap, so that
	// repeated gaps cannot extend the discard window indefinitely.

	d.T0 = utc.Zero
	d.T0UpdatedAt = utc.Zero
	d.StartupT0Correction = statsutil.RawStatistics[duration.Millis]{}
}
