package mpegts

import (
	"time"

	"github.com/Comcast/gots/v2/packet"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

const DefaultPcrGapThreshold = duration.Second

// pcrPidUnset marks that no PCR PID has been pinned yet (see pcrScheduler.pcrPid). Valid PIDs are 0..8191, so -1 is an
// unambiguous sentinel.
const pcrPidUnset = -1

// TsDisruptorPacerConfig holds configuration for a TsDisruptorPacer.
type TsDisruptorPacerConfig struct {
	Stream   string    `json:"-"` // Stream is the stream name for logging.
	StatsLog elog.ILog `json:"-"` // StatsLog is the logger to use for stats logging. If nil, stats are not logged.
	EventLog elog.ILog `json:"-"` // EventLog is the logger to use for event logging. If nil, events are not logged.

	// Logic holds timing logic configuration. ToDuration will be overridden to PcrToDuration; SeqThreshold and
	// TsThreshold are unused for MPEG-TS pacing (PCR gap detection is handled separately via PcrGapThreshold).
	Logic pacer.PacerLogicConfig `json:"logic"`

	// PcrGapThreshold is the maximum PCR jump between consecutive PCR-bearing packets before a stream reset is
	// triggered. Defaults to 1 second when zero.
	PcrGapThreshold duration.Spec `json:"pcr_gap_threshold"`

	BufferCapacity    int           `json:"buffer_capacity"`     // ring buffer capacity (rounded up to next power of 2; 0 → pacer.DefaultDisruptorCapacity)
	MinSleepThreshold duration.Spec `json:"min_sleep_threshold"` // sleep durations shorter than this are skipped (0 → pacer.DefaultMinSleepThreshold)
	TickerPeriod      duration.Spec `json:"ticker_period"`       // ticker period for scheduling delivery (0 → pacer.DefaultTickerPeriod)
	OversleepMargin   duration.Spec `json:"oversleep_margin"`    // jitter tolerated above TickerPeriod before a wake is counted as an oversleep (0 → pacer.DefaultOversleepMargin)
	StatsInterval     duration.Spec `json:"stats_interval"`      // interval for periodic stats logging (0 → pacer.DefaultStatsInterval, -1 → disabled)

	// SendAhead is how early the consumer dispatches a packet before its target time. 0 = dispatch at targetTs.
	SendAhead duration.Spec `json:"send_ahead"`

	// DeliveryMargin is the minimum lead time guaranteed to the "deliver" callback:
	//   sendAt = max(targetTs, now + DeliveryMargin)
	// Should be ≤ SendAhead so the floor is reliably reachable under normal conditions. 0 = disabled.
	DeliveryMargin duration.Spec `json:"delivery_margin"`

	// EstimatePcrRate, when true, schedules no-PCR batches using a PCR-tick rate estimated from consecutive
	// PCR-bearing batches instead of raw arrival time. This smooths input jitter for fixed-bandwidth streams where
	// packets arrive at a nominally constant rate. Falls back to arrival-time scheduling until the estimate is
	// available (requires two consecutive non-discarded PCR batches).
	EstimatePcrRate bool `json:"estimate_pcr_rate"`

	// StripRtp, when true, strips the RTP header from each incoming byte slice before extracting PCR.
	StripRtp bool `json:"strip_rtp"`

	// MaxBlock caps how long a Push may block on a full ring buffer before the packet is dropped instead. See
	// pacer.DisruptorEngineConfig.MaxBlock. 0, the default, waits indefinitely.
	MaxBlock duration.Spec `json:"max_block"`
}

func (c *TsDisruptorPacerConfig) InitDefaults() *TsDisruptorPacerConfig {
	c.Logic.InitDefaults()
	c.PcrGapThreshold = DefaultPcrGapThreshold
	c.BufferCapacity = pacer.DefaultDisruptorCapacity
	c.MinSleepThreshold = pacer.DefaultMinSleepThreshold
	c.TickerPeriod = pacer.DefaultTickerPeriod
	c.OversleepMargin = pacer.DefaultOversleepMargin
	c.StatsInterval = pacer.DefaultStatsInterval
	c.SendAhead = 0
	c.DeliveryMargin = 0
	c.StripRtp = true
	c.MaxBlock = 0
	return c
}

// engineConfig maps the protocol-independent knobs onto a pacer.DisruptorEngineConfig.
func (c *TsDisruptorPacerConfig) engineConfig() pacer.DisruptorEngineConfig {
	return pacer.DisruptorEngineConfig{
		Stream:            c.Stream,
		StatsLog:          c.StatsLog,
		EventLog:          c.EventLog,
		BufferCapacity:    c.BufferCapacity,
		MinSleepThreshold: c.MinSleepThreshold,
		TickerPeriod:      c.TickerPeriod,
		OversleepMargin:   c.OversleepMargin,
		StatsInterval:     c.StatsInterval,
		SendAhead:         c.SendAhead,
		DeliveryMargin:    c.DeliveryMargin,
		MaxBlock:          c.MaxBlock,
	}
}

// TsDisruptorPacer is an MPEG-TS callback pacer that uses a lock-free disruptor ring buffer as the jitter buffer. It
// uses PCR (Program Clock Reference, 27 MHz clock) for timestamp calculations and target-time scheduling. PCR timing is
// pinned to the first PID a PCR is detected on; PCRs on any other PID are ignored, so the independent program clocks of
// a multi-program transport stream do not corrupt the timeline. All protocol-independent machinery (ring buffer,
// consumer loop, stats, lifecycle) lives in the embedded pacer.DisruptorEngine; this type only supplies the
// PCR-specific scheduling via pcrScheduler.
//
// Usage:
//
//	pacer, _ := NewTsDisruptorPacer(conf)
//	go func() {
//	    err := pacer.Run(func(pkt []byte, at utc.UTC) error { ... })
//	}()
//	for _, pkt := range packets {
//	    pacer.Push(pkt)
//	}
//	pacer.Shutdown()
var _ pacer.StatsReporter = (*TsDisruptorPacer)(nil)

type TsDisruptorPacer struct {
	*pacer.DisruptorEngine
}

// NewTsDisruptorPacer creates a new TsDisruptorPacer with the given configuration.
func NewTsDisruptorPacer(conf TsDisruptorPacerConfig) (*TsDisruptorPacer, error) {
	if conf.StatsLog == nil {
		conf.StatsLog = elog.Noop
	}
	if conf.EventLog == nil {
		conf.EventLog = elog.Noop
	}
	if conf.Logic.EventLog == nil {
		conf.Logic.EventLog = conf.EventLog
	}
	if conf.Logic.Stream == "" {
		conf.Logic.Stream = conf.Stream
	}
	// Override ToDuration to PCR 27 MHz clock; callers cannot change this.
	conf.Logic.ToDuration = func(ts int64) time.Duration { return PcrToDuration(uint64(ts)) }
	if conf.PcrGapThreshold == 0 {
		conf.PcrGapThreshold = DefaultPcrGapThreshold
	}

	stats := &pacer.InStats{}
	sched := &pcrScheduler{
		logic:           pacer.NewPacerLogic(conf.Logic, stats),
		stats:           stats,
		gapDet:          PcrGapDetector{Threshold: DurationToPcr(conf.PcrGapThreshold.Duration())},
		pcrPid:          pcrPidUnset,
		estimatePcrRate: conf.EstimatePcrRate,
		stripRtp:        conf.StripRtp,
		pcrGapThreshold: conf.PcrGapThreshold,
		eventLog:        conf.EventLog,
		stream:          conf.Stream,
	}
	engine, err := pacer.NewDisruptorEngine(conf.engineConfig(), sched)
	if err != nil {
		return nil, errors.E("NewTsDisruptorPacer", err)
	}
	return &TsDisruptorPacer{DisruptorEngine: engine}, nil
}

// pcrScheduler is the MPEG-TS pacer.PacketScheduler. It extracts PCR timing from a batch of TS packets (pinned to a
// single PID) and computes target delivery times via PacerLogic. Batches without PCR are scheduled by offsetting the
// last known target by the elapsed wall-clock time since that target was established; if EstimatePcrRate is enabled and
// an estimate is available, the estimated per-batch PCR tick rate is used instead to smooth input jitter. Batches
// arriving before any PCR has been seen are discarded. All fields are accessed only from the engine's Push goroutine,
// under the engine's input-stats lock.
type pcrScheduler struct {
	logic  *pacer.PacerLogic
	stats  *pacer.InStats
	gapDet PcrGapDetector

	lastTarget     utc.UTC // most recent target (for no-PCR batches)
	lastPcrArrival utc.UTC // wall clock arrival time of the batch that set lastTarget

	// pcrPid is the PID that PCR timing is pinned to: the first PID a PCR is detected on. PCRs on any other PID are
	// ignored so that the independent clocks of other programs in a multi-program transport stream do not corrupt the
	// timeline. pcrPidUnset until the first PCR is seen.
	pcrPid int

	// PCR rate estimation fields.
	lastPcrUnwrapped     int64 // unwrapped PCR value from the last non-discarded PCR batch
	estimatedPcrPerBatch int64 // estimated PCR ticks per batch; 0 = estimate not yet available
	noPcrBatchCount      int   // number of consecutive non-PCR batches since the last PCR batch

	estimatePcrRate bool
	stripRtp        bool
	pcrGapThreshold duration.Spec
	eventLog        elog.ILog
	stream          string
}

var _ pacer.PacketScheduler = (*pcrScheduler)(nil)

func (s *pcrScheduler) InStats() *pacer.InStats { return s.stats }

// ResetSource drops every piece of state pinned to the previous source. Most importantly pcrPid: it is pinned to the
// first PID a PCR is seen on and never re-pinned, so a new source carrying PCR on a different PID would find no PCR at
// all and be paced off the previous source's lastTarget.
func (s *pcrScheduler) ResetSource() {
	s.logic.ResetSource()
	s.pcrPid = pcrPidUnset
	s.gapDet.Unwrapper = PcrUnwrapper{}
	s.lastTarget = utc.Zero
	s.lastPcrArrival = utc.Zero
	s.lastPcrUnwrapped = 0
	s.estimatedPcrPerBatch = 0
	s.noPcrBatchCount = 0
}

func (s *pcrScheduler) Schedule(now utc.UTC, bts []byte) (utc.UTC, []byte, bool, error) {
	if s.stripRtp {
		stripped, err := rtp.StripHeader(bts)
		if err != nil {
			return utc.Zero, nil, false,
				errors.E("pcrScheduler.Schedule", errors.K.Invalid, "reason", "failed to strip RTP header", err)
		}
		bts = stripped
	}

	// Scan TS packets in the batch for a PCR. Timing is pinned to a single PID: once the first PCR PID is detected,
	// only PCRs on that PID are used. PCRs from other programs (other PIDs) in a multi-program transport stream carry
	// independent clocks and are ignored so they cannot corrupt the timeline.
	var pcrFound bool
	var pcrValue uint64
	for scan := bts; len(scan) >= packet.PacketSize; scan = scan[packet.PacketSize:] {
		// Cast slice to pointer directly to avoid copying 188 bytes into a local value that would escape to the heap
		// because its address is taken when calling ExtractPCR.
		pkt := (*packet.Packet)(scan[:packet.PacketSize])
		if s.pcrPid != pcrPidUnset && pkt.PID() != s.pcrPid {
			continue // not the pinned PCR PID
		}
		if pcr, ok := ExtractPCR(pkt); ok {
			pcrFound = true
			pcrValue = pcr
			if s.pcrPid == pcrPidUnset {
				s.pcrPid = pkt.PID() // pin timing to the first PID we detect a PCR on
				s.eventLog.Info("pinned PCR PID", "stream", s.stream, "pid", s.pcrPid)
			}
			break
		}
	}

	if pcrFound {
		prev, curr, gap := s.gapDet.Detect(pcrValue)
		if gap {
			s.eventLog.Warn("pcr gap",
				"stream", s.stream,
				"pid", s.pcrPid,
				"prev_pcru", prev,
				"curr_pcru", curr,
				"diff", curr-prev,
				"threshold", s.pcrGapThreshold)
			if s.estimatePcrRate {
				// Invalidate the rate estimate across a gap: the PCR clock jumps discontinuously.
				s.estimatedPcrPerBatch = 0
				s.noPcrBatchCount = 0
			}
		}

		target, discard, err := s.logic.Packet(now, curr, gap)
		// Update the TS stats after Packet (which may have reset the stats via reset() on a gap).
		s.stats.Ts.PCR = pcrValue
		s.stats.Ts.PCRu = curr
		s.stats.Ts.PID = s.pcrPid
		if err != nil {
			return utc.Zero, nil, false, errors.E("pcrScheduler.Schedule", err)
		}
		if discard {
			return utc.Zero, nil, true, nil
		}
		if s.estimatePcrRate {
			// Update rate estimate from the interval between consecutive non-discarded PCR batches. totalBatches counts
			// the intervals covered: noPcrBatchCount non-PCR batches + this PCR batch.
			if !s.lastTarget.IsZero() {
				pcrDelta := curr - s.lastPcrUnwrapped
				totalBatches := int64(s.noPcrBatchCount + 1)
				if pcrDelta > 0 {
					s.estimatedPcrPerBatch = pcrDelta / totalBatches
				}
			}
			s.lastPcrUnwrapped = curr
			s.noPcrBatchCount = 0
		}
		s.lastTarget = target
		s.lastPcrArrival = now
		return target, bts, false, nil
	}

	// No PCR in this batch.
	if s.lastTarget.IsZero() {
		// No PCR seen yet — drop the batch; we cannot schedule it.
		return utc.Zero, nil, true, nil
	}
	if s.estimatePcrRate {
		// Always count no-PCR batches so the denominator is correct when the first estimate is computed.
		s.noPcrBatchCount++
	}
	var target utc.UTC
	if s.estimatePcrRate && s.estimatedPcrPerBatch > 0 {
		// Schedule using the estimated PCR tick rate to avoid propagating arrival-time jitter.
		target = s.lastTarget.Add(PcrToDuration(uint64(s.estimatedPcrPerBatch) * uint64(s.noPcrBatchCount)))
	} else {
		// Fall back to arrival-time offset (also used before the estimate is available).
		target = s.lastTarget.Add(now.Sub(s.lastPcrArrival))
	}
	return target, bts, false, nil
}
