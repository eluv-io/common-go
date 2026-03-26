package rtp

import (
	"context"
	"sync"
	"time"

	pionrtp "github.com/pion/rtp"
	"github.com/smarty/go-disruptor"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/ifutil"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

const (
	// MaxDisruptorCapacity is the max capacity of the ring buffer. The disruptor uses uint32 for sequence numbers and
	// the capacity must be a power of 2, so the largest power of 2 smaller than MaxUint (1<<32-1) is 1<<31.
	MaxDisruptorCapacity = 1 << 31

	// DefaultDisruptorCapacity is the default ring buffer capacity. Must be a power of 2.
	DefaultDisruptorCapacity = 1 << 12 // 4096 slots

	// DefaultDeliveryMargin is the default delivery margin. 0 = disabled, which means that packets may be sent with a
	// targetTs in the past.
	DefaultDeliveryMargin = 0

	// DefaultMinSleepThreshold is the default minimum sleep threshold. Sleep durations shorter than this are skipped.
	DefaultMinSleepThreshold = 5 * duration.Millisecond

	// DefaultTickerPeriod is the default ticker period used to schedule packet delivery. A ticker avoids the
	// per-packet timer allocation of time.After and supports prompt Shutdown interruption.
	DefaultTickerPeriod = duration.Millisecond

	// DefaultStatsInterval is the default interval for periodic stats logging.
	DefaultStatsInterval = 5 * duration.Second

	// DefaultOversleepThreshold is the minimum oversleep duration that is recorded in the oversleeps stat. Oversleeps
	// shorter than this are considered normal scheduler jitter and are not tracked.
	DefaultOversleepThreshold = 5 * duration.Millisecond
)

// disruptorEntry is a pre-allocated ring buffer slot. The entry is populated by the producer and read by the consumer.
type disruptorEntry struct {
	targetTs utc.UTC // target wall clock time when to send the packet
	inTs     utc.UTC // wall clock time when the packet was written to the ring buffer
	pkt      []byte  // the RTP packet bytes
}

// DisruptorPacerConfig holds configuration for a DisruptorPacer.
type DisruptorPacerConfig struct {
	Stream   string    // Stream is the stream name for logging.
	StatsLog elog.ILog // StatsLog is the logger to use for stats logging. If nil, stats are not logged.
	EventLog elog.ILog // EventLog is the logger to use for event logging. If nil, events are not logged.

	Logic             PacerLogicConfig // timing logic configuration
	BufferCapacity    int              // ring buffer capacity (is rounded up to the next power of 2; 0 → DefaultDisruptorCapacity)
	MinSleepThreshold duration.Spec    // sleep durations shorter than this are skipped (0 → DefaultMinSleepThreshold)
	TickerPeriod      duration.Spec    // ticker period for scheduling delivery (0 → DefaultTickerPeriod)
	StatsInterval     duration.Spec    // interval for periodic stats logging (0 → DefaultStatsInterval, -1 → disabled)

	// QueueAhead is how early the consumer dispatches a packet before its target time. The ticker loop wakes up when
	// now >= targetTs - QueueAhead, giving the "deliver" callback a lead-time window.
	// 0 = dispatch at targetTs.
	QueueAhead duration.Spec

	// DeliveryMargin is the minimum lead time guaranteed to the "deliver" callback:
	//   sendAt = max(targetTs, now + DeliveryMargin)
	// Packets that cannot satisfy this floor (targetTs already too close to now) are tracked as LateSends.
	// Should be ≤ QueueAhead so the floor is reliably reachable under normal conditions. 0 = disabled.
	DeliveryMargin duration.Spec
}

// DisruptorPacer is an RTP callback pacer that uses a lock-free disruptor ring buffer as the jitter buffer. It uses
// PacerLogic for timestamp calculations and target-time scheduling. The ring buffer replaces the Go channel used by
// RtpPacer, trading simplicity for lower and more consistent per-slot overhead.
//
// Usage:
//
//	pacer, _ := NewDisruptorPacer(conf)
//	go func() {
//	    err := pacer.Run(func(pkt []byte, at time.Time) error { ... })
//	}()
//	for _, pkt := range packets {
//	    pacer.Push(pkt)
//	}
//	pacer.Shutdown()
type DisruptorPacer struct {
	conf     DisruptorPacerConfig
	logic    *PacerLogic
	stats    InStats
	outStats OutStats

	// outStatsMu guards outStats between Handle() (per-packet UpdateNow calls) and logStats() (forced period close).
	// inStatsMu guards p.stats (InStats) between Push() (updated via p.logic.Packet()) and logStats() (snapshot read).
	// Both mutexes are uncontended in the fast path; logStats() holds each for ~100ns once per StatsInterval.
	outStatsMu sync.Mutex
	inStatsMu  sync.Mutex

	ringBuffer   []disruptorEntry
	bufferMask   int64
	dis          disruptor.Disruptor
	handler      *disruptorHandler
	ctx          context.Context
	cancel       context.CancelCauseFunc
	shutdownOnce sync.Once
}

// NewDisruptorPacer creates a new DisruptorPacer with the given configuration.
func NewDisruptorPacer(conf DisruptorPacerConfig) (*DisruptorPacer, error) {
	if conf.BufferCapacity <= 0 {
		conf.BufferCapacity = DefaultDisruptorCapacity
	} else if conf.BufferCapacity > MaxDisruptorCapacity {
		return nil, errors.E("NewDisruptorPacer",
			"reason", "buffer capacity too large",
			"max", MaxDisruptorCapacity,
			"actual", conf.BufferCapacity,
		)
	}
	if conf.BufferCapacity&(conf.BufferCapacity-1) != 0 {
		// Round up to the next power of 2. Shifts go up to >>32 to cover the full int64 range even though
		// MaxDisruptorCapacity (1<<31) currently bounds the input; the extra shift is a no-op in practice.
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
		conf.MinSleepThreshold = DefaultMinSleepThreshold
	}
	if conf.TickerPeriod <= 0 {
		conf.TickerPeriod = DefaultTickerPeriod
	}
	if conf.StatsInterval == 0 {
		conf.StatsInterval = DefaultStatsInterval
	}
	if conf.DeliveryMargin < 0 {
		conf.DeliveryMargin = DefaultDeliveryMargin
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

	ctx, cancel := context.WithCancelCause(context.Background())
	p := &DisruptorPacer{
		conf:       conf,
		outStats:   newOutStats(conf.StatsInterval),
		ringBuffer: make([]disruptorEntry, conf.BufferCapacity),
		bufferMask: int64(conf.BufferCapacity - 1),
		ctx:        ctx,
		cancel:     cancel,
	}
	p.logic = NewPacerLogic(conf.Logic, &p.stats)

	handler := &disruptorHandler{pacer: p}
	dis, err := disruptor.New(
		disruptor.Options.BufferCapacity(uint32(conf.BufferCapacity)),
		disruptor.Options.NewHandlerGroup(handler),
	)
	if err != nil {
		cancel(err)
		return nil, errors.E("NewDisruptorPacer", err)
	}
	p.dis = dis
	p.handler = handler
	return p, nil
}

// Push parses the RTP packet, computes its target transmission time via PacerLogic, and writes it into the ring buffer.
// It returns an error if the pacer has been shut down or the packet is invalid. Packets in the discard phase are
// silently dropped (nil returned). Push must be called from a single goroutine.
func (p *DisruptorPacer) Push(bts []byte) error {
	if p.ctx.Err() != nil {
		return errors.E("DisruptorPacer.Push", errors.K.Cancelled, context.Cause(p.ctx))
	}

	// Use a stack-local Packet so escape analysis keeps it off the heap. ParsePacket returns *rtp.Packet, which forces
	// a heap allocation on every call; inlining the unmarshal here eliminates that alloc in the steady-state path.
	var pkt pionrtp.Packet
	if err := pkt.Unmarshal(bts); err != nil {
		return errors.E("DisruptorPacer.Push", errors.K.Invalid, err)
	}

	now := utc.Now()
	// Hold inStatsMu around Packet() so that logStats() can take a consistent snapshot of p.stats.
	p.inStatsMu.Lock()
	target, discard, err := p.logic.Packet(now, pkt.SequenceNumber, pkt.Timestamp)
	p.inStatsMu.Unlock()
	if err != nil {
		return errors.E("DisruptorPacer.Push", err)
	}
	if discard {
		return nil
	}

	// Reserve one slot; blocks (spin-waits) if the ring buffer is full.
	seq := p.dis.Reserve(1)
	entry := &p.ringBuffer[seq&p.bufferMask]
	entry.targetTs = target
	entry.inTs = now
	// Copy packet bytes; the caller's buffer may be reused after Push returns.
	if cap(entry.pkt) >= len(bts) {
		entry.pkt = entry.pkt[:len(bts)]
	} else {
		entry.pkt = make([]byte, len(bts))
	}
	copy(entry.pkt, bts)
	p.outStats.buffered.Add(1)
	p.dis.Commit(seq, seq)
	return nil
}

// Run starts the consumer loop and calls deliver for each packet at its scheduled time. It blocks until the pacer is
// shut down via Shutdown. deliver is called sequentially from a single goroutine. The at parameter is the scheduled
// delivery time, but no less than now + DeliveryMargin. The provided []byte will be re-used after the call to deliver
// returns - make a copy if needed to avoid data races.
//
// Run starts the consumer loop and calls the deliver function for each pushed packet at its scheduled target time less
// the "queue ahead period". Run blocks until the pacer is shut down via Shutdown().
//
// The deliver function is called sequentially from a single goroutine. The []byte is the packet paylod and will be
// re-used after the deliver call returns - make a copy if needed to avoid data races. The target time is passed as the
// second parameter.
func (p *DisruptorPacer) Run(deliver func(bts []byte, at utc.UTC) error) error {
	p.handler.deliver = deliver
	p.handler.ticker = time.NewTicker(p.conf.TickerPeriod.Duration())
	p.handler.lastTick = time.Now() // simulated first tick
	defer p.handler.ticker.Stop()

	// logStats goroutine: the sole logging goroutine. Runs independently of packet flow.
	if p.conf.StatsInterval > 0 {
		go p.logStats()
	}

	p.dis.Listen() // blocks until dis.Close() is called
	return context.Cause(p.ctx)
}

// Shutdown stops the pacer. Any in-progress sleep in the consumer is interrupted. Idempotent.
func (p *DisruptorPacer) Shutdown(err ...error) {
	p.shutdownOnce.Do(func() {
		p.cancel(ifutil.FirstOrDefault[error](
			err,
			errors.NoTrace("DisruptorPacer.Shutdown", errors.K.Cancelled, "reason", "pacer shutdown"),
		))
		_ = p.dis.Close()
	})
}

// BufferCap returns the actual ring buffer capacity, which is the configured capacity rounded up to the next power
// of 2.
func (p *DisruptorPacer) BufferCap() int {
	return len(p.ringBuffer)
}

// disruptorHandler implements disruptor.MessageHandler and is the consumer side of the ring buffer. For each batch
// [lower, upper] it waits until each packet's scheduled time, then calls deliver.
type disruptorHandler struct {
	pacer    *DisruptorPacer
	deliver  func(bts []byte, at utc.UTC) error
	ticker   *time.Ticker
	lastTick time.Time
}

func (h *disruptorHandler) Handle(lower, upper int64) {
	for seq := lower; seq <= upper; seq++ {
		now := utc.Now()
		entry := &h.pacer.ringBuffer[seq&h.pacer.bufferMask]
		os := &h.pacer.outStats

		// Sleep until QueueAhead before targetTs, counting ticker ticks consumed.
		wakeTarget := entry.targetTs.Add(-h.pacer.conf.QueueAhead.Duration())
		wait := wakeTarget.Sub(now)
		var ticksConsumed int
		var overslept duration.Millis
		if wait > h.pacer.conf.MinSleepThreshold.Duration() {
			// Wake up QueueAhead before targetTs so the deliver callback has a look-ahead scheduling window.
			// Using a ticker avoids the per-packet timer allocation of time.After and lets ctx.Done() interrupt
			// a long wait promptly.
			for wakeTarget.Mono().After(h.lastTick) {
				select {
				case h.lastTick = <-h.ticker.C:
					ticksConsumed++
				case <-h.pacer.ctx.Done():
					return
				}
			}
			// Measure whether we overslept
			if ticksConsumed > 0 {
				now = utc.Now()
				overslept = duration.Millis(now.Sub(wakeTarget))
			}

		}

		if h.pacer.ctx.Err() != nil {
			return
		}

		// Decrement the buffered counter atomically; the value feeds into bufFill stats under the lock below.
		// Note: technically, the packet is still buffered until after it's wakeTarget is reached.
		bufFill := os.buffered.Add(-1)

		// lateness is how much the actual send time (sendAt) falls short of the ideal target time. If a packet is "on
		// time", lateness is 0.
		var lateness duration.Millis

		// Compute the actual send time:
		// * When DeliveryMargin=0: sendAt = targetTs (packets may be sent with a target ts in the past).
		// * Otherwise:             sendAt = max(targetTs, now+DeliveryMargin)
		sendAt := entry.targetTs
		minSendAt := now.Add(h.pacer.conf.DeliveryMargin.Duration())
		if sendAt.Before(minSendAt) {
			lateness = duration.Millis(minSendAt.Sub(sendAt))
			if h.pacer.conf.DeliveryMargin > 0 {
				// correct targetTs to the minimum send time only if a delivery margin is configured. Otherwise, we send
				// the packet with a taget ts in the past (and let the "deliver" callback deal with it).
				sendAt = minSendAt
			}
		}

		sendAhead := duration.Millis(sendAt.Sub(now))

		// Update all outStats under the mutex so that logStats() always observes a consistent view when it
		// forces a period close. The lock is held only for the fast UpdateNow calls — never during sleeps.
		h.pacer.outStatsMu.Lock()
		{
			os.bufFill.UpdateNow(now, bufFill)
			if duration.Spec(overslept) > DefaultOversleepThreshold {
				os.oversleeps.UpdateNow(now, overslept)
			}
			if lateness > 0 {
				os.lateness.UpdateNow(now, lateness)
			}
			os.sendAhead.UpdateNow(now, sendAhead)
			if os.lastPacket.IsZero() {
				// Artificial update to initialise ipd with the same timestamp as all other stats. Results in
				// min IPD of 0, but only for the very first period.
				os.ipd.UpdateNow(now, 0)
			} else {
				_ = os.ipd.UpdateNow(now, duration.Millis(now.Sub(os.lastPacket)))
			}
			os.lastPacket = now
			os.chd.UpdateNow(now, duration.Millis(now.Sub(entry.inTs)))
			if wait > 0 {
				os.wait.UpdateNow(now, duration.Millis(wait))
			}
			os.sleeps += ticksConsumed
		}
		h.pacer.outStatsMu.Unlock()

		if err := h.deliver(entry.pkt, sendAt); err != nil {
			h.pacer.conf.EventLog.Warn("deliver error",
				"stream", h.pacer.conf.Stream,
				"err", err)
		}
	}
}

// logStats is the sole logging goroutine. It fires every StatsInterval, forces a period close on both outStats
// and stats (InStats) under their respective mutexes, and logs a full snapshot — even during silence (where the
// snapshot has Count=0 for all Statistics fields, indicating no traffic in that period).
func (p *DisruptorPacer) logStats() {
	t := time.NewTicker(p.conf.StatsInterval.Duration())
	defer t.Stop()
	for {
		select {
		case <-t.C:
			now := utc.Now()

			p.inStatsMu.Lock()
			inSnap := p.stats // plain value copy; InStats has no atomics or sync values
			p.inStatsMu.Unlock()

			p.outStatsMu.Lock()
			outSnap := p.outStats.switchPeriod(now)
			p.outStatsMu.Unlock()

			p.conf.StatsLog.Info("stats",
				"stream", p.conf.Stream,
				"out", jsonutil.Stringer(outSnap),
				"in", jsonutil.Stringer(&inSnap))
		case <-p.ctx.Done():
			return
		}
	}
}

func (p *DisruptorPacer) Stats() (InStats, OutStatsPeriod) {
	p.inStatsMu.Lock()
	inSnap := p.stats // plain value copy; InStats has no atomics or sync values
	p.inStatsMu.Unlock()

	p.outStatsMu.Lock()
	outSnap := p.outStats.total()
	p.outStatsMu.Unlock()

	return inSnap, *outSnap
}
