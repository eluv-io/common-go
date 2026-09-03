package pacer

import (
	"context"
	"sync"
	"time"

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
	DefaultTickerPeriod = 5 * duration.Millisecond

	// DefaultStatsInterval is the default interval for periodic stats logging.
	DefaultStatsInterval = 5 * duration.Second

	// DefaultOversleepMargin is the default DisruptorEngineConfig.OversleepMargin: the scheduling jitter tolerated on
	// top of the unavoidable ticker quantization before a wake-up is recorded as an oversleep. The effective oversleep
	// threshold is TickerPeriod + OversleepMargin: because the consumer wakes on ticker ticks, a wake can legitimately
	// land up to one TickerPeriod past its target without any real scheduling overrun, so that quantization must not be
	// counted as an oversleep.
	DefaultOversleepMargin = 5 * duration.Millisecond
)

// PacketScheduler converts a raw packet into a scheduling decision. Implementations hold the protocol-specific timing
// state (gap detector, PacerLogic, clock conversion) and the input statistics they populate. Schedule is called under
// the engine's input-stats lock, once per Push, from a single producer goroutine.
type PacketScheduler interface {
	// Schedule parses bts, updates the input stats (see InStats), and returns the target delivery time plus the
	// payload bytes to enqueue. The returned payload may alias bts; the engine copies it into the ring buffer. Return
	// discard=true to drop the packet silently (e.g. during the startup/discard phase or before timing is
	// established). On error the packet is neither enqueued nor discarded silently; the error is returned to Push.
	Schedule(now utc.UTC, bts []byte) (target utc.UTC, payload []byte, discard bool, err error)

	// InStats returns the pointer to the input statistics that Schedule populates. The engine reads it under its
	// input-stats lock for periodic logging and Stats() snapshots. The pointer must be stable for the scheduler's
	// lifetime, and the scheduler must mutate it only from within Schedule (which the engine calls under the lock).
	InStats() *InStats

	// ResetSource discards every piece of state tied to the stream being paced, so the next packet is treated as the
	// start of a new one: timing baseline, gap detection and any clock identity pinned from the first packets. The
	// engine calls it under its input-stats lock, from DisruptorEngine.ResetSource.
	//
	// This is for a caller that knowingly switches source, which gap detection cannot infer reliably: two sources may
	// differ by less than the gap threshold, or by so much that the difference reads as a clock wraparound.
	//
	// The new source still has to be located in time, so a discard phase runs and packets are withheld for its
	// duration - a visible gap, since output is already flowing. It runs on the shorter source-change periods where
	// those are configured, falling back to the startup ones otherwise. See PacerLogic.ResetSource.
	ResetSource()
}

// DisruptorEngineConfig holds the protocol-independent configuration of a DisruptorEngine.
type DisruptorEngineConfig struct {
	Stream   string    `json:"-"` // Stream is the stream name for logging.
	StatsLog elog.ILog `json:"-"` // StatsLog is the logger to use for stats logging. If nil, stats are not logged.
	EventLog elog.ILog `json:"-"` // EventLog is the logger to use for event logging. If nil, events are not logged.

	BufferCapacity    int           `json:"buffer_capacity"`     // ring buffer capacity (rounded up to next power of 2; 0 → DefaultDisruptorCapacity)
	MinSleepThreshold duration.Spec `json:"min_sleep_threshold"` // sleep durations shorter than this are skipped (0 → DefaultMinSleepThreshold)
	TickerPeriod      duration.Spec `json:"ticker_period"`       // ticker period for scheduling delivery (0 → DefaultTickerPeriod)
	OversleepMargin   duration.Spec `json:"oversleep_margin"`    // jitter tolerated above TickerPeriod before a wake is counted as an oversleep (0 → DefaultOversleepMargin)
	StatsInterval     duration.Spec `json:"stats_interval"`      // interval for periodic stats logging (0 → DefaultStatsInterval, -1 → disabled)

	// SendAhead is how early the consumer dispatches a packet before its target time. The ticker loop wakes up when
	// now >= targetTs - SendAhead, giving the "deliver" callback a lead-time window. 0 = dispatch at targetTs.
	SendAhead duration.Spec `json:"send_ahead"`

	// DeliveryMargin is the minimum lead time guaranteed to the "deliver" callback:
	//   sendAt = max(targetTs, now + DeliveryMargin)
	// Packets that cannot satisfy this floor (targetTs already too close to now) are tracked as lateness. Should be ≤
	// SendAhead so the floor is reliably reachable under normal conditions. 0 = disabled.
	DeliveryMargin duration.Spec `json:"delivery_margin"`

	// MaxBlock bounds how long Push waits for a free ring buffer slot before dropping the packet.
	//
	// A full ring does not free one slot at a time. The consumer is handed every packet committed since its last pass
	// and publishes its position only after pacing through all of them, so a producer that fills the ring waits for
	// the ring's entire contents to play out - seconds, at a high bitrate with a large capacity.
	//
	// 0, the default, waits indefinitely, which is the right choice when the producer can be slowed down: it becomes
	// backpressure. Set it for a producer that cannot, where falling permanently behind is worse than a gap in the
	// output. Either way the stall is counted and reported.
	MaxBlock duration.Spec `json:"max_block"`
}

