package rtp

import (
	"time"

	pionrtp "github.com/pion/rtp"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

// The disruptor timing/capacity defaults are owned by the pacer package (the home of DisruptorEngine). These aliases
// preserve the historical rtp.Default* names for existing callers.
const (
	MaxDisruptorCapacity     = pacer.MaxDisruptorCapacity
	DefaultDisruptorCapacity = pacer.DefaultDisruptorCapacity
	DefaultDeliveryMargin    = pacer.DefaultDeliveryMargin
	DefaultMinSleepThreshold = pacer.DefaultMinSleepThreshold
	DefaultTickerPeriod      = pacer.DefaultTickerPeriod
	DefaultStatsInterval     = pacer.DefaultStatsInterval
	DefaultOversleepMargin   = pacer.DefaultOversleepMargin
)

// DisruptorPacerConfig holds configuration for a DisruptorPacer.
type DisruptorPacerConfig struct {
	Stream   string    `json:"-"` // Stream is the stream name for logging.
	StatsLog elog.ILog `json:"-"` // StatsLog is the logger to use for stats logging. If nil, stats are not logged.
	EventLog elog.ILog `json:"-"` // EventLog is the logger to use for event logging. If nil, events are not logged.

	Logic             pacer.PacerLogicConfig `json:"logic"`               // timing logic configuration
	SeqThreshold      int64                  `json:"seq_threshold"`       // sequence number gap threshold (0 → 1)
	TsThreshold       duration.Spec          `json:"ts_threshold"`        // RTP timestamp gap threshold (0 → 1 second)
	BufferCapacity    int                    `json:"buffer_capacity"`     // ring buffer capacity (is rounded up to the next power of 2; 0 → DefaultDisruptorCapacity)
	MinSleepThreshold duration.Spec          `json:"min_sleep_threshold"` // sleep durations shorter than this are skipped (0 → DefaultMinSleepThreshold)
	TickerPeriod      duration.Spec          `json:"ticker_period"`       // ticker period for scheduling delivery (0 → DefaultTickerPeriod)
	OversleepMargin   duration.Spec          `json:"oversleep_margin"`    // jitter tolerated above TickerPeriod before a wake is counted as an oversleep (0 → DefaultOversleepMargin)
	StatsInterval     duration.Spec          `json:"stats_interval"`      // interval for periodic stats logging (0 → DefaultStatsInterval, -1 → disabled)

	// SendAhead is how early the consumer dispatches a packet before its target time. The ticker loop wakes up when
	// now >= targetTs - SendAhead, giving the "deliver" callback a lead-time window.
	// 0 = dispatch at targetTs.
	SendAhead duration.Spec `json:"send_ahead"`

	// DeliveryMargin is the minimum lead time guaranteed to the "deliver" callback:
	//   sendAt = max(targetTs, now + DeliveryMargin)
	// Packets that cannot satisfy this floor (targetTs already too close to now) are tracked as LateSends.
	// Should be ≤ SendAhead so the floor is reliably reachable under normal conditions. 0 = disabled.
	DeliveryMargin duration.Spec `json:"delivery_margin"`
}

func (c *DisruptorPacerConfig) InitDefaults() *DisruptorPacerConfig {
	c.Logic.InitDefaults()
	c.SeqThreshold = 1
	c.TsThreshold = duration.Second
	c.BufferCapacity = DefaultDisruptorCapacity
	c.MinSleepThreshold = DefaultMinSleepThreshold
	c.TickerPeriod = DefaultTickerPeriod
	c.OversleepMargin = DefaultOversleepMargin
	c.StatsInterval = DefaultStatsInterval
	c.SendAhead = 0
	c.DeliveryMargin = DefaultDeliveryMargin
	return c
}

// engineConfig maps the protocol-independent knobs onto a pacer.DisruptorEngineConfig.
func (c *DisruptorPacerConfig) engineConfig() pacer.DisruptorEngineConfig {
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
	}
}

var _ pacer.StatsReporter = (*DisruptorPacer)(nil)

