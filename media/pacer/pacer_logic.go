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
	DefaultDriftThreshold = 2 * time.Millisecond
	DefaultPosDriftPeriod = time.Minute

	// DefaultDiscardT0Threshold is the default PacerLogicConfig.DiscardT0Threshold: the T0 improvement required to
	// restart the discard period. Large enough that ordinary arrival jitter does not keep the window open, small
	// enough to still catch the real convergence steps of a stream being read faster than real time.
	DefaultDiscardT0Threshold = 50 * time.Millisecond
)

// PacerLogicConfig holds the configuration for PacerLogic.
type PacerLogicConfig struct {
	// Stream is the id used for logging
	Stream string `json:"-"`

	// EventLog is the log for recording events (gaps, timing baseline adjustments)
	EventLog elog.ILog `json:"-"`

	// DiscardPeriod is the period for determining T0 during which all packets are discarded
	DiscardPeriod duration.Spec `json:"discard_period"`

	// MaxDiscardPeriod caps the discard phase. It is measured from the first packet of the stream, while DiscardPeriod
	// is measured from the last time the baseline improved, so it must be comfortably larger than DiscardPeriod:
	// reaching a live edge takes as long as it takes to read the backlog, and every improvement restarts DiscardPeriod.
	// On expiry the phase completes with the best baseline found so far, rather than failing the stream.
	MaxDiscardPeriod duration.Spec `json:"max_discard_period"`

	// SourceChangeDiscardPeriod is DiscardPeriod for a deliberate switch to a different source (see
	// DiscardContext.ResetForSourceChange), where the pacer is already running and its output is being consumed.
	//
	// The new source still has to be located in time, so the phase cannot be skipped, but its cost is different from
	// startup: no client is waiting at startup, whereas during a switch every connected client sees the window as a
	// gap. Keep it well below the receiver's idle timeout - an SRT peer defaults to dropping the connection after 2s
	// of silence. 0 falls back to DiscardPeriod.
	SourceChangeDiscardPeriod duration.Spec `json:"source_change_discard_period"`

	// MaxSourceChangeDiscardPeriod caps the source-change discard phase, as MaxDiscardPeriod does for startup. 0 falls
	// back to MaxDiscardPeriod.
	MaxSourceChangeDiscardPeriod duration.Spec `json:"max_source_change_discard_period"`

	// DiscardT0Threshold is the improvement in T0 required to restart the discard period. Without it any improvement
	// at all, down to a nanosecond, restarts the window: the running minimum of a jittery signal keeps creeping down,
	// so the phase would routinely run to MaxDiscardPeriod instead of completing. The baseline still takes every
	// improvement - this only governs whether the clock is restarted. Distinct from DriftThreshold, which is a
	// steady-state dead-band an order of magnitude smaller than the convergence steps seen here. Default: 50ms.
	DiscardT0Threshold duration.Spec `json:"discard_t0_threshold"`

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

	// DriftThreshold is the drift dead-band applied in both directions: positive drift is only acted on once its mean
	// over PosDriftPeriod exceeds this, and a single early packet is only treated as negative drift once it exceeds it.
	// This keeps normal jitter from re-anchoring the timing baseline. Applies to drift detection regardless of
	// AdjustTimeDrift (which only gates whether a detected drift also corrects baseTime). Default: 2ms when zero.
	DriftThreshold duration.Spec `json:"drift_threshold"`

	// MaxPosDriftCorrection caps the per-period baseTime advance applied for positive drift when AdjustTimeDrift is
	// true. Zero means no cap: the full mean drift over the period is applied at once (so a large accumulated backlog
	// is re-anchored in a single step). A non-zero cap makes recovery gradual (at most this much per PosDriftPeriod).
	MaxPosDriftCorrection duration.Spec `json:"max_pos_drift_correction"`

	// MaxDriftCorrectionStep caps how much of a detected positive-drift correction (after any MaxPosDriftCorrection
	// capping) is applied to baseTime on any single packet. The remainder is queued and drained on subsequent packets,
	// at most this much per packet, until fully applied. This caps the inter-packet-delay impact of a single correction
	// (e.g. a persistent clock-rate mismatch, or a faulty encoder holding/under-incrementing its timestamp) without
	// discarding any of it, unlike MaxPosDriftCorrection which permanently drops the excess above its cap. Zero means
	// no cap: the full (capped) correction is applied in one step, exactly as if this field did not exist.
	//
	// Deliberately applied to positive drift only: negative-drift corrections shorten the gap to the next target
	// time, so never trigger IPD peaks. Negative drift is always applied in a single step, as before.
	//
	// The configured value needs to be larger than genuine sustained drift per packet: (mean drift per PosDriftPeriod)
	// / (packets per PosDriftPeriod). For realistic packet rates and drift magnitudes this is a small fraction of a
	// millisecond. If this cap is set below that sustained rate, corrections queue faster than they drain and the
	// timing baseline falls behind the source indefinitely - the same behavior we observed before applying the full
	// positive drift.
	//
	// So safe values are > ~1ms, and must be smaller than the IPD increase we are willing to accept. E.g. 10ms, with a
	// nominal IPD of 10ms, would lead to max IPD of 20ms.
	MaxDriftCorrectionStep duration.Duration `json:"max_drift_correction_step"`

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
	c.DriftThreshold = duration.Spec(DefaultDriftThreshold)
	c.MaxPosDriftCorrection = 0
	c.MaxDriftCorrectionStep = 0
	c.SourceChangeDiscardPeriod = 0
	c.MaxSourceChangeDiscardPeriod = 0
	c.DiscardT0Threshold = duration.Spec(DefaultDiscardT0Threshold)
	return c
}

