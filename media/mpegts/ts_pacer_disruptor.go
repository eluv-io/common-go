package mpegts

import (
	"context"
	"sync"
	"time"

	"github.com/Comcast/gots/v2/packet"
	"github.com/smarty/go-disruptor"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/pacer"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/common-go/util/ifutil"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

const DefaultPcrGapThreshold = duration.Second

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

	BufferCapacity    int           `json:"buffer_capacity"`     // ring buffer capacity (rounded up to next power of 2; 0 → rtp.DefaultDisruptorCapacity)
	MinSleepThreshold duration.Spec `json:"min_sleep_threshold"` // sleep durations shorter than this are skipped (0 → rtp.DefaultMinSleepThreshold)
	TickerPeriod      duration.Spec `json:"ticker_period"`       // ticker period for scheduling delivery (0 → rtp.DefaultTickerPeriod)
	StatsInterval     duration.Spec `json:"stats_interval"`      // interval for periodic stats logging (0 → rtp.DefaultStatsInterval, -1 → disabled)

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
}

func (c *TsDisruptorPacerConfig) InitDefaults() *TsDisruptorPacerConfig {
	c.Logic.InitDefaults()
	c.PcrGapThreshold = DefaultPcrGapThreshold
	c.BufferCapacity = rtp.DefaultDisruptorCapacity
	c.MinSleepThreshold = rtp.DefaultMinSleepThreshold
	c.TickerPeriod = rtp.DefaultTickerPeriod
	c.StatsInterval = rtp.DefaultStatsInterval
	c.SendAhead = 0
	c.DeliveryMargin = 0
	c.StripRtp = true
	return c
}

// tsDisruptorEntry is a pre-allocated ring buffer slot.
type tsDisruptorEntry struct {
	targetTs utc.UTC // target wall clock time when to send the packet
	inTs     utc.UTC // wall clock time when the packet was written to the ring buffer
	pkt      []byte  // the TS packet bytes
}

// TsDisruptorPacer is an MPEG-TS callback pacer that uses a lock-free disruptor ring buffer as the jitter buffer. It
// uses PCR (Program Clock Reference, 27 MHz clock) for timestamp calculations and target-time scheduling. The first PCR
// found in each batch drives timing regardless of which PID carries it.
//
// Usage:
//
//	pacer, _ := NewTsDisruptorPacer(conf)
//	go func() {
//	    err := pacer.Run(func(pkt []byte, at time.Time) error { ... })
//	}()
//	for _, pkt := range packets {
//	    pacer.Push(pkt)
//	}
//	pacer.Shutdown()
var _ pacer.StatsReporter = (*TsDisruptorPacer)(nil)

type TsDisruptorPacer struct {
	conf    TsDisruptorPacerConfig
	logic   *pacer.PacerLogic // PCR timing logic; accessed only from Push goroutine (under inStatsMu for logStats)
	inStats pacer.InStats     // timing input stats; accessed only from Push goroutine (under inStatsMu for logStats)
	gapDet  PcrGapDetector    // PCR gap detector; accessed only from Push goroutine
	tsStats pacer.TsInStats   // last seen PCR value/PID; accessed only from Push goroutine (under inStatsMu for logStats)

	lastTarget     utc.UTC // most recent target (for no-PCR batches)
	lastPcrArrival utc.UTC // wall clock arrival time of the batch that set lastTarget

	// PCR rate estimation fields — accessed only from the Push goroutine; no locking required.
	lastPcrUnwrapped     int64 // unwrapped PCR value from the last non-discarded PCR batch
	estimatedPcrPerBatch int64 // estimated PCR ticks per batch; 0 = estimate not yet available
	noPcrBatchCount      int   // number of consecutive non-PCR batches since the last PCR batch

	outStats pacer.OutStats

	// outStatsMu guards outStats between the consumer goroutine and logStats.
	outStatsMu sync.Mutex
	// inStatsMu guards inStats and tsStats (updated by Push, read by logStats for snapshots).
	inStatsMu sync.Mutex
	// NOTE: both outStatsMu and inStatsMu are uncontended in the fast path; logStats holds each for ~100ns once per
	// StatsInterval.

	ringBuffer   []tsDisruptorEntry
	bufferMask   int64
	dis          disruptor.Disruptor
	handler      *tsDisruptorHandler
	ctx          context.Context
	cancel       context.CancelCauseFunc
	shutdownOnce sync.Once
}

