package rtp

import (
	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

// DiscardContext tracks early packet discard state for RTP streams. This is used during startup to wait for a stable
// RTP stream before establishing timing baselines.
type DiscardContext struct {
	DiscardPeriod    duration.Spec // How long to wait after baseline update
	MaxDiscardPeriod duration.Spec // Max time to wait after baseline update

	DiscardComplete     bool                                     // True once discard phase is over
	FirstPacketTime     utc.UTC                                  // Timestamp of the first received packet
	T0                  utc.UTC                                  // Wall clock time when (unwrapped) RTP timestamp was 0
	T0UpdatedAt         utc.UTC                                  // When the baseline was last updated
	StartupT0Correction statsutil.RawStatistics[duration.Millis] // T0 adjustment stats during startup/discard phase (reset on gap!)
}

// NewDiscardContext creates a new discard context with the specified period.
func NewDiscardContext(discardPeriod, maxDiscardPeriod duration.Spec) *DiscardContext {
	return &DiscardContext{
		DiscardPeriod:    discardPeriod,
		MaxDiscardPeriod: max(discardPeriod, maxDiscardPeriod),
	}
}

// ShouldDiscard determines if a packet should be discarded based on the unwrapped RTP timestamp. Returns true if the
// packet should be discarded, false if it should be processed. Returns an error if the max discard period has been
// exceeded without establishing a stable baseline. This is needed when reading from buffered RTP data (e.g. from parts)
// and we need to find the edge before proceeding.
//
//  1. First packet (no baseline): discard and establish T0 baseline
//  2. If packet's T0 < stored T0: discard and update baseline (stream restart/discontinuity)
//  3. If packet's T0 >= stored T0 and discard period not elapsed: discard
//  4. If packet's T0 >= stored T0 and discard period elapsed: stop discarding permanently
func (d *DiscardContext) ShouldDiscard(rtpTimestamp int64, now utc.UTC) (bool, error) {
	// If discard phase is complete, never discard
	if d.DiscardComplete {
		return false, nil
	}

	// Calculate T0 for this packet (wall clock time when RTP ts was 0)
	t0 := now.Add(-TicksToDuration(rtpTimestamp))

	if d.FirstPacketTime.IsZero() {
		// first packet
		d.FirstPacketTime = now
	} else if d.MaxDiscardPeriod != 0 && now.Sub(d.FirstPacketTime) > d.MaxDiscardPeriod.Duration() {
		return true, errors.NoTrace("discard",
			errors.K.Timeout,
			"reason", "max discard period exceeded",
			"max_discard_period", d.MaxDiscardPeriod,
		)
	}

	if d.T0UpdatedAt.IsZero() {
		// First packet (since last reset) - establish baseline
		d.T0 = t0
		d.T0UpdatedAt = now
		log.Debug("discard: first packet, establishing baseline",
			"rtp_ts", rtpTimestamp,
			"t0", t0)
		if d.DiscardPeriod == 0 {
			// Discard is disabled: complete the phase immediately so subsequent packets bypass all discard logic,
			// including T0-shift discards.
			d.DiscardComplete = true
			return false, nil
		}
		return true, nil
	}

	// If this packet's T0 is earlier than stored T0, update baseline
	if t0.Before(d.T0) {
		adjustment := d.T0.Sub(t0)
		d.StartupT0Correction.Update(now, duration.Millis(adjustment))
		log.Debug("discard: T0 adjusted, updating baseline",
			"rtp_ts", rtpTimestamp,
			"old_t0", d.T0,
			"new_t0", t0,
			"delta", adjustment,
			"total_adj_ms", float64(d.StartupT0Correction.Sum)/1e6)
		d.T0 = t0
		d.T0UpdatedAt = now
		return true, nil // discard - baseline was just updated
	}

	// T0 is not earlier - check if discard period has elapsed
	elapsed := now.Sub(d.T0UpdatedAt)
	if elapsed < d.DiscardPeriod.Duration() {
		return true, nil // still in discard period
	}

	// Discard period complete - mark it and stop discarding
	d.DiscardComplete = true
	log.Debug("discard: period complete, starting normal operation",
		"rtp_ts", rtpTimestamp,
		"t0", d.T0,
		"elapsed", elapsed)
	return false, nil
}

func (d *DiscardContext) ResetOnGap() {
	log.Debug("discard: resetting discard context after RTP gap", "discard_complete", d.DiscardComplete)

	if d.DiscardComplete {
		d.DiscardComplete = false
		d.FirstPacketTime = utc.Zero // start a new discard period
	}

	d.T0 = utc.Zero
	d.T0UpdatedAt = utc.Zero
	d.StartupT0Correction = statsutil.RawStatistics[duration.Millis]{}
}
