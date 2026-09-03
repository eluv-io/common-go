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
	DefaultMinSleepThreshold = 5 * duration.MS

	// DefaultTickerPeriod is the default ticker period used to schedule packet delivery. A ticker avoids the
	// per-packet timer allocation of time.After and supports prompt Shutdown interruption.
	DefaultTickerPeriod = 5 * duration.MS

	// DefaultStatsInterval is the default interval for periodic stats logging.
	DefaultStatsInterval = 5 * duration.S

	// DefaultOversleepMargin is the default DisruptorEngineConfig.OversleepMargin: the scheduling jitter tolerated on
	// top of the unavoidable ticker quantization before a wake-up is recorded as an oversleep. The effective oversleep
	// threshold is TickerPeriod + OversleepMargin: because the consumer wakes on ticker ticks, a wake can legitimately
	// land up to one TickerPeriod past its target without any real scheduling overrun, so that quantization must not be
	// counted as an oversleep.
	DefaultOversleepMargin = 5 * duration.MS
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
}

// DisruptorEngineConfig holds the protocol-independent configuration of a DisruptorEngine.
type DisruptorEngineConfig struct {
	Stream   string    `json:"-"` // Stream is the stream name for logging.
	StatsLog elog.ILog `json:"-"` // StatsLog is the logger to use for stats logging. If nil, stats are not logged.
	EventLog elog.ILog `json:"-"` // EventLog is the logger to use for event logging. If nil, events are not logged.

	BufferCapacity    int               `json:"buffer_capacity"`     // ring buffer capacity (rounded up to next power of 2; 0 → DefaultDisruptorCapacity)
	MinSleepThreshold duration.Duration `json:"min_sleep_threshold"` // sleep durations shorter than this are skipped (0 → DefaultMinSleepThreshold)
	TickerPeriod      duration.Duration `json:"ticker_period"`       // ticker period for scheduling delivery (0 → DefaultTickerPeriod)
	OversleepMargin   duration.Duration `json:"oversleep_margin"`    // jitter tolerated above TickerPeriod before a wake is counted as an oversleep (0 → DefaultOversleepMargin)
	StatsInterval     duration.Duration `json:"stats_interval"`      // interval for periodic stats logging (0 → DefaultStatsInterval, -1 → disabled)

	// SendAhead is how early the consumer dispatches a packet before its target time. The ticker loop wakes up when
	// now >= targetTs - SendAhead, giving the "deliver" callback a lead-time window. 0 = dispatch at targetTs.
	SendAhead duration.Duration `json:"send_ahead"`

	// DeliveryMargin is the minimum lead time guaranteed to the "deliver" callback:
	//   sendAt = max(targetTs, now + DeliveryMargin)
	// Packets that cannot satisfy this floor (targetTs already too close to now) are tracked as lateness. Should be ≤
	// SendAhead so the floor is reliably reachable under normal conditions. 0 = disabled.
	DeliveryMargin duration.Duration `json:"delivery_margin"`
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

// enqueue reserves a ring buffer slot and copies the payload into it. It blocks (spin-waits) if the ring buffer is
// full.
func (e *DisruptorEngine) enqueue(now, target utc.UTC, payload []byte) {
	seq := e.dis.Reserve(1)
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
			if duration.Duration(overslept) > e.conf.TickerPeriod+e.conf.OversleepMargin {
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