// NewTsDisruptorPacer creates a new TsDisruptorPacer with the given configuration.
func NewTsDisruptorPacer(conf TsDisruptorPacerConfig) (*TsDisruptorPacer, error) {
	if conf.BufferCapacity <= 0 {
		conf.BufferCapacity = rtp.DefaultDisruptorCapacity
	} else if conf.BufferCapacity > rtp.MaxDisruptorCapacity {
		return nil, errors.E("NewTsDisruptorPacer",
			"reason", "buffer capacity too large",
			"max", rtp.MaxDisruptorCapacity,
			"actual", conf.BufferCapacity,
		)
	}
	if conf.BufferCapacity&(conf.BufferCapacity-1) != 0 {
		conf.BufferCapacity--
		conf.BufferCapacity |= conf.BufferCapacity >> 1
		conf.BufferCapacity |= conf.BufferCapacity >> 2
		conf.BufferCapacity |= conf.BufferCapacity >> 4
		conf.BufferCapacity |= conf.BufferCapacity >> 8
		conf.BufferCapacity |= conf.BufferCapacity >> 16
		conf.BufferCapacity |= conf.BufferCapacity >> 32
		conf.BufferCapacity++
	}
	if conf.MinSleepThreshold <= 0 {
		conf.MinSleepThreshold = rtp.DefaultMinSleepThreshold
	}
	if conf.TickerPeriod <= 0 {
		conf.TickerPeriod = rtp.DefaultTickerPeriod
	}
	if conf.StatsInterval == 0 {
		conf.StatsInterval = rtp.DefaultStatsInterval
	}
	if conf.DeliveryMargin < 0 {
		conf.DeliveryMargin = rtp.DefaultDeliveryMargin
	}
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

	ctx, cancel := context.WithCancelCause(context.Background())
	p := &TsDisruptorPacer{
		conf:       conf,
		gapDet:     PcrGapDetector{Threshold: DurationToPcr(conf.PcrGapThreshold.Duration())},
		outStats:   pacer.NewOutStats(conf.StatsInterval),
		ringBuffer: make([]tsDisruptorEntry, conf.BufferCapacity),
		bufferMask: int64(conf.BufferCapacity - 1),
		ctx:        ctx,
		cancel:     cancel,
	}
	p.logic = pacer.NewPacerLogic(conf.Logic, &p.inStats)

	handler := &tsDisruptorHandler{pacer: p}
	dis, err := disruptor.New(
		disruptor.Options.BufferCapacity(uint32(conf.BufferCapacity)),
		disruptor.Options.NewHandlerGroup(handler),
	)
	if err != nil {
		cancel(err)
		return nil, errors.E("NewTsDisruptorPacer", err)
	}
	p.dis = dis
	p.handler = handler
	return p, nil
}

