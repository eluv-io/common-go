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

// PacerStats tracks pacer statistics
type PacerStats struct {
	// PushAhead is (targetTime - currentTime) when packet is pushed
	PushAhead statsutil.Statistics[time.Duration]

	// T0 adjustment stats during startup/discard phase
	StartupT0Adjustment statsutil.Statistics[time.Duration]

	// T0 adjustment stats during active phase
	T0Adjustment statsutil.Statistics[time.Duration]

	// Minimum T0 seen, zero value means not set
	MinT0 utc.UTC

	// The number of times the stream has been reset due to an RTP gap
	StreamResets int
	// The time of the last stream reset
	LastStreamReset utc.UTC
}

func (s *PacerStats) Reset() {
	*s = PacerStats{}
}

type PacerLogicConfig struct {
	Stream           string        // for logging
	Log              *elog.Log     // the log
	DiscardPeriod    time.Duration // period for determining T0 during which all packets are discarded
	MaxDiscardPeriod time.Duration // max period for discarding packets.
	Delay            time.Duration // the size of the de-jitter buffer
	RtpSeqThreshold  int64         // sequence threshold for RTP gap detection
	RtpTsThreshold   duration.Spec // timestamp threshold for RTP gap detection
}

type PacerLogic struct {
	conf              PacerLogicConfig
	log               *elog.Log
	stats             *PacerStats
	discard           *DiscardContext // Early packet discard logic
	firstRtpTimestamp int64           // First unwrapped RTP timestamp
	baseTime          utc.UTC         // Base time for first packet (now + delay)
	gapDetector       *GapDetector    // gap detector & seq/ts unwrapper
}

func NewPacerLogic(
	conf PacerLogicConfig,
	stats *PacerStats,
) *PacerLogic {
	p := &PacerLogic{
		conf:        conf,
		log:         conf.Log,
		stats:       stats,
		gapDetector: NewRtpGapDetector(conf.RtpSeqThreshold, conf.RtpTsThreshold.Duration()),
		discard:     NewDiscardContext(conf.DiscardPeriod, conf.MaxDiscardPeriod),
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
	// gap detector is already updated by the last Detect() call, so no need to reset
}

// Packet signals reception of a new RTP packet with the given sequence number and timestamp. It returns the target
// transmission time for the packet, and whether the packet should be discarded (e.g. received during the discard phase
// at startup or if a gap was detected and we're waiting for the stream to stabilize).
func (p *PacerLogic) Packet(now utc.UTC, rtpSeq uint16, rtpTimestamp uint32) (target utc.UTC, discard bool, err error) {
	_, ts, err := p.gapDetector.Detect(rtpSeq, rtpTimestamp)
	if err != nil {
		p.log.Warn("rtp gap detected", "err", err, "stream", p.conf.Stream)
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
		p.baseTime = now.Add(p.conf.Delay)

		// Capture startup T0 adjustment from discard phase
		p.stats.StartupT0Adjustment = p.discard.StartupT0Adjustment

		p.log.Info("rtp pacer - timing baseline established",
			"rtp_ts", rtpTimestamp,
			"rtp_ts_unwrapped", ts,
			"rtp_seq", rtpSeq,
			"stream", p.conf.Stream,
			"base_time", p.baseTime.Format(time.RFC3339Nano),
			"delay", p.conf.Delay,
			"startup_t0_adj_ms", fmt.Sprintf("%.1f", float64(p.stats.StartupT0Adjustment.Sum)/1e6))
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
		// T0 decreased - track the adjustment
		adjustment := p.stats.MinT0.Sub(t0)
		p.stats.T0Adjustment.Update(now, adjustment)
		p.stats.MinT0 = t0
	}

	return targetTime, false, nil
}
