package rtp

import (
	"fmt"
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

func (s *InStats) Reset() {
	*s = InStats{}
}

type PacerLogicConfig struct {
	Stream           string        // for logging
	EventLog         elog.ILog     // the log for recording events (rtp gaps, timing baseline adjustments)
	DiscardPeriod    duration.Spec // period for determining T0 during which all packets are discarded
	MaxDiscardPeriod duration.Spec // max period for discarding packets.
	Delay            duration.Spec // the size of the de-jitter buffer
	RtpSeqThreshold  int64         // sequence threshold for RTP gap detection
	RtpTsThreshold   duration.Spec // timestamp threshold for RTP gap detection

	// AdjustTimeRef enables continuous time-reference correction: whenever T0 drifts backward (stream running fast
	// relative to wall clock), baseTime is shifted earlier by the observed drift amount (subject to MaxT0AdjPerPacket).
	AdjustTimeRef bool

	// MaxT0AdjPerPacket caps the per-packet baseTime correction applied when AdjustTimeRef is true. Zero means no cap:
	// the full observed drift is applied immediately.
	MaxT0AdjPerPacket duration.Spec

	// SlowDriftPeriod is the window over which positive T0 drift is averaged for slow-drift detection.
	// Default: 1 minute when zero.
	SlowDriftPeriod duration.Spec

	// SlowDriftThreshold is the mean positive drift over SlowDriftPeriod that triggers a correction.
	// Only meaningful when AdjustTimeRef is true. Default: 2ms when zero.
	SlowDriftThreshold duration.Spec

	// SlowDriftCorrection is the fixed baseTime advance applied when the mean positive drift exceeds
	// SlowDriftThreshold. Only meaningful when AdjustTimeRef is true. Default: 1ms when zero.
	SlowDriftCorrection duration.Spec
}

type PacerLogic struct {
	conf              PacerLogicConfig
	log               elog.ILog
	stats             *InStats
	discard           *DiscardContext               // Early packet discard logic
	firstRtpTimestamp int64                         // First unwrapped RTP timestamp
	baseTime          utc.UTC                       // Base time for first packet (now + delay)
	gapDetector       *GapDetector                  // gap detector & seq/ts unwrapper
	slowDriftTracker  statsutil.Periodic[time.Duration] // rolling mean of positive T0 drift per packet
	slowDriftThreshold  time.Duration               // effective threshold (conf or default 2ms)
	slowDriftCorrection time.Duration               // effective correction (conf or default 1ms)
}

func NewPacerLogic(
	conf PacerLogicConfig,
	stats *InStats,
) *PacerLogic {
	slowDriftThreshold := conf.SlowDriftThreshold.Duration()
	if slowDriftThreshold == 0 {
		slowDriftThreshold = 2 * time.Millisecond
	}
	slowDriftCorrection := conf.SlowDriftCorrection.Duration()
	if slowDriftCorrection == 0 {
		slowDriftCorrection = time.Millisecond
	}
	slowDriftPeriod := conf.SlowDriftPeriod.Duration()
	if slowDriftPeriod == 0 {
		slowDriftPeriod = time.Minute
	}
	p := &PacerLogic{
		conf:                conf,
		log:                 conf.EventLog,
		stats:               stats,
		gapDetector:         NewRtpGapDetector(conf.RtpSeqThreshold, conf.RtpTsThreshold.Duration()),
		discard:             NewDiscardContext(conf.DiscardPeriod.Duration(), conf.MaxDiscardPeriod.Duration()),
		slowDriftThreshold:  slowDriftThreshold,
		slowDriftCorrection: slowDriftCorrection,
		slowDriftTracker:    statsutil.Periodic[time.Duration]{Period: duration.Spec(slowDriftPeriod)},
	}
	p.reset()
	return p
}

// reset resets all state, so that we start afresh
func (p *PacerLogic) reset() {
	// p.discard = NewDiscardContext(p.discardPeriod, p.maxDiscardPeriod)
	p.discard.ResetOnGap()
	p.baseTime = utc.Zero
	p.stats.Reset()
	p.slowDriftTracker = statsutil.Periodic[time.Duration]{Period: p.slowDriftTracker.Period}
	// gap detector is already updated by the last Detect() call, so no need to reset
}

// Packet signals reception of a new RTP packet with the given sequence number and timestamp. It returns the target
// transmission time for the packet, and whether the packet should be discarded (e.g. received during the discard phase
// at startup or if a gap was detected and we're waiting for the stream to stabilize).
func (p *PacerLogic) Packet(now utc.UTC, rtpSeq uint16, rtpTimestamp uint32) (target utc.UTC, discard bool, err error) {
	seq, ts, err := p.gapDetector.Detect(rtpSeq, rtpTimestamp)
	{
		p.stats.Seq = rtpSeq
		p.stats.Sequ = seq
		p.stats.Ts = rtpTimestamp
		p.stats.Tsu = ts
	}
	if err != nil {
		p.log.Warn("rtp gap", "stream", p.conf.Stream, err)
		p.reset()
		p.stats.StreamResets++
		p.stats.LastStreamReset = now
	}

	// discard early packets until stream stabilizes
	discard, err = p.discard.ShouldDiscard(ts, now)
	if err != nil {
		return now, true, errors.E(err, "stream", p.conf.Stream, "stats", p.stats)
	} else if discard {
		return now, true, nil
	}

	// on first non-discarded packet, establish timing baseline
	if p.baseTime.IsZero() {
		p.firstRtpTimestamp = ts
		// Base time is now + delay (when first packet should be sent)
		p.baseTime = now.Add(p.conf.Delay.Duration())

		// Capture startup T0 adjustment from discard phase
		p.stats.StartupT0Adjustment = p.discard.StartupT0Adjustment

		p.log.Info("timing baseline established",
			"rtp_ts", rtpTimestamp,
			"rtp_ts_unwrapped", ts,
			"rtp_seq", rtpSeq,
			"stream", p.conf.Stream,
			"base_time", p.baseTime.Format(time.RFC3339Nano),
			"delay", p.conf.Delay,
			"startup_t0_adj_ms", fmt.Sprintf("%.1f", float64(p.stats.StartupT0Adjustment.Sum)/float64(time.Millisecond)))
	}

	// Calculate target transmission time based on unwrapped RTP timestamp delta
	rtpDelta := ts - p.firstRtpTimestamp

	// Target time = base time + time delta from first packet
	targetTime := p.baseTime.Add(TicksToDuration(rtpDelta))

	// Track push freshness: how far ahead is target time from now when pushed
	pushAhead := targetTime.Sub(now)
	p.stats.PushAhead.Update(now, pushAhead)

	// Calculate T0 for this packet (wall clock time when RTP timestamp was 0)
	// T0 = now - (rtpTimestamp / 90000) seconds
	t0 := now.Add(-TicksToDuration(ts))

	// Track T0: if this T0 is earlier than our stored min, it's an adjustment
	if p.stats.MinT0.IsZero() {
		// First T0 after discard completed
		p.stats.MinT0 = t0
	} else if t0.Before(p.stats.MinT0) {
		// T0 decreased — record nominal drift and optionally apply a capped correction to baseTime.
		adjustment := p.stats.MinT0.Sub(t0)
		p.stats.T0Adjustment.Update(now, adjustment)
		p.stats.MinT0 = t0
		// Reset the slow-drift tracker: prior samples were relative to the old (higher) MinT0 and would
		// inflate the next period's mean if kept.
		p.slowDriftTracker = statsutil.Periodic[time.Duration]{Period: p.slowDriftTracker.Period}
		if p.conf.AdjustTimeRef {
			apply := adjustment
			if maxAdj := p.conf.MaxT0AdjPerPacket.Duration(); maxAdj > 0 && apply > maxAdj {
				apply = maxAdj
			}
			p.stats.T0AdjApplied.Update(now, apply)
			p.baseTime = p.baseTime.Add(-apply)
			targetTime = targetTime.Add(-apply)
			p.log.Info("time ref adjusted",
				"stream", p.conf.Stream,
				"t0_adj_ms", fmt.Sprintf("%.3f", float64(adjustment)/float64(time.Millisecond)),
				"applied_ms", fmt.Sprintf("%.3f", float64(apply)/float64(time.Millisecond)),
				"new_base_time", p.baseTime.Format(time.RFC3339Nano))
		}
	}

	// Track T0 drift (stream running slow relative to wall clock) and optionally correct baseTime forward.
	// Negative drift values (stream momentarily fast) are included so they pull the mean down and prevent
	// spurious corrections after a fast burst.
	if !p.stats.MinT0.IsZero() {
		drift := t0.Sub(p.stats.MinT0)
		if periodEnded := p.slowDriftTracker.UpdateNow(now, drift); periodEnded {
			meanDrift := time.Duration(p.slowDriftTracker.Previous.Mean)
			if meanDrift > p.slowDriftThreshold {
				p.stats.T0SlowDrift.Update(now, meanDrift)
				if p.conf.AdjustTimeRef {
					p.stats.T0SlowDriftApplied.Update(now, p.slowDriftCorrection)
					p.baseTime = p.baseTime.Add(p.slowDriftCorrection)
					targetTime = targetTime.Add(p.slowDriftCorrection)
					p.stats.MinT0 = p.stats.MinT0.Add(p.slowDriftCorrection)
					p.log.Info("slow drift corrected",
						"stream", p.conf.Stream,
						"mean_drift_ms", fmt.Sprintf("%.3f", float64(meanDrift)/float64(time.Millisecond)),
						"applied_ms", fmt.Sprintf("%.3f", float64(p.slowDriftCorrection)/float64(time.Millisecond)),
						"new_base_time", p.baseTime.Format(time.RFC3339Nano))
				}
			}
		}
	}

	return targetTime, false, nil
}