// Push extracts PCR timing from the batch of TS packets and schedules the batch for delivery at the computed target
// time. Batches without PCR are scheduled by offsetting the last known target by the elapsed wall-clock time since
// that target was established; if EstimatePcrRate is enabled and an estimate is available, the estimated per-batch
// PCR tick rate is used instead to smooth input jitter. Batches arriving before any PCR has been seen are silently
// dropped. Push must be called from a single goroutine.
func (p *TsDisruptorPacer) Push(bts []byte) error {
	if p.ctx.Err() != nil {
		return errors.E("TsDisruptorPacer.Push", errors.K.Cancelled, context.Cause(p.ctx))
	}

	if p.conf.StripRtp {
		var err error
		bts, err = rtp.StripHeader(bts)
		if err != nil {
			return errors.E("TsDisruptorPacer.Push", errors.K.Invalid, "reason", "failed to strip RTP header", err)
		}
	}

	now := utc.Now()

	// Scan TS packets in the batch for the first PCR from any PID.
	var pcrFound bool
	var pcrValue uint64
	var pcrPid int
	for scan := bts; len(scan) >= packet.PacketSize; scan = scan[packet.PacketSize:] {
		// Cast slice to pointer directly to avoid copying 188 bytes into a local value that would escape to the heap
		// because its address is taken when calling ExtractPCR.
		pkt := (*packet.Packet)(scan[:packet.PacketSize])
		if pcr, ok := ExtractPCR(pkt); ok {
			pcrFound = true
			pcrValue = pcr
			pcrPid = pkt.PID()
			break
		}
	}

	var target utc.UTC
	if pcrFound {
		prev, curr, gap := p.gapDet.Detect(pcrValue)
		if gap {
			p.conf.EventLog.Warn("pcr gap",
				"stream", p.conf.Stream,
				"pid", pcrPid,
				"prev_pcru", prev,
				"curr_pcru", curr,
				"diff", curr-prev,
				"threshold", p.conf.PcrGapThreshold)
			if p.conf.EstimatePcrRate {
				// Invalidate the rate estimate across a gap: the PCR clock jumps discontinuously.
				p.estimatedPcrPerBatch = 0
				p.noPcrBatchCount = 0
			}
		}

		p.inStatsMu.Lock()
		p.tsStats.PCR = pcrValue
		p.tsStats.PCRu = curr
		p.tsStats.PID = pcrPid
		var discard bool
		var err error
		target, discard, err = p.logic.Packet(now, curr, gap)
		p.inStatsMu.Unlock()

		if err != nil {
			return errors.E("TsDisruptorPacer.Push", err)
		}
		if discard {
			return nil
		}
		if p.conf.EstimatePcrRate {
			// Update rate estimate from the interval between consecutive non-discarded PCR batches.
			// totalBatches counts the intervals covered: noPcrBatchCount non-PCR batches + this PCR batch.
			if !p.lastTarget.IsZero() {
				pcrDelta := curr - p.lastPcrUnwrapped
				totalBatches := int64(p.noPcrBatchCount + 1)
				if pcrDelta > 0 {
					p.estimatedPcrPerBatch = pcrDelta / totalBatches
				}
			}
			p.lastPcrUnwrapped = curr
			p.noPcrBatchCount = 0
		}
		p.lastTarget = target
		p.lastPcrArrival = now
	} else {
		// No PCR in this batch.
		if p.lastTarget.IsZero() {
			// No PCR seen yet — drop the batch; we cannot schedule it.
			return nil
		}
		if p.conf.EstimatePcrRate {
			// Always count no-PCR batches so the denominator is correct when the first estimate is computed.
			p.noPcrBatchCount++
		}
		if p.conf.EstimatePcrRate && p.estimatedPcrPerBatch > 0 {
			// Schedule using the estimated PCR tick rate to avoid propagating arrival-time jitter.
			target = p.lastTarget.Add(PcrToDuration(uint64(p.estimatedPcrPerBatch) * uint64(p.noPcrBatchCount)))
		} else {
			// Fall back to arrival-time offset (also used before the estimate is available).
			target = p.lastTarget.Add(now.Sub(p.lastPcrArrival))
		}
	}

	// Reserve one slot; blocks (spin-waits) if the ring buffer is full.
	seq := p.dis.Reserve(1)
	entry := &p.ringBuffer[seq&p.bufferMask]
	entry.targetTs = target
	entry.inTs = now
	if cap(entry.pkt) >= len(bts) {
		entry.pkt = entry.pkt[:len(bts)]
	} else {
		entry.pkt = make([]byte, len(bts))
	}
	copy(entry.pkt, bts)
	p.outStats.IncrBuffered()
	p.dis.Commit(seq, seq)
	return nil
}

// Run starts the consumer loop and calls deliver for each batch at its scheduled time. It blocks until the pacer is
// shut down via Shutdown. deliver is called sequentially from a single goroutine. The at parameter is the scheduled
// delivery time. The provided []byte will be re-used after the call to deliver returns — make a copy if needed.
func (p *TsDisruptorPacer) Run(deliver func(bts []byte, at utc.UTC) error) error {
	p.handler.deliver = deliver
	p.handler.ticker = time.NewTicker(p.conf.TickerPeriod.Duration())
	p.handler.lastTick = time.Now()
	defer p.handler.ticker.Stop()

	if p.conf.StatsInterval > 0 {
		go p.logStats()
	}

	p.dis.Listen()
	return context.Cause(p.ctx)
}

// Shutdown stops the pacer. Any in-progress sleep in the consumer is interrupted. Idempotent.
func (p *TsDisruptorPacer) Shutdown(err ...error) {
	p.shutdownOnce.Do(func() {
		p.cancel(ifutil.FirstOrDefault[error](
			err,
			errors.NoTrace("TsDisruptorPacer.Shutdown", errors.K.Cancelled, "reason", "pacer shutdown"),
		))
		_ = p.dis.Close()
	})
}

