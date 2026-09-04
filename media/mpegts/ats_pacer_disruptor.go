package mpegts

import (
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

// DefaultAtsGapThreshold is the default AtsDisruptorPacerConfig.GapThreshold: the maximum jump between consecutive
// arrival timestamps before a stream reset (baseline re-establishment) is triggered.
const DefaultAtsGapThreshold = duration.Second

// AtsDisruptorPacerConfig holds configuration for an AtsDisruptorPacer.
type AtsDisruptorPacerConfig struct {
	Stream   string    `json:"-"` // Stream is the stream name for logging.
	StatsLog elog.ILog `json:"-"` // StatsLog is the logger to use for stats logging. If nil, stats are not logged.
	EventLog elog.ILog `json:"-"` // EventLog is the logger to use for event logging. If nil, events are not logged.

	// Logic holds timing logic configuration. ToDuration will be overridden to the identity conversion (arrival
	// timestamps are already wall-clock nanoseconds); callers cannot change it.
	Logic pacer.PacerLogicConfig `json:"logic"`

	// GapThreshold is the maximum jump between consecutive arrival timestamps before a stream reset is triggered.
	// Defaults to DefaultAtsGapThreshold when zero; a negative value disables gap detection (arrival gaps of any size
	// are then reproduced faithfully).
	GapThreshold duration.Spec `json:"gap_threshold"`

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

	// MaxBlock caps how long a Push may block on a full ring buffer before the packet is dropped instead. See
	// pacer.DisruptorEngineConfig.MaxBlock. 0, the default, waits indefinitely.
	MaxBlock duration.Spec `json:"max_block"`
}

func (c *AtsDisruptorPacerConfig) InitDefaults() *AtsDisruptorPacerConfig {
	c.Logic.InitDefaults()
	c.GapThreshold = DefaultAtsGapThreshold
	c.BufferCapacity = pacer.DefaultDisruptorCapacity
	c.MinSleepThreshold = pacer.DefaultMinSleepThreshold
	c.TickerPeriod = pacer.DefaultTickerPeriod
	c.OversleepMargin = pacer.DefaultOversleepMargin
	c.StatsInterval = pacer.DefaultStatsInterval
	c.SendAhead = 0
	c.DeliveryMargin = 0
	c.MaxBlock = 0
	return c
}

// engineConfig maps the protocol-independent knobs onto a pacer.DisruptorEngineConfig.
func (c *AtsDisruptorPacerConfig) engineConfig() pacer.DisruptorEngineConfig {
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

// AtsDisruptorPacer is a callback pacer for Arrival-Time-Stamped MPEG-TS (ATS-TS): each pushed packet carries an 8-byte
// big-endian arrival timestamp (int64 nanoseconds since the Unix epoch, see AtsTimestampLen) followed by the raw TS
// packets of a single received datagram. The pacer reproduces the original reception timing by pacing on the arrival
// timestamp, and delivers only the TS payload (the timestamp prefix is consumed for scheduling).
//
// Because arrival timestamps are already absolute wall-clock nanoseconds, the timing logic uses an identity clock
// conversion, which lets PacerLogic's de-jitter delay and drift-correction machinery apply unchanged. All
// protocol-independent machinery (ring buffer, consumer loop, stats, lifecycle) lives in the embedded
// pacer.DisruptorEngine; this type only supplies the arrival-timestamp scheduling via atsScheduler.
//
// Usage:
//
//	pacer, _ := NewAtsDisruptorPacer(conf)
//	go func() {
//	    err := pacer.Run(func(pkt []byte, at utc.UTC) error { ... })
//	}()
//	for _, pkt := range packets {
//	    pacer.Push(pkt) // pkt = [8-byte arrival ns][TS packets]
//	}
//	pacer.Shutdown()
var _ pacer.StatsReporter = (*AtsDisruptorPacer)(nil)

type AtsDisruptorPacer struct {
	*pacer.DisruptorEngine
}

// NewAtsDisruptorPacer creates a new AtsDisruptorPacer with the given configuration.
func NewAtsDisruptorPacer(conf AtsDisruptorPacerConfig) (*AtsDisruptorPacer, error) {
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
	// Arrival timestamps are already wall-clock nanoseconds, so the clock conversion is the identity. Callers cannot
	// change this.
	conf.Logic.ToDuration = func(ts int64) time.Duration { return time.Duration(ts) }
	if conf.GapThreshold == 0 {
		conf.GapThreshold = DefaultAtsGapThreshold
	}

	stats := &pacer.InStats{}
	sched := &atsScheduler{
		logic:        pacer.NewPacerLogic(conf.Logic, stats),
		stats:        stats,
		gapThreshold: conf.GapThreshold.Duration().Nanoseconds(),
		eventLog:     conf.EventLog,
		stream:       conf.Stream,
	}
	engine, err := pacer.NewDisruptorEngine(conf.engineConfig(), sched)
	if err != nil {
		return nil, errors.E("NewAtsDisruptorPacer", err)
	}
	return &AtsDisruptorPacer{DisruptorEngine: engine}, nil
}

// atsScheduler is the ATS-TS pacer.PacketScheduler. It reads the 8-byte arrival-timestamp prefix from each packet and
// computes target delivery times via PacerLogic (with an identity clock conversion). All fields are accessed only from
// the engine's Push goroutine, under the engine's input-stats lock.
type atsScheduler struct {
	logic *pacer.PacerLogic
	stats *pacer.InStats

	gapThreshold int64 // maximum arrival-timestamp jump (nanoseconds) before a reset; ≤0 disables gap detection
	lastArrival  int64 // arrival timestamp of the previous packet (nanoseconds)
	haveLast     bool  // whether lastArrival has been set

	eventLog elog.ILog
	stream   string
}

var _ pacer.PacketScheduler = (*atsScheduler)(nil)

func (s *atsScheduler) InStats() *pacer.InStats { return s.stats }

// ResetSource drops the timing baseline and the previous source's last arrival timestamp, so the first packet of the
// new source is not measured as a gap against it.
func (s *atsScheduler) ResetSource() {
	s.logic.ResetSource()
	s.lastArrival = 0
	s.haveLast = false
}

func (s *atsScheduler) Schedule(now utc.UTC, bts []byte) (utc.UTC, []byte, bool, error) {
	arrival, payload, err := ParseAtsTs(bts)
	if err != nil {
		return utc.Zero, nil, false, errors.E("atsScheduler.Schedule", err)
	}

	gap := false
	if s.haveLast && s.gapThreshold > 0 {
		diff := arrival - s.lastArrival
		if diff < 0 {
			diff = -diff
		}
		if diff > s.gapThreshold {
			gap = true
			s.eventLog.Warn("arrival gap",
				"stream", s.stream,
				"prev_ns", s.lastArrival,
				"curr_ns", arrival,
				"diff", duration.Spec(time.Duration(arrival-s.lastArrival)),
				"threshold", duration.Spec(time.Duration(s.gapThreshold)))
		}
	}
	s.lastArrival = arrival
	s.haveLast = true

	target, discard, err := s.logic.Packet(now, arrival, gap)
	// Update the arrival stat after Packet (which may have reset the stats via reset() on a gap).
	s.stats.Ats.ArrivalNs = arrival
	if err != nil {
		return utc.Zero, nil, false, errors.E("atsScheduler.Schedule", err)
	}
	if discard {
		return utc.Zero, nil, true, nil
	}
	return target, payload, false, nil
}