// PacerLogic computes target delivery times for packetized streams. It handles early-packet discarding, timing
// baseline establishment, and optional drift correction.
type PacerLogic struct {
	conf                 PacerLogicConfig
	log                  elog.ILog
	stats                *InStats
	discard              *DiscardContext                   // Early packet discard logic
	firstTimestamp       int64                             // First unwrapped timestamp
	baseTime             utc.UTC                           // Base time for first packet (now + delay)
	posDriftTracker      statsutil.Periodic[time.Duration] // Rolling mean of T0 drift per packet
	posDriftBaseline     utc.UTC                           // Reference time for the positive-drift mean (see below)
	driftThreshold       time.Duration                     // Effective drift dead-band (conf or default 2ms)
	toDuration           func(int64) time.Duration         // Effective clock conversion function
	pendingPosCorrection time.Duration                     // Positive-drift correction not yet drained into baseTime
}

// NewPacerLogic creates a new PacerLogic with the given configuration and stats collector.
func NewPacerLogic(
	conf PacerLogicConfig,
	stats *InStats,
) *PacerLogic {
	driftThreshold := conf.DriftThreshold.Duration()
	if driftThreshold == 0 {
		driftThreshold = DefaultDriftThreshold
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
		conf:            conf,
		log:             conf.EventLog,
		stats:           stats,
		toDuration:      toDuration,
		discard:         NewDiscardContext(conf.DiscardPeriod, conf.MaxDiscardPeriod, toDuration),
		driftThreshold:  driftThreshold,
		posDriftTracker: statsutil.Periodic[time.Duration]{Period: duration.Spec(posDriftPeriod)},
	}
	p.discard.SourceChangePeriod = conf.SourceChangeDiscardPeriod
	p.discard.MaxSourceChangePeriod = conf.MaxSourceChangeDiscardPeriod
	p.discard.T0Threshold = conf.DiscardT0Threshold
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
	p.posDriftBaseline = utc.Zero
	p.pendingPosCorrection = 0
	// gap detector is already updated by the last Detect() call, so no need to reset
}

// ResetSource prepares the logic for a deliberate switch to a different source: the next packet re-establishes the
// timing baseline and is delivered, rather than starting a new discard period (see DiscardContext.ResetForSourceChange).
//
// stats.Reset() also clears MinT0, which matters: a MinT0 carried over from the previous source would make the new
// source's first T0 look like a large negative drift and trigger a full-size baseline correction on the spot.
func (p *PacerLogic) ResetSource() {
	p.reset()
	p.discard.ResetForSourceChange()
}

