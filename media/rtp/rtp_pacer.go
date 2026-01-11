package rtp

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/eluv-io/common-go/format/duration"
	"github.com/eluv-io/common-go/util/jsonutil"
	"github.com/eluv-io/common-go/util/statsutil"
	"github.com/eluv-io/common-go/util/timeutil"
	"github.com/eluv-io/errors-go"
	"github.com/eluv-io/utc-go"
)

// WrapTicks is the number of ticks that the RTP timestamp wraps around
var WrapTicks = uint64(1 << 32)

// WrapDuration is the duration for RTP timestamps to wrap around (based on 90kHz clock)
var WrapDuration = TicksToDuration(1 << 32)

// NewRtpPacer creates a pacer that can be used to playout RTP packets at the correct rate.
func NewRtpPacer() *RtpPacer {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &RtpPacer{
		logThrottler: timeutil.NewPeriodic(1 * time.Second),
		stream:       "n/a",
		ctx:          ctx,
		cancel:       cancel,
		packetCh:     make(chan *pacerPacket, 10_000),
		gapDetector:  NewRtpGapDetector(1, time.Second),
	}
}

// RtpPacer is an implementation of srtpub.Pacer that can be used to playout RTP packets at the correct rate.
type RtpPacer struct {
	initialDelay  time.Duration     // initial delay before sending the first packet
	adjustTimeRef bool              // whether to adjust the time reference when RTP packets arrive too early
	logThrottler  timeutil.Periodic // prevent spamming the log with errors
	stream        string            // stream name

	start       utc.UTC      // the wall clock time when the stream started
	last        utc.UTC      // the wall clock time when the last packet was received
	refTime     timeref      // timing reference (of first packet or adapted dynamically)
	gapDetector *GapDetector // RTP gap detector (and unwrapper for RTP timestamps)
	stats       stats        // statistics

	// async mode
	packetCh chan *pacerPacket       // channel to send packets to
	ctx      context.Context         // context for canceling the pacer
	cancel   context.CancelCauseFunc // to cancel the pacer
	outStats outStats                // output statistics
}

func (p *RtpPacer) SetDelay(delay time.Duration) {
	p.initialDelay = delay
}

func (p *RtpPacer) WithDelay(delay time.Duration) *RtpPacer {
	p.SetDelay(delay)
	return p
}

func (p *RtpPacer) WithAdjustTimeRef(adjust bool) *RtpPacer {
	p.adjustTimeRef = adjust
	return p
}

func (p *RtpPacer) WithStream(name string) *RtpPacer {
	p.stream = name
	return p
}

func (p *RtpPacer) WithNoLog() *RtpPacer {
	p.logThrottler = NoopPeriodic{}
	return p
}

func (p *RtpPacer) Wait(bts []byte) {
	if p.initialDelay > 0 {
		time.Sleep(p.initialDelay)
		p.initialDelay = 0
	}

	pkt, err := ParsePacket(bts)
	if err != nil {
		p.logThrottler.Do(func() {
			log.Info("rtpPacer: packet error", errors.ClearStacktrace(err), "stream", p.stream)
		})
		return
	}

	now := utc.Now()
	wait := p.CalculateWait(now, pkt.SequenceNumber, pkt.Timestamp)
	if wait > 0 {
		// log.Trace("rtpPacer: wait",
		// 	"stream", p.stream,
		// 	"d", now.Sub(p.start),
		// 	"timestamp", pkt.Timestamp,
		// 	"wait", wait)
		time.Sleep(wait)
	} else if wait < -20*time.Millisecond {
		// log.Info("rtpPacer: packet delayed",
		// 	"stream", p.stream,
		// 	"d", now.Sub(p.start),
		// 	"timestamp", pkt.Timestamp,
		// 	"wait", wait)
	}
}

func (p *RtpPacer) CalculateWait(now utc.UTC, rtpSequence uint16, rtpTimestamp uint32) time.Duration {
	wait, _ := p.calculateWait(now, rtpSequence, rtpTimestamp)
	return wait
}