// InitDefaults sets all fields to their default values.
func (c *DisruptorEngineConfig) InitDefaults() *DisruptorEngineConfig {
	c.BufferCapacity = DefaultDisruptorCapacity
	c.MinSleepThreshold = DefaultMinSleepThreshold
	c.TickerPeriod = DefaultTickerPeriod
	c.OversleepMargin = DefaultOversleepMargin
	c.StatsInterval = DefaultStatsInterval
	c.SendAhead = 0
	c.DeliveryMargin = DefaultDeliveryMargin
	return c
}

// normalize validates and fills in default values for zero-valued fields. It returns an error if BufferCapacity
// exceeds MaxDisruptorCapacity.
func (c *DisruptorEngineConfig) normalize() error {
	if c.BufferCapacity <= 0 {
		c.BufferCapacity = DefaultDisruptorCapacity
	} else if c.BufferCapacity > MaxDisruptorCapacity {
		return errors.E("DisruptorEngineConfig.normalize",
			"reason", "buffer capacity too large",
			"max", MaxDisruptorCapacity,
			"actual", c.BufferCapacity,
		)
	}
	c.BufferCapacity = roundUpPow2(c.BufferCapacity)
	if c.MinSleepThreshold <= 0 {
		c.MinSleepThreshold = DefaultMinSleepThreshold
	}
	if c.TickerPeriod <= 0 {
		c.TickerPeriod = DefaultTickerPeriod
	}
	if c.OversleepMargin <= 0 {
		c.OversleepMargin = DefaultOversleepMargin
	}
	if c.StatsInterval == 0 {
		c.StatsInterval = DefaultStatsInterval
	}
	if c.DeliveryMargin < 0 {
		c.DeliveryMargin = DefaultDeliveryMargin
	}
	if c.MaxBlock < 0 {
		c.MaxBlock = 0
	}
	if c.StatsLog == nil {
		c.StatsLog = elog.Noop
	}
	if c.EventLog == nil {
		c.EventLog = elog.Noop
	}
	return nil
}

// roundUpPow2 rounds n up to the next power of 2. Values that are already a power of 2 are returned unchanged. Shifts
// go up to >>32 to cover the full int64 range even though MaxDisruptorCapacity (1<<31) currently bounds the input; the
// extra shift is a no-op in practice.
func roundUpPow2(n int) int {
	if n&(n-1) == 0 {
		return n
	}
	n--
	n |= n >> 1
	n |= n >> 2
	n |= n >> 4
	n |= n >> 8
	n |= n >> 16
	n |= n >> 32
	n++
	return n
}

// disruptorEntry is a pre-allocated ring buffer slot. The entry is populated by the producer and read by the consumer.
type disruptorEntry struct {
	targetTs utc.UTC // target wall clock time when to send the packet
	inTs     utc.UTC // wall clock time when the packet was written to the ring buffer
	pkt      []byte  // the packet bytes
}

// DisruptorEngine is the protocol-independent core of a callback pacer. It uses a lock-free disruptor ring buffer as
// the jitter buffer and delegates all protocol-specific parsing and target-time computation to a PacketScheduler.
//
// The engine owns the ring buffer, the consumer loop (target-time scheduling and output-statistics accounting),
// Run/Shutdown lifecycle, and periodic stats logging. Protocol-specific pacers wrap the engine (typically by embedding
// *DisruptorEngine) and construct it with the appropriate scheduler.
//
// Usage:
//
//	engine, _ := pacer.NewDisruptorEngine(conf, scheduler)
//	go func() {
//	    err := engine.Run(func(pkt []byte, at utc.UTC) error { ... })
//	}()
//	for _, pkt := range packets {
//	    engine.Push(pkt)
//	}
//	engine.Shutdown()
var _ StatsReporter = (*DisruptorEngine)(nil)