// BufferCap returns the actual ring buffer capacity.
func (p *TsDisruptorPacer) BufferCap() int {
	return len(p.ringBuffer)
}

// Stats implements pacer.StatsReporter, returning a snapshot of the current input and output statistics.
func (p *TsDisruptorPacer) Stats() pacer.PacerStats {
	p.inStatsMu.Lock()
	inSnap := p.inStats
	p.inStatsMu.Unlock()

	p.outStatsMu.Lock()
	outSnap := p.outStats.Total()
	p.outStatsMu.Unlock()

	return pacer.PacerStats{In: inSnap, Out: *outSnap}
}

// logStats is the sole logging goroutine. It fires every StatsInterval and logs a full snapshot.
func (p *TsDisruptorPacer) logStats() {
	t := time.NewTicker(p.conf.StatsInterval.Duration())
	defer t.Stop()
	for {
		select {
		case <-t.C:
			now := utc.Now()

			p.inStatsMu.Lock()
			inSnap := p.inStats
			tsSnap := p.tsStats
			p.inStatsMu.Unlock()

			p.outStatsMu.Lock()
			outSnap := p.outStats.SwitchPeriod(now)
			p.outStatsMu.Unlock()

			p.conf.StatsLog.Info("stats",
				"stream", p.conf.Stream,
				"out", jsonutil.Stringer(outSnap),
				"in", jsonutil.Stringer(inSnap),
				"ts", jsonutil.Stringer(tsSnap))
		case <-p.ctx.Done():
			return
		}
	}
}

// tsDisruptorHandler implements disruptor.MessageHandler and is the consumer side of the ring buffer.
type tsDisruptorHandler struct {
	pacer    *TsDisruptorPacer
	deliver  func(bts []byte, at utc.UTC) error
	ticker   *time.Ticker
	lastTick time.Time
}

func (h *tsDisruptorHandler) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		now := utc.Now()
		entry := &h.pacer.ringBuffer[seq&h.pacer.bufferMask]
		os := &h.pacer.outStats

		// Sleep until SendAhead before targetTs, counting ticker ticks consumed.
		wakeTarget := entry.targetTs.Time.Add(-h.pacer.conf.SendAhead.Duration())
		wait := wakeTarget.Sub(now.Time)
		var ticksConsumed int
		var overslept duration.Millis
		if wait > h.pacer.conf.MinSleepThreshold.Duration() {
			for wakeTarget.After(h.lastTick) {
				select {
				case h.lastTick = <-h.ticker.C:
					ticksConsumed++
				case <-h.pacer.ctx.Done():
					return
				}
			}
			if ticksConsumed > 0 {
				now = utc.Now()
				overslept = duration.Millis(now.Time.Sub(wakeTarget))
			}
		}

		if h.pacer.ctx.Err() != nil {
			return
		}

		bufFill := os.DecrBuffered()

		var lateness duration.Millis
		sendAt := entry.targetTs
		minSendAt := now.Add(h.pacer.conf.DeliveryMargin.Duration())
		if sendAt.Before(minSendAt) {
			lateness = duration.Millis(minSendAt.Sub(sendAt))
			if h.pacer.conf.DeliveryMargin > 0 {
				sendAt = minSendAt
			}
		}

		sendAhead := duration.Millis(sendAt.Sub(now))

		h.pacer.outStatsMu.Lock()
		{
			os.UpdateBufFill(now, bufFill)
			if duration.Spec(overslept) > rtp.DefaultOversleepThreshold {
				os.UpdateOversleeps(now, overslept)
			}
			if lateness > 0 {
				os.UpdateLateness(now, lateness)
			}
			os.UpdateSendAhead(now, sendAhead)
			os.UpdateIPD(now)
			os.UpdateJBD(now, entry.inTs)
			if wait > 0 {
				os.UpdateWait(now, duration.Millis(wait))
			}
			os.AddSleeps(ticksConsumed)
		}
		h.pacer.outStatsMu.Unlock()

		if err := h.deliver(entry.pkt, sendAt); err != nil {
			h.pacer.conf.EventLog.Warn("deliver error",
				"stream", h.pacer.conf.Stream,
				"err", err)
		}
	}
}