// DisruptorPacer is an RTP callback pacer that uses a lock-free disruptor ring buffer as the jitter buffer. It uses
// PacerLogic for timestamp calculations and target-time scheduling. The ring buffer replaces the Go channel used by
// RtpPacer, trading simplicity for lower and more consistent per-slot overhead. All protocol-independent machinery
// (ring buffer, consumer loop, stats, lifecycle) lives in the embedded pacer.DisruptorEngine; this type only supplies
// the RTP-specific scheduling via rtpScheduler.
//
// Usage:
//
//	pacer, _ := NewDisruptorPacer(conf)
//	go func() {
//	    err := pacer.Run(func(pkt []byte, at utc.UTC) error { ... })
//	}()
//	for _, pkt := range packets {
//	    pacer.Push(pkt)
//	}
//	pacer.Shutdown()
type DisruptorPacer struct {
	*pacer.DisruptorEngine
}

// NewDisruptorPacer creates a new DisruptorPacer with the given configuration.
func NewDisruptorPacer(conf DisruptorPacerConfig) (*DisruptorPacer, error) {
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
	if conf.Logic.ToDuration == nil {
		conf.Logic.ToDuration = TicksToDuration
	}
	seqThreshold := conf.SeqThreshold
	if seqThreshold == 0 {
		seqThreshold = 1
	}
	tsThreshold := conf.TsThreshold.Duration()
	if tsThreshold == 0 {
		tsThreshold = time.Second
	}

	stats := &pacer.InStats{}
	sched := &rtpScheduler{
		logic:       pacer.NewPacerLogic(conf.Logic, stats),
		gapDetector: NewGapDetector(seqThreshold, tsThreshold),
		stats:       stats,
		eventLog:    conf.EventLog,
		stream:      conf.Stream,
	}
	engine, err := pacer.NewDisruptorEngine(conf.engineConfig(), sched)
	if err != nil {
		return nil, errors.E("NewDisruptorPacer", err)
	}
	return &DisruptorPacer{DisruptorEngine: engine}, nil
}

// rtpScheduler is the RTP pacer.PacketScheduler: it unmarshals RTP packets, detects sequence/timestamp gaps, and
// computes target delivery times via PacerLogic. It is accessed only from the engine's Push goroutine, under the
// engine's input-stats lock.
type rtpScheduler struct {
	logic       *pacer.PacerLogic
	gapDetector *GapDetector
	stats       *pacer.InStats
	eventLog    elog.ILog
	stream      string
}

var _ pacer.PacketScheduler = (*rtpScheduler)(nil)

func (s *rtpScheduler) InStats() *pacer.InStats { return s.stats }

// ResetSource drops the timing baseline and the gap detector's unwrapping state, whose sequence numbers and timestamps
// belong to the previous source and would otherwise be extended by the new source's values.
func (s *rtpScheduler) ResetSource() {
	s.logic.ResetSource()
	s.gapDetector.Sequence = SequenceUnwrapper{}
	s.gapDetector.Timestamp = TimestampUnwrapper{}
}

func (s *rtpScheduler) Schedule(now utc.UTC, bts []byte) (utc.UTC, []byte, bool, error) {
	// Use a stack-local Packet so escape analysis keeps it off the heap. ParsePacket returns *rtp.Packet, which forces
	// a heap allocation on every call; inlining the unmarshal here eliminates that alloc in the steady-state path.
	var pkt pionrtp.Packet
	if err := pkt.Unmarshal(bts); err != nil {
		return utc.Zero, nil, false, errors.E("rtpScheduler.Schedule", errors.K.Invalid, err)
	}

	seq, ts, gapErr := s.gapDetector.Detect(pkt.SequenceNumber, pkt.Timestamp)
	if gapErr != nil {
		s.eventLog.Warn("gap", "stream", s.stream, gapErr)
	}
	target, discard, err := s.logic.Packet(now, ts, gapErr != nil)
	// Update RTP-specific stats after Packet (which may have reset the stats via reset()).
	s.stats.Rtp.Seq = pkt.SequenceNumber
	s.stats.Rtp.Sequ = seq
	s.stats.Rtp.Ts = pkt.Timestamp
	s.stats.Rtp.Tsu = ts
	if err != nil {
		return utc.Zero, nil, false, errors.E("rtpScheduler.Schedule", err)
	}
	if discard {
		return utc.Zero, nil, true, nil
	}
	return target, bts, false, nil
}