// Packet computes the target delivery time for a pre-unwrapped timestamp. If gap is true, the pacer resets its internal
// state (discard phase restart, baseline re-establishment) before computing the target time. This is the clock-agnostic
// core: each PacketScheduler calls it after its own gap detection and unwrapping, whether the clock is an RTP
// timestamp, an MPEG-TS PCR or an ATS arrival timestamp.
func (p *PacerLogic) Packet(now utc.UTC, tsUnwrapped int64, gap bool) (target utc.UTC, discard bool, err error) {
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
		p.posDriftBaseline = p.discard.T0

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

	// Track T0: if this T0 is earlier than our stored min by more than the drift threshold, it's a negative drift
	// event. Sub-threshold dips are treated as jitter and deliberately ignored: re-anchoring MinT0 (and resetting the
	// pos-drift tracker) on every early packet would let normal jitter ratchet the timeline earlier and repeatedly wipe
	// positive-drift recovery. Ignored dips still feed the pos-drift tracker below as small negative samples, which
	// naturally damps the mean.
	if negDrift := p.stats.MinT0.Sub(t0); negDrift > p.driftThreshold {
		// T0 decreased (negative drift) — record nominal drift and apply a capped correction to baseTime immediately.
		// Unlike positive drift below, this is deliberately NOT spread via MaxDriftCorrectionStep: a negative
		// correction shortens (rather than lengthens) the gap to the next target time.
		p.stats.NegDrift.Update(now, duration.Millis(negDrift))
		p.stats.MinT0 = t0
		// Reset the pos-drift tracker and its baseline: prior samples were relative to the old (higher) reference and
		// would inflate the next period's mean if kept.
		p.posDriftTracker = statsutil.Periodic[time.Duration]{Period: p.posDriftTracker.Period}
		p.posDriftBaseline = t0
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
				"neg_drift_ms", duration.Millis(negDrift),
				"applied_drift_ms", duration.Millis(apply),
				"new_base_time", p.baseTime.Format(time.RFC3339Nano))
		}
	}

	// Track positive T0 drift (stream running slow relative to wall clock) and optionally queue a correction to move
	// baseTime forward. The correction is proportional: the mean drift over the period is queued at once (optionally
	// capped by MaxPosDriftCorrection), so the timeline actually tracks a persistently-slow source instead of creeping.
	// Using the mean is flip-flop-safe: it is averaged over PosDriftPeriod (so infrequent late packets barely move it),
	// gated by the DriftThreshold dead-band, and never exceeds the current drift. Negative drift values (stream
	// momentarily fast) are included so they pull the mean down and prevent spurious corrections.
	//
	// The reference here is posDriftBaseline, not MinT0 - the two are kept deliberately independent:
	//   - posDriftBaseline advances by the full queued correction as soon as it's queued, so the same drift isn't
	//     measured and queued again next period - even though baseTime itself only catches up gradually, via the
	//     pendingPosCorrection drain below.
	//   - MinT0 is left untouched here. It stays the true historical minimum t0, read only by the negative-drift
	//     branch above to detect genuine early arrivals.
	// Coupling the two would lead to an immediate negative correction on the next regular packet.
	{
		drift := t0.Sub(p.posDriftBaseline)
		if periodEnded := p.posDriftTracker.UpdateNow(now, drift); periodEnded {
			meanDrift := p.posDriftTracker.Previous.Mean
			if meanDrift > p.driftThreshold {
				p.stats.PosDrift.Update(now, duration.Millis(meanDrift))
				if p.conf.AdjustTimeDrift {
					apply := meanDrift
					if maxCorr := p.conf.MaxPosDriftCorrection.Duration(); maxCorr > 0 && apply > maxCorr {
						apply = maxCorr
					}
					p.pendingPosCorrection += apply
					p.posDriftBaseline = p.posDriftBaseline.Add(apply)
					p.log.Info("positive drift detected",
						"stream", p.conf.Stream,
						"mean_drift_ms", duration.Millis(meanDrift),
						"queued_drift_ms", duration.Millis(apply))
				}
			}
		}
	}

	// Drain any pending positive-drift correction into baseTime, at most MaxDriftCorrectionStep per packet (zero =
	// uncapped, i.e. drained in a single step exactly as before). This runs every packet, including the packet that
	// just queued a fresh correction above, so a newly detected correction starts taking effect immediately; it only
	// spreads corrections that exceed the per-packet step over multiple consecutive packets, bounding each packet's
	// IPD impact. Positive-only: adding a step-limited amount always lengthens (never inverts) the gap to the next
	// target time, which is what keeps this bounded rather than a new instability - see the negative-drift branch
	// above for why the same spreading is not safe in that direction.
	if p.pendingPosCorrection > 0 {
		step := p.pendingPosCorrection
		if maxStep := p.conf.MaxDriftCorrectionStep.Duration(); maxStep > 0 && step > maxStep {
			step = maxStep
		}
		p.baseTime = p.baseTime.Add(step)
		targetTime = targetTime.Add(step)
		p.pendingPosCorrection -= step
		p.stats.PosDriftApplied.Update(now, duration.Millis(step))
		p.log.Info("positive drift corrected",
			"stream", p.conf.Stream,
			"applied_drift_ms", duration.Millis(step),
			"remaining_drift_ms", duration.Millis(p.pendingPosCorrection))
	}

	// Track push freshness: how far ahead is target time from now when pushed
	pushAhead := targetTime.Sub(now)
	p.stats.PushAhead.Update(now, duration.Millis(pushAhead))

	return targetTime, false, nil
}
