package mpegts

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/Comcast/gots/v2/packet"
	"github.com/smarty/go-disruptor"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/media/rtp"
	"github.com/eluv-io/common-go/util/ifutil"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/errors-go"
	elog "github.com/eluv-io/log-go"
	"github.com/eluv-io/utc-go"
)

// TsDisruptorPacerConfig holds configuration for a TsDisruptorPacer.
type TsDisruptorPacerConfig struct {
	Stream   string    // Stream is the stream name for logging.
	StatsLog elog.ILog // StatsLog is the logger to use for stats logging. If nil, stats are not logged.
	EventLog elog.ILog // EventLog is the logger to use for event logging. If nil, events are not logged.

	// Logic holds timing logic configuration. ToDuration will be overridden to PcrToDuration; RtpSeqThreshold and
	// RtpTsThreshold are unused for MPEG-TS pacing (PCR gap detection is handled separately via PcrGapThreshold).
	Logic rtp.PacerLogicConfig

	// PcrGapThreshold is the maximum PCR jump between consecutive PCR-bearing packets before a stream reset is
	// triggered. Defaults to 1 second when zero.
	PcrGapThreshold duration.Spec

	BufferCapacity    int           // ring buffer capacity (rounded up to next power of 2; 0 → rtp.DefaultDisruptorCapacity)
	MinSleepThreshold duration.Spec // sleep durations shorter than this are skipped (0 → rtp.DefaultMinSleepThreshold)
	TickerPeriod      duration.Spec // ticker period for scheduling delivery (0 → rtp.DefaultTickerPeriod)
	StatsInterval     duration.Spec // interval for periodic stats logging (0 → rtp.DefaultStatsInterval, -1 → disabled)

	// QueueAhead is how early the consumer dispatches a packet before its target time. 0 = dispatch at targetTs.
	QueueAhead duration.Spec

	// DeliveryMargin is the minimum lead time guaranteed to the "deliver" callback:
	//   sendAt = max(targetTs, now + DeliveryMargin)
	// Should be ≤ QueueAhead so the floor is reliably reachable under normal conditions. 0 = disabled.
	DeliveryMargin duration.Spec

	// StripRtp, when true, strips the RTP header from each incoming byte slice before extracting PCR.
	StripRtp bool
}

// TsInStats holds PCR-specific input statistics for a single PCR PID.
type TsInStats struct {
	PCR  uint64 `json:"pcr"`  // most recent raw PCR value
	PCRu int64  `json:"pcru"` // most recent unwrapped PCR value
	PID  int    `json:"pid"`  // PCR PID
}

// pidState holds per-PCR-PID timing state.
type pidState struct {
	logic   *rtp.PacerLogic
	inStats rtp.InStats
	gapDet  tsPcrGapDetector
	tsStats TsInStats
}

// tsDisruptorEntry is a pre-allocated ring buffer slot.
type tsDisruptorEntry struct {
	targetTs utc.UTC // target wall clock time when to send the packet
	inTs     utc.UTC // wall clock time when the packet was written to the ring buffer
	pkt      []byte  // the TS packet bytes
}