type DisruptorEngine struct {
	conf  DisruptorEngineConfig
	sched PacketScheduler

	outStats OutStats

	// outStatsMu guards outStats between Handle() (per-packet UpdateNow calls) and logStats() (forced period close).
	// inStatsMu guards the scheduler's InStats between Push() (updated via sched.Schedule) and logStats()/Stats()
	// (snapshot reads). Both mutexes are uncontended in the fast path; logStats() holds each for ~100ns once per
	// StatsInterval.
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

// NewDisruptorEngine creates a new DisruptorEngine with the given configuration and scheduler.
func NewDisruptorEngine(conf DisruptorEngineConfig, sched PacketScheduler) (*DisruptorEngine, error) {
	if err := conf.normalize(); err != nil {
		return nil, errors.E("NewDisruptorEngine", err)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	e := &DisruptorEngine{
		conf:       conf,
		sched:      sched,
		outStats:   NewOutStats(conf.StatsInterval),
		ringBuffer: make([]disruptorEntry, conf.BufferCapacity),
		bufferMask: int64(conf.BufferCapacity - 1),
		ctx:        ctx,
		cancel:     cancel,
	}

	handler := &disruptorHandler{engine: e}
	dis, err := disruptor.New(
		disruptor.Options.BufferCapacity(uint32(conf.BufferCapacity)),
		disruptor.Options.NewHandlerGroup(handler),
	)
	if err != nil {
		cancel(err)
		return nil, errors.E("NewDisruptorEngine", err)
	}
	e.dis = dis
	e.handler = handler
	return e, nil
}

// Push hands the raw packet to the scheduler to compute its target transmission time, then writes it into the ring
// buffer. It returns an error if the engine has been shut down or the scheduler rejects the packet. Packets the
// scheduler discards (e.g. during the discard phase) are silently dropped (nil returned). Push must be called from a
// single goroutine.
func (e *DisruptorEngine) Push(bts []byte) error {
	if e.ctx.Err() != nil {
		return errors.E("DisruptorEngine.Push", errors.K.Cancelled, context.Cause(e.ctx))
	}

	now := utc.Now()
	// Hold inStatsMu around the scheduling decision so that logStats()/Stats() can take a consistent snapshot of the
	// scheduler's InStats.
	e.inStatsMu.Lock()
	target, payload, discard, err := e.sched.Schedule(now, bts)
	e.inStatsMu.Unlock()
	if err != nil {
		return errors.E("DisruptorEngine.Push", err)
	}
	if discard {
		return nil
	}

	e.enqueue(now, target, payload)
	return nil
}

// enqueue reserves a ring buffer slot and copies the payload into it. A packet dropped because the ring buffer stayed
// full for MaxBlock is counted and reported, not returned as an error: the caller cannot act on it, and treating it as
// a failure would tear down a stream that is otherwise healthy.
func (e *DisruptorEngine) enqueue(now, target utc.UTC, payload []byte) {
	seq := e.dis.TryReserve(1)
	if seq < 0 {
		// The stall is timed from here rather than from the packet's arrival: everything up to this point is
		// scheduling work, and charging it to MaxBlock would let a slow scheduler drop a packet that never actually
		// waited for a slot. It would overstate the reported Blocked time by the same amount.
		var ok bool
		if seq, ok = e.waitForSlot(utc.Now()); !ok {
			return
		}
	}
	entry := &e.ringBuffer[seq&e.bufferMask]
	entry.targetTs = target
	entry.inTs = now
	// Copy packet bytes; the caller's buffer (and the scheduler's returned payload, which may alias it) may be reused
	// after Push returns.
	if cap(entry.pkt) >= len(payload) {
		entry.pkt = entry.pkt[:len(payload)]
	} else {
		entry.pkt = make([]byte, len(payload))
	}
	copy(entry.pkt, payload)
	e.outStats.IncrBuffered()
	e.dis.Commit(seq, seq)
}

// waitForSlot polls for a free ring buffer slot until one appears, the engine shuts down, or MaxBlock elapses. It
// reports false when no slot was obtained and the packet must be dropped.
//
// It polls rather than using the disruptor's own blocking Reserve, whose wait strategy spins on a 1ns sleep and so
// burns a core for the whole stall. A slot can only appear when the consumer finishes pacing its current batch, so
// polling at the consumer's own scheduling granularity is as responsive as spinning, at no cost.
func (e *DisruptorEngine) waitForSlot(start utc.UTC) (seq int64, ok bool) {
	// A quarter of the consumer's ticker period, so a freed slot is picked up well within one of its wake-ups without
	// polling so often that the wait costs anything. Bounded below so a tiny TickerPeriod cannot turn this into a spin.
	poll := max(e.conf.TickerPeriod.Duration()/4, 100*time.Microsecond)
	maxBlock := e.conf.MaxBlock.Duration()

	timer := time.NewTimer(poll)
	defer timer.Stop()
	for {
		// Each wait is clamped to what is left of the budget, so a poll interval longer than MaxBlock cannot overshoot
		// it - a large TickerPeriod with a small MaxBlock would otherwise blow the cap on the very first sleep.
		wait := poll
		if maxBlock > 0 {
			remaining := maxBlock - utc.Now().Sub(start)
			if remaining <= 0 {
				e.reportStall(start, true)
				return 0, false
			}
			wait = min(wait, remaining)
		}

		// Waiting on the timer rather than sleeping, so shutdown is not held up for the rest of the interval.
		timer.Reset(wait)
		select {
		case <-e.ctx.Done():
			return 0, false
		case <-timer.C:
		}

		if seq = e.dis.TryReserve(1); seq >= 0 {
			e.reportStall(start, false)
			return seq, true
		}
	}
}

// reportStall records how long the producer waited for a free slot, and whether the packet was ultimately dropped. The
// log line is throttled: an overflow that persists produces one stall per packet, and the counters carry the volume.
func (e *DisruptorEngine) reportStall(start utc.UTC, dropped bool) {
	now := utc.Now()
	blocked := now.Sub(start)

	e.outStatsMu.Lock()
	e.outStats.UpdateBlocked(now, duration.Millis(blocked))
	if dropped {
		e.outStats.AddDropped(1)
	}
	e.outStatsMu.Unlock()

	e.conf.EventLog.Throttle("pacer-buffer-full").Warn("pacer buffer full",
		"stream", e.conf.Stream,
		"blocked", duration.Spec(blocked).RoundTo(2),
		"dropped", dropped,
		"buffer_capacity", e.conf.BufferCapacity)
}

// Run starts the consumer loop and calls deliver for each packet at its scheduled time. It blocks until the engine is
// shut down via Shutdown. deliver is called sequentially from a single goroutine. The at parameter is the scheduled
// delivery time (max(targetTs, now+DeliveryMargin)); SendAhead shortens the consumer's sleep but does not affect the
// value of at. The provided []byte will be re-used after the call to deliver returns — make a copy if needed to avoid
// data races.
func (e *DisruptorEngine) Run(deliver func(bts []byte, at utc.UTC) error) error {
	e.handler.deliver = deliver
	e.handler.ticker = time.NewTicker(e.conf.TickerPeriod.Duration())
	e.handler.lastTick = time.Now() // simulated first tick
	defer e.handler.ticker.Stop()

	// logStats goroutine: the sole logging goroutine. Runs independently of packet flow.
	if e.conf.StatsInterval > 0 {
		go e.logStats()
	}

	e.dis.Listen() // blocks until dis.Close() is called
	return context.Cause(e.ctx)
}

// Shutdown stops the engine. Any in-progress sleep in the consumer is interrupted. Idempotent.
func (e *DisruptorEngine) Shutdown(err ...error) {
	e.shutdownOnce.Do(func() {
		e.cancel(ifutil.FirstOrDefault[error](
			err,
			errors.NoTrace("DisruptorEngine.Shutdown", errors.K.Cancelled, "reason", "pacer shutdown"),
		))
		_ = e.dis.Close()
	})
}

// ResetSource tells the scheduler that subsequent packets come from a different source, so state tied to the previous
// one is dropped instead of being carried across the switch. See PacketScheduler.ResetSource.
//
// It must be called from the same goroutine as Push. It takes the input-stats lock, which is what makes it safe
// against a concurrent logStats()/Stats() snapshot.
func (e *DisruptorEngine) ResetSource() {
	e.inStatsMu.Lock()
	defer e.inStatsMu.Unlock()
	e.sched.ResetSource()
}

// BufferCap returns the actual ring buffer capacity, which is the configured capacity rounded up to the next power of 2.
func (e *DisruptorEngine) BufferCap() int {
	return len(e.ringBuffer)
}

// Stats implements StatsReporter, returning a snapshot of the current input and output statistics.
func (e *DisruptorEngine) Stats() PacerStats {
	e.inStatsMu.Lock()
	inSnap := *e.sched.InStats() // plain value copy; InStats has no atomics or sync values
	e.inStatsMu.Unlock()

	e.outStatsMu.Lock()
	outSnap := e.outStats.Total()
	e.outStatsMu.Unlock()

	return PacerStats{In: inSnap, Out: *outSnap}
}

// logStats is the sole logging goroutine. It fires every StatsInterval, forces a period close on outStats and reads a
// snapshot of the scheduler's InStats under their respective mutexes, and logs a full snapshot — even during silence
// (where the snapshot has Count=0 for all Statistics fields, indicating no traffic in that period).
func (e *DisruptorEngine) logStats() {
	t := time.NewTicker(e.conf.StatsInterval.Duration())
	defer t.Stop()
	for {
		select {
		case <-t.C:
			now := utc.Now()

			e.inStatsMu.Lock()
			inSnap := *e.sched.InStats() // plain value copy; InStats has no atomics or sync values
			e.inStatsMu.Unlock()

			e.outStatsMu.Lock()
			outSnap := e.outStats.SwitchPeriod(now)
			e.outStatsMu.Unlock()

			e.conf.StatsLog.Info("stats",
				"stream", e.conf.Stream,
				"out", jsonutil.Stringer(outSnap),
				"in", jsonutil.Stringer(&inSnap))
		case <-e.ctx.Done():
			return
		}
	}
}

// disruptorHandler implements disruptor.MessageHandler and is the consumer side of the ring buffer. For each batch
// [lower, upper] it waits until each packet's scheduled time, then calls deliver.
type disruptorHandler struct {
	engine   *DisruptorEngine
	deliver  func(bts []byte, at utc.UTC) error
	ticker   *time.Ticker
	lastTick time.Time
}

func (h *disruptorHandler) Handle(lower, upper int64) {
	e := h.engine
	for seq := lower; seq <= upper; seq++ {
		now := utc.Now()
		entry := &e.ringBuffer[seq&e.bufferMask]
		os := &e.outStats

		// Sleep until SendAhead before targetTs, counting ticker ticks consumed.
		wakeTarget := entry.targetTs.Add(-e.conf.SendAhead.Duration())
		wait := wakeTarget.Sub(now)
		var ticksConsumed int
		var overslept duration.Millis
		if wait > e.conf.MinSleepThreshold.Duration() {
			// Wake up SendAhead before targetTs so the deliver callback has a look-ahead scheduling window. Using a
			// ticker avoids the per-packet timer allocation of time.After and lets ctx.Done() interrupt a long wait
			// promptly.
			for wakeTarget.Mono().After(h.lastTick) {
				select {
				case h.lastTick = <-h.ticker.C:
					ticksConsumed++
				case <-e.ctx.Done():
					return
				}
			}
			// Measure whether we overslept.
			if ticksConsumed > 0 {
				now = utc.Now()
				overslept = duration.Millis(now.Sub(wakeTarget))
			}
		}

		if e.ctx.Err() != nil {
			return
		}

		// Decrement the buffered counter atomically; the value feeds into bufFill stats under the lock below. Note:
		// technically, the packet is still buffered until after its wakeTarget is reached.
		bufFill := os.DecrBuffered()

		// lateness is how much the actual send time (sendAt) falls short of the ideal target time. If a packet is "on
		// time", lateness is 0.
		var lateness duration.Millis

		// Compute the actual send time:
		// * When DeliveryMargin=0: sendAt = targetTs (packets may be sent with a target ts in the past).
		// * Otherwise:             sendAt = max(targetTs, now+DeliveryMargin)
		sendAt := entry.targetTs
		minSendAt := now.Add(e.conf.DeliveryMargin.Duration())
		if sendAt.Before(minSendAt) {
			lateness = duration.Millis(minSendAt.Sub(sendAt))
			if e.conf.DeliveryMargin > 0 {
				// Correct targetTs to the minimum send time only if a delivery margin is configured. Otherwise, we send
				// the packet with a target ts in the past (and let the "deliver" callback deal with it).
				sendAt = minSendAt
			}
		}

		sendAhead := duration.Millis(sendAt.Sub(now))

		// Update all outStats under the mutex so that logStats() always observes a consistent view when it forces a
		// period close. The lock is held only for the fast UpdateNow calls — never during sleeps.
		e.outStatsMu.Lock()
		{
			os.UpdateBufFill(now, bufFill)
			if duration.Spec(overslept) > e.conf.TickerPeriod+e.conf.OversleepMargin {
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
		e.outStatsMu.Unlock()

		if err := h.deliver(entry.pkt, sendAt); err != nil {
			e.conf.EventLog.Warn("deliver error",
				"stream", e.conf.Stream,
				"err", err)
		}
	}
}
