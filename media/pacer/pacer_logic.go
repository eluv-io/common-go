package pacer

import (
	"fmt"
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

const (
	DefaultPosDriftThreshold  = 2 * time.Millisecond
	DefaultPosDriftCorrection = time.Millisecond
	DefaultPosDriftPeriod     = time.Minute
)

// PacerLogicConfig holds the configuration for PacerLogic.
type PacerLogicConfig struct {
	// Stream is the id used for logging
	Stream string `json:"-"`

	// EventLog is the log for recording events (gaps, timing baseline adjustments)
	EventLog elog.ILog `json:"-"`

	// DiscardPeriod is the period for determining T0 during which all packets are discarded
	DiscardPeriod duration.Spec `json:"discard_period"`

	// MaxDiscardPeriod is the maximum period for discarding packets.
	MaxDiscardPeriod duration.Spec `json:"max_discard_period"`

	// Delay is the size of the de-jitter buffer
	Delay duration.Spec `json:"delay"`

	// AdjustTimeDrift enables continuous drift correction: negative drift (T0 drifts backward, stream running fast)
	// shifts baseTime earlier; positive drift (T0 drifts forward, stream running slow) shifts baseTime later.
	AdjustTimeDrift bool `json:"adjust_time_drift"`

	// MaxNegDriftCorrection caps the per-packet baseTime correction applied for negative drift when AdjustTimeDrift is
	// true. Zero means no cap: the full observed drift is applied immediately.
	MaxNegDriftCorrection duration.Spec `json:"max_neg_drift_correction"`

	// PosDriftPeriod is the window over which T0 drift is averaged for positive-drift detection.
	// Default: 1 minute when zero.
	PosDriftPeriod duration.Spec `json:"pos_drift_period"`

	// PosDriftThreshold is the mean positive drift over PosDriftPeriod that triggers a correction.
	// Only meaningful when AdjustTimeDrift is true. Default: 2ms when zero.
	PosDriftThreshold duration.Spec `json:"pos_drift_threshold"`

	// PosDriftCorrection is the fixed baseTime advance applied when the mean positive drift exceeds
	// PosDriftThreshold. Only meaningful when AdjustTimeDrift is true. Default: 1ms when zero.
	PosDriftCorrection duration.Spec `json:"pos_drift_correction"`

	// ToDuration converts an unwrapped timestamp (in clock units) to a time.Duration. Must not be nil. Set this to the
	// appropriate clock conversion, e.g. rtp.TicksToDuration for 90 kHz RTP clocks, or mpegts.PcrToDuration for MPEG-TS
	// PCR-based pacing (27 MHz clock).
	ToDuration func(int64) time.Duration `json:"-"`
}

func (c *PacerLogicConfig) InitDefaults() *PacerLogicConfig {
	c.DiscardPeriod = 0
	c.MaxDiscardPeriod = 0
	c.Delay = duration.Second
	c.AdjustTimeDrift = true
	c.MaxNegDriftCorrection = 0
	c.PosDriftPeriod = duration.Spec(DefaultPosDriftPeriod)
	c.PosDriftThreshold = duration.Spec(DefaultPosDriftThreshold)
	c.PosDriftCorrection = duration.Spec(DefaultPosDriftCorrection)
	return c
}

// PacerLogic computes target delivery times for packetized streams. It handles early-packet discarding, timing
// baseline establishment, and optional drift correction.
type PacerLogic struct {
	conf               PacerLogicConfig
	log                elog.ILog
	stats              *InStats
	discard            *DiscardContext                   // Early packet discard logic
	firstTimestamp     int64                             // First unwrapped timestamp
	baseTime           utc.UTC                           // Base time for first packet (now + delay)
	posDriftTracker    statsutil.Periodic[time.Duration] // rolling mean of T0 drift per packet
	posDriftThreshold  time.Duration                     // effective threshold (conf or default 2ms)
	posDriftCorrection time.Duration                     // effective correction (conf or default 1ms)
	toDuration         func(int64) time.Duration         // effective clock conversion function
}

// NewPacerLogic creates a new PacerLogic with the given configuration and stats collector.
func NewPacerLogic(
	conf PacerLogicConfig,
	stats *InStats,
) *PacerLogic {
	posDriftThreshold := conf.PosDriftThreshold.Duration()
	if posDriftThreshold == 0 {
		posDriftThreshold = DefaultPosDriftThreshold
	}
	posDriftCorrection := conf.PosDriftCorrection.Duration()
	if posDriftCorrection == 0 {
		posDriftCorrection = DefaultPosDriftCorrection
	}
	if posDriftCorrection > posDriftThreshold {
		posDriftCorrection = posDriftThreshold // cap correction amount to threshold
	}
	posDriftPeriod := conf.PosDriftPeriod.Duration()
	if posDriftPeriod == 0 {
		posDriftPeriod = DefaultPosDriftPeriod
	}
	toDuration := conf.ToDuration
	if toDuration == nil {
		panic("pacer: PacerLogicConfig.ToDuration must not be nil")
	}
	p := &PacerLogic{
		conf:               conf,
		log:                conf.EventLog,
		stats:              stats,
		toDuration:         toDuration,
		discard:            NewDiscardContext(conf.DiscardPeriod, conf.MaxDiscardPeriod, toDuration),
		posDriftThreshold:  posDriftThreshold,
		posDriftCorrection: posDriftCorrection,
		posDriftTracker:    statsutil.Periodic[time.Duration]{Period: duration.Spec(posDriftPeriod)},
	}
	p.reset()
	return p
}

// reset resets all state, so that we start afresh
func (p *PacerLogic) reset() {
	p.discard.ResetOnGap()
	p.baseTime = utc.Zero
	p.firstTimestamp = 0
	p.stats.Reset()
	p.posDriftTracker = statsutil.Periodic[time.Duration]{Period: p.posDriftTracker.Period}
	// gap detector is already updated by the last Detect() call, so no need to reset
}

// PacketTs computes the target delivery time for a pre-unwrapped timestamp. If gap is true, the pacer resets its
// internal state (discard phase restart, baseline re-establishment) before computing the target time. This is the
// clock-agnostic core; Packet() calls it after RTP-specific gap detection and unwrapping.
func (p *PacerLogic) PacketTs(now utc.UTC, tsUnwrapped int64, gap bool) (target utc.UTC, discard bool, err error) {
	if gap {
		p.reset()
		p.stats.StreamResets++
		p.stats.LastStreamReset = now
	}

	ts := tsUnwrapped

	// discard early packets until stream stabilizes
	discard, err = p.discard.ShouldDiscard(ts, now)
	if err != nil {
		return now, true, errors.E(err, "stream", p.conf.Stream, "stats", p.stats)
	} else if discard {
		return now, true, nil
	}

	// on first non-discarded packet, establish timing baseline
	if p.baseTime.IsZero() {
		p.firstTimestamp = ts
		// Anchor baseTime to the stable discard T0, not to `now`. The first non-discarded packet may arrive with
		// positive or negative jitter, which would otherwise offset the entire timeline.
		// discard.T0 + toDuration(ts) equals now for a jitter-free arrival, and correctly removes any jitter
		// offset from the baseline.
		p.baseTime = p.discard.T0.Add(p.toDuration(ts)).Add(p.conf.Delay.Duration())

		// Initialize MinT0 from the stable discard T0 so that drift tracking starts from the correct reference. Using
		// t0 of the first packet (= now - toDuration(ts)) would inflate MinT0 by any arrival jitter and trigger
		// spurious drift corrections on subsequent jitter-free packets.
		p.stats.MinT0 = p.discard.T0

		// Capture startup negative drift from discard phase
		p.stats.StartupT0Correction = p.discard.StartupT0Correction

		p.log.Info("timing baseline established",
			"ts_unwrapped", ts,
			"stream", p.conf.Stream,
			"base_time", p.baseTime.Format(time.RFC3339Nano),
			"delay", p.conf.Delay,
			"startup_t0_correction_ms", fmt.Sprintf("%.1f", float64(p.stats.StartupT0Correction.Sum)/float64(time.Millisecond)))
	}

	// Calculate target transmission time based on unwrapped timestamp delta
	tsDelta := ts - p.firstTimestamp

	// Target time = base time + time delta from first packet
	targetTime := p.baseTime.Add(p.toDuration(tsDelta))

	// Calculate T0 for this packet (wall clock time when the timestamp was 0)
	t0 := now.Add(-p.toDuration(ts))

	// Track T0: if this T0 is earlier than our stored min, it's a negative drift event
	if t0.Before(p.stats.MinT0) {
		// T0 decreased (negative drift) — record nominal drift and optionally apply a capped correction to baseTime.
		negDrift := p.stats.MinT0.Sub(t0)
		p.stats.NegDrift.Update(now, duration.Millis(negDrift))
		p.stats.MinT0 = t0
		// Reset the pos-drift tracker: prior samples were relative to the old (higher) MinT0 and would
		// inflate the next period's mean if kept.
		p.posDriftTracker = statsutil.Periodic[time.Duration]{Period: p.posDriftTracker.Period}
		if p.conf.AdjustTimeDrift {
			apply := negDrift
			if maxCorr := p.conf.MaxNegDriftCorrection.Duration(); maxCorr > 0 && apply > maxCorr {
				apply = maxCorr
			}
			p.stats.NegDriftApplied.Update(now, duration.Millis(apply))
			p.baseTime = p.baseTime.Add(-apply)
			targetTime = targetTime.Add(-apply)
			p.log.Info("negative drift corrected",
				"stream", p.conf.Stream,
				"neg_drift_ms", fmt.Sprintf("%.3f", float64(negDrift)/float64(time.Millisecond)),
				"applied_drift_ms", fmt.Sprintf("%.3f", float64(apply)/float64(time.Millisecond)),
				"new_base_time", p.baseTime.Format(time.RFC3339Nano))
		}
	}

	// Track T0 drift (stream running slow relative to wall clock) and optionally correct baseTime forward.
	// Negative drift values (stream momentarily fast) are included so they pull the mean down and prevent
	// spurious corrections after a fast burst.
	{
		drift := t0.Sub(p.stats.MinT0)
		if periodEnded := p.posDriftTracker.UpdateNow(now, drift); periodEnded {
			meanDrift := time.Duration(p.posDriftTracker.Previous.Mean)
			if meanDrift > p.posDriftThreshold {
				p.stats.PosDrift.Update(now, duration.Millis(meanDrift))
				if p.conf.AdjustTimeDrift {
					p.stats.PosDriftApplied.Update(now, duration.Millis(p.posDriftCorrection))
					p.baseTime = p.baseTime.Add(p.posDriftCorrection)
					targetTime = targetTime.Add(p.posDriftCorrection)
					p.stats.MinT0 = p.stats.MinT0.Add(p.posDriftCorrection)
					p.log.Info("positive drift corrected",
						"stream", p.conf.Stream,
						"mean_drift_ms", fmt.Sprintf("%.3f", float64(meanDrift)/float64(time.Millisecond)),
						"applied_drift_ms", fmt.Sprintf("%.3f", float64(p.posDriftCorrection)/float64(time.Millisecond)),
						"new_base_time", p.baseTime.Format(time.RFC3339Nano))
				}
			}
		}
	}

	// Track push freshness: how far ahead is target time from now when pushed
	pushAhead := targetTime.Sub(now)
	p.stats.PushAhead.Update(now, duration.Millis(pushAhead))

	return targetTime, false, nil
}