// TsDisruptorPacer is an MPEG-TS callback pacer that uses a lock-free disruptor ring buffer as the jitter buffer.
// It uses PCR (Program Clock Reference, 27 MHz clock) for timestamp calculations and target-time scheduling. Multiple
// PCR PIDs are supported; each PID maintains independent timing state.
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
type TsDisruptorPacer struct {
	conf       TsDisruptorPacerConfig
	pidStates  map[int]*pidState // keyed by PCR PID; accessed only from Push goroutine (under inStatsMu for logStats)
	lastTarget utc.UTC           // most recent target from any PID (for no-PCR batches)
	outStats   rtp.OutStats

	// outStatsMu guards outStats between the consumer goroutine and logStats.
	// inStatsMu guards pidStates (updated by Push via PacketTs, read by logStats for snapshots).
	// Both mutexes are uncontended in the fast path; logStats holds each for ~100ns once per StatsInterval.
	outStatsMu sync.Mutex
	inStatsMu  sync.Mutex

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
		conf.PcrGapThreshold = duration.Spec(time.Second)
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	p := &TsDisruptorPacer{
		conf:       conf,
		pidStates:  make(map[int]*pidState),
		outStats:   rtp.NewOutStats(conf.StatsInterval),
		ringBuffer: make([]tsDisruptorEntry, conf.BufferCapacity),
		bufferMask: int64(conf.BufferCapacity - 1),
		ctx:        ctx,
		cancel:     cancel,
	}

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
// time. If no PCR is found in the batch, the last known target time is reused. Batches arriving before any PCR has
// been seen are silently dropped. Push must be called from a single goroutine.
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
		pkt := packet.Packet(scan)
		if pcr, ok := ExtractPCR(&pkt); ok {
			pcrFound = true
			pcrValue = pcr
			pcrPid = pkt.PID()
			break
		}
	}

	var target utc.UTC
	if pcrFound {
		state := p.pidStateFor(pcrPid, now)

		prev, curr, gap := state.gapDet.detect(pcrValue)
		if gap {
			p.conf.EventLog.Warn("pcr gap",
				"stream", p.conf.Stream,
				"pid", pcrPid,
				"prev_pcru", prev,
				"curr_pcru", curr,
				"diff", curr-prev,
				"threshold", p.conf.PcrGapThreshold)
		}

		p.inStatsMu.Lock()
		state.tsStats.PCR = pcrValue
		state.tsStats.PCRu = curr
		state.tsStats.PID = pcrPid
		var discard bool
		var err error
		target, discard, err = state.logic.PacketTs(now, curr, gap)
		p.inStatsMu.Unlock()

		if err != nil {
			return errors.E("TsDisruptorPacer.Push", err)
		}
		if discard {
			return nil
		}
		p.lastTarget = target
	} else {
		// No PCR in this batch: reuse last known target so the batch is delivered at the same scheduled time as the
		// preceding PCR batch.
		target = p.lastTarget
		if target.IsZero() {
			// No PCR seen yet — drop the batch; we cannot schedule it.
			return nil
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

// pidStateFor returns the existing pidState for the given PID, or creates a new one. Must be called from the Push
// goroutine; concurrent access from logStats is guarded by inStatsMu only for stats reads, not for map writes.
func (p *TsDisruptorPacer) pidStateFor(pid int, now utc.UTC) *pidState {
	if state, ok := p.pidStates[pid]; ok {
		return state
	}
	state := &pidState{
		gapDet: tsPcrGapDetector{
			threshold: DurationToPcr(p.conf.PcrGapThreshold.Duration()),
		},
		tsStats: TsInStats{PID: pid},
	}
	state.logic = rtp.NewPacerLogic(p.conf.Logic, &state.inStats)
	p.conf.EventLog.Info("new PCR PID", "stream", p.conf.Stream, "pid", pid)
	p.pidStates[pid] = state
	_ = now
	return state
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

// Stats returns a snapshot of the current input and output statistics. The InStats returned is for the first PCR PID
// seen; use StatsForPID for per-PID stats.
func (p *TsDisruptorPacer) Stats() (rtp.InStats, rtp.OutStatsPeriod) {
	p.inStatsMu.Lock()
	var inSnap rtp.InStats
	for _, state := range p.pidStates {
		inSnap = state.inStats
		break
	}
	p.inStatsMu.Unlock()

	p.outStatsMu.Lock()
	outSnap := p.outStats.Total()
	p.outStatsMu.Unlock()

	return inSnap, *outSnap
}

// logStats is the sole logging goroutine. It fires every StatsInterval and logs a full snapshot.
func (p *TsDisruptorPacer) logStats() {
	t := time.NewTicker(p.conf.StatsInterval.Duration())
	defer t.Stop()
	for {
		select {
		case <-t.C:
			now := utc.Now()

			// Snapshot per-PID input stats under inStatsMu.
			p.inStatsMu.Lock()
			type pidSnap struct {
				In rtp.InStats `json:"in"`
				TS TsInStats   `json:"ts"`
			}
			snaps := make(map[string]pidSnap, len(p.pidStates))
			for pid, state := range p.pidStates {
				snaps[fmt.Sprintf("pid%d", pid)] = pidSnap{
					In: state.inStats,
					TS: state.tsStats,
				}
			}
			p.inStatsMu.Unlock()

			p.outStatsMu.Lock()
			outSnap := p.outStats.SwitchPeriod(now)
			p.outStatsMu.Unlock()

			p.conf.StatsLog.Info("stats",
				"stream", p.conf.Stream,
				"out", jsonutil.Stringer(outSnap),
				"in", jsonutil.Stringer(snaps))
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

		// Sleep until QueueAhead before targetTs, counting ticker ticks consumed.
		wakeTarget := entry.targetTs.Time.Add(-h.pacer.conf.QueueAhead.Duration())
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
			os.UpdateCHD(now, entry.inTs)
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

// tsPcrUnwrapper converts 42-bit PCR values to a monotonic int64 sequence, handling wraparound at MaxPCR.
type tsPcrUnwrapper struct {
	hasLast  bool
	last     uint64
	current  int64
	previous int64
}

func (u *tsPcrUnwrapper) unwrap(pcr uint64) (previous, current int64) {
	if !u.hasLast {
		u.hasLast = true
		u.last = pcr
		u.current = int64(pcr)
		u.previous = u.current - 1
		return u.previous, u.current
	}

	// PCR is a 42-bit counter. Detect wraparound by checking whether the signed difference exceeds half the range.
	const halfRange = int64(MaxPCR / 2)
	diff := int64(pcr) - int64(u.last)
	if diff < -halfRange {
		diff += int64(MaxPCR) + 1 // wrapped forward
	} else if diff > halfRange {
		diff -= int64(MaxPCR) + 1 // wrapped backward
	}

	u.previous = u.current
	u.current += diff
	u.last = pcr
	return u.previous, u.current
}

// tsPcrGapDetector detects PCR jumps larger than a configured threshold.
type tsPcrGapDetector struct {
	unwrapper tsPcrUnwrapper
	threshold int64 // max allowed delta in PCR ticks; 0 = no gap detection
}

// detect unwraps the PCR and returns whether a gap (delta > threshold) was detected.
func (d *tsPcrGapDetector) detect(pcr uint64) (previous, current int64, gap bool) {
	previous, current = d.unwrapper.unwrap(pcr)
	if !d.unwrapper.hasLast {
		return // first ever call — not a gap
	}
	if d.threshold > 0 {
		delta := current - previous
		if delta < 0 {
			delta = -delta
		}
		gap = delta > d.threshold
	}
	return
}