func (p *RtpPacer) calculateWait(now utc.UTC, rtpSequence uint16, rtpTimestamp uint32) (time.Duration, bool) {
	p.stats.TotalPackets++
	seqUnwrapped, tsUnwrapped, errGap := p.gapDetector.Detect(rtpSequence, rtpTimestamp)

	if p.start.IsZero() {
		// first packet: initialize internal state
		p.start = now
		p.last = now
		p.refTime.wallClock = now
		p.refTime.rtpTimestamp = tsUnwrapped
		p.refTime.rtpSequence = seqUnwrapped
		return 0, false
	}

	defer func() {
		newPeriod := p.stats.ipd.UpdateNow(now, duration.Spec(now.Sub(p.last)))
		if newPeriod {
			p.stats.IPDLast = p.stats.ipd.Previous
			log.Debug("rtpPacer: statistics", "stream", p.stream, "stats", jsonutil.Stringer(p.stats))
		}
		p.last = now
	}()

	p.stats.RtpSeq = seqUnwrapped
	p.stats.RtpTs = tsUnwrapped

	tickDiff := tsUnwrapped - p.refTime.rtpTimestamp
	tsDiff := TicksToDuration(tickDiff)

	targetClock := p.refTime.wallClock.Add(tsDiff)
	clockDiff := targetClock.Sub(now)

	// adjust the time reference if an RTP gap is detected or the clock is too far off even if timeref adjustments are
	// disabled... Otherwise, the wait time can be huge.
	if errGap != nil || p.adjustTimeRef && clockDiff > 0 || clockDiff > time.Second {
		if errGap != nil {
			log.Warn("rtpPacer: gap detected", errGap)
			p.stats.Discontinuities++
		} else {
			// actual time diff is smaller than the time difference based on the RTP timestamps
			p.stats.RefTimeChanges++
			p.stats.RefTimeDiff += duration.Spec(tsDiff)
		}

		log.Throttle("pacer").Info("rtpPacer: adjusting time reference",
			"stream", p.stream,
			"d", now.Sub(p.start),
			"old_ref", p.refTime.wallClock,
			"new_ref", now,
			"clock_diff", clockDiff,
			"old_rtp_ts", p.refTime.rtpTimestamp,
			"new_rtp_ts", tsUnwrapped,
			"ts_diff", fmt.Sprintf("%d/%s", tickDiff, tsDiff),
			"old_rtp_seq", p.refTime.rtpSequence,
			"new_rtp_seq", seqUnwrapped,
			"stats", jsonutil.Stringer(p.stats),
		)

		// ==> adjust the reference time
		p.refTime.wallClock = now
		p.refTime.rtpTimestamp = tsUnwrapped
		p.refTime.rtpSequence = seqUnwrapped

		// PENDING(LUK): Add startup phase during which all packets are discarded.
		//
		// Possibilities:
		//  - if no more timeref adjustments for x seconds
		//  - threshold on adjustment duration
		//  - constant: x seconds
		//
		// return 0, now.Sub(p.start) < time.Second

		// ==> send immediately, don't wait
		return 0, false
	}

	wait := clockDiff

	if wait < -20*time.Millisecond {
		p.stats.DelayedPackets++
	} else if wait > 20*time.Millisecond {
		p.stats.EarlyPackets++
	}
	p.stats.MinWait = min(p.stats.MinWait, duration.Spec(wait))
	p.stats.MaxWait = max(p.stats.MaxWait, duration.Spec(wait))
	p.stats.waitSum += duration.Spec(wait)
	p.stats.AvgWait = p.stats.waitSum / duration.Spec(p.stats.TotalPackets)

	return wait, false
}

func TicksToDuration(ts int64) time.Duration {
	// RTP with video uses a 90kHz clock, i.e. 1 tick = 1/90000 s or 1s = 90000 ticks
	return time.Duration(ts) * 100 * time.Microsecond / 9
}

func DurationToTicks(ts time.Duration) int64 {
	// RTP with video uses a 90kHz clock, i.e. 1 tick = 1/90000 s or 1s = 90000 ticks
	return int64((ts*9 + 1) / 100 / time.Microsecond)
}

type timeref struct {
	wallClock    utc.UTC // the wall clock time clock time corresponding to the rtpTimestamp
	rtpTimestamp int64   // the reference RTP timestamp. Initially the first timestamp received, then adapted dynamically
	rtpSequence  int64   // the reference RTP sequence number (informative only).
}

type stats struct {
	TotalPackets    int                                 `json:"total"`
	DelayedPackets  int                                 `json:"delayed"`          // packets delayed by more than 20ms
	EarlyPackets    int                                 `json:"early"`            // packets early by more than 20ms
	RtpSeq          int64                               `json:"rtp_seq"`          // current RTP sequence number
	RtpTs           int64                               `json:"rtp_ts"`           // current RTP timestamp
	MinWait         duration.Spec                       `json:"min_wait"`         // minimum wait time - negative for delayed packets
	MaxWait         duration.Spec                       `json:"max_wait"`         // maximum wait time
	AvgWait         duration.Spec                       `json:"avg_wait"`         // average wait time
	Discontinuities int                                 `json:"discontinuities"`  // number of times the stream was reset
	RefTimeChanges  int                                 `json:"ref_time_changes"` // number of times the reference time was adapted
	RefTimeDiff     duration.Spec                       `json:"ref_time_diff"`    // total time difference of all reference time adaptations
	IPDLast         statsutil.Statistics[duration.Spec] `json:"ipd"`              // inter-packet delay statistics for last period
	ipd             statsutil.Periodic[duration.Spec]   // inter-packet delay statistics
	waitSum         duration.Spec                       // sum of all wait times
}

type outStats struct {
	WaitLast        statsutil.Statistics[duration.Spec] `json:"wait"`          // wait time statistics for last period
	IPDLast         statsutil.Statistics[duration.Spec] `json:"ipd"`           // inter-packet delay statistics for last period
	CHDLast         statsutil.Statistics[duration.Spec] `json:"chd"`           // channel delay statistics for last period
	BufferedPackets int32                               `json:"buffered"`      // number of packets currently in the channel
	DelayedPackets  int                                 `json:"delayed"`       // number of packets that were popped from the queue after their nominal sending time
	Sleeps          int                                 `json:"sleeps"`        // number of times the pacer had to wait before sending a packet
	OverSlept       int                                 `json:"over_slept"`    // number of times sleep was more than 5ms longer than expected
	MaxOverslept    time.Duration                       `json:"max_overslept"` // the maximum amount of time that a sleep was longer than expected

	wait       statsutil.Periodic[duration.Spec] // collector for wait times
	ipd        statsutil.Periodic[duration.Spec] // collector for inter-packet delays
	chd        statsutil.Periodic[duration.Spec] // collector for channel delays
	buffered   atomic.Int32                      // current count of packets in channel
	lastPacket utc.UTC                           // wall clock time when the last packet was popped
}

type NoopPeriodic struct{}

func (n NoopPeriodic) Do(func()) bool {
	return false
}
